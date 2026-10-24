// Package overpass turns a geographic area into a SUMO road network.
//
// The console lets an operator draw a box on the map and ask for a network
// covering it. That request has to reach OpenStreetMap, ask for the roads inside
// the box, and hand back a .net.xml the importer already understands. This
// package is that step.
//
// Two things it deliberately does not do. It does not call netconvert: the
// service is a single Go binary and shelling out to a SUMO install would make a
// deployment depend on one. It does not cache: an area is converted once, at the
// operator's request, and the result is installed or downloaded immediately.
package overpass

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrAreaTooLarge is returned when the requested box is bigger than the
	// builder accepts. A whole-island extract is minutes of Overpass time and
	// hundreds of megabytes of XML; refusing early is kinder than timing out.
	ErrAreaTooLarge = errors.New("selected area is too large")
	// ErrNoRoads is returned when the area contains no roads of the selected
	// classes.
	ErrNoRoads = errors.New("the selected area contains no roads of those classes")
	// ErrUpstream is returned when Overpass cannot be reached or refuses.
	ErrUpstream = errors.New("the road data service is unavailable")
)

// DefaultEndpoint is the Overpass interpreter the builder queries.
const DefaultEndpoint = "https://overpass-api.de/api/interpreter"

// RoadClasses are the highway values the builder can ask for, cheapest first.
// The names are OSM's own, because the query is OSM's.
var RoadClasses = []string{"motorway", "trunk", "primary", "secondary", "tertiary", "residential"}

// Area is a geographic bounding box in degrees.
type Area struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

// MaxAreaDeg2 bounds the box a request may cover.
//
// 0.25 square degrees is roughly 55 km by 55 km at Jakarta's latitude: larger
// than the city and its suburbs, small enough that Overpass answers in seconds
// rather than minutes.
const MaxAreaDeg2 = 0.25

// Validate checks the box is well formed and small enough to fetch.
func (a Area) Validate() error {
	switch {
	case a.MinLat < -90 || a.MaxLat > 90 || a.MinLon < -180 || a.MaxLon > 180:
		return fmt.Errorf("area is outside the world: %v", a)
	case a.MinLat >= a.MaxLat:
		return errors.New("area has min_lat greater than or equal to max_lat")
	case a.MinLon >= a.MaxLon:
		return errors.New("area has min_lon greater than or equal to max_lon")
	}
	deg2 := (a.MaxLat - a.MinLat) * (a.MaxLon - a.MinLon)
	if deg2 > MaxAreaDeg2 {
		return fmt.Errorf("%w: %.4f square degrees is over the %.2f limit; select a smaller box",
			ErrAreaTooLarge, deg2, MaxAreaDeg2)
	}
	return nil
}

// Query builds the Overpass QL that fetches the ways of the given classes in the
// area, together with the nodes those ways reference.
//
// The recursion is the part worth understanding: `(._;>;);` means "take the ways
// just selected and add everything they point at". Without it Overpass returns
// ways whose node references are dangling, which is not a usable extract.
func (a Area) Query(classes []string) string {
	if len(classes) == 0 {
		classes = []string{"motorway", "trunk", "primary", "secondary"}
	}
	pattern := "^(" + strings.Join(classes, "|") + ")(_link)?$"

	return fmt.Sprintf(`[out:xml][timeout:%d];way["highway"~"%s"](%s);(._;>;);out body;`,
		queryTimeoutS, pattern, a.bbox())
}

// bbox renders the area as Overpass's (south,west,north,east) order.
func (a Area) bbox() string {
	return fmt.Sprintf("%s,%s,%s,%s",
		trimFloat(a.MinLat), trimFloat(a.MinLon), trimFloat(a.MaxLat), trimFloat(a.MaxLon))
}

// queryTimeoutS is the time Overpass is given. It is a ceiling, not a target: a
// city box answers in a second or two, and this only bounds a pathological one.
const queryTimeoutS = 180

// maxResponseBytes bounds what the builder will read from Overpass. A city box
// of arterials is tens of megabytes; past this the answer is not a network a
// dashboard can draw.
const maxResponseBytes = 256 << 20

// Builder fetches areas and converts them to SUMO networks.
type Builder struct {
	// Endpoint is the Overpass interpreter URL. Empty means DefaultEndpoint.
	Endpoint string
	// Client is the HTTP client. Empty means a client with a generous timeout.
	Client *http.Client
}

// Fetch returns the raw OSM XML for the area.
func (b *Builder) Fetch(ctx context.Context, area Area, classes []string) ([]byte, error) {
	if err := area.Validate(); err != nil {
		return nil, err
	}

	form := url.Values{"data": {area.Query(classes)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Overpass answers a request with no User-Agent with 406, and it is entitled
	// to: the operator agent is how they tell a script from a browser.
	req.Header.Set("User-Agent", "FleetFlow/0.1 (network builder)")
	req.Header.Set("Accept", "*/*")

	resp, err := b.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read a little of the body: Overpass explains its refusals (rate limit,
		// capacity, syntax) and that text is more useful than the status alone.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrUpstream, resp.StatusCode,
			strings.TrimSpace(string(detail)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", ErrUpstream, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("%w: response exceeded %d bytes; select a smaller area",
			ErrAreaTooLarge, maxResponseBytes)
	}
	return body, nil
}

func (b *Builder) endpoint() string {
	if b.Endpoint != "" {
		return b.Endpoint
	}
	return DefaultEndpoint
}

func (b *Builder) client() *http.Client {
	if b.Client != nil {
		return b.Client
	}
	return &http.Client{Timeout: 4 * time.Minute}
}

// --- OSM XML -> SUMO .net.xml ---------------------------------------------

// osmDoc is the subset of OSM XML the converter reads.
type osmDoc struct {
	XMLName xml.Name  `xml:"osm"`
	Nodes   []osmNode `xml:"node"`
	Ways    []osmWay  `xml:"way"`
}

type osmNode struct {
	ID  string `xml:"id,attr"`
	Lat string `xml:"lat,attr"`
	Lon string `xml:"lon,attr"`
}

type osmWay struct {
	ID    string   `xml:"id,attr"`
	Nodes []osmNd  `xml:"nd"`
	Tags  []osmTag `xml:"tag"`
}

// osmNd is one node reference inside a way. The id is an attribute, not the
// element's text: `<nd ref="123"/>`.
type osmNd struct {
	Ref string `xml:"ref,attr"`
}

type osmTag struct {
	K string `xml:"k,attr"`
	V string `xml:"v,attr"`
}

// NetXML converts OSM XML into a SUMO .net.xml.
//
// The output is the small, well-defined part of SUMO's schema: a location with a
// projection, junctions at every node a road passes through, and two directed
// edges per way (one per direction) carrying the road's geometry. It is what
// netconvert would produce for the same input minus the lane-level turning
// model, and it is exactly what FleetFlow's importer reads back.
//
// The projection is an equirectangular one anchored at the centre of the area:
// over a city box that is accurate to a few metres, which is below the precision
// a routing graph needs and avoids depending on a projection library.
func NetXML(osmXML []byte) ([]byte, error) {
	var doc osmDoc
	if err := xml.Unmarshal(osmXML, &doc); err != nil {
		return nil, fmt.Errorf("could not parse OSM XML: %w", err)
	}
	if len(doc.Ways) == 0 {
		return nil, ErrNoRoads
	}

	coords := make(map[string]osmCoord, len(doc.Nodes))
	for _, n := range doc.Nodes {
		lat, latErr := strconv.ParseFloat(n.Lat, 64)
		lon, lonErr := strconv.ParseFloat(n.Lon, 64)
		if latErr != nil || lonErr != nil {
			continue
		}
		coords[n.ID] = osmCoord{lat: lat, lon: lon}
	}

	// Split ways at shared nodes so crossing roads actually meet.
	//
	// This is the part that decides whether the result is a road network or a
	// pile of unconnected lines. OSM ways share a node where they meet, but one
	// way usually runs through several junctions. If each way became a single
	// edge from its first node to its last, a junction in the middle of a way
	// would have no edge incident to it, and a router could never turn onto it.
	segments := splitAtJunctions(doc.Ways, coords)
	if len(segments) == 0 {
		return nil, ErrNoRoads
	}

	// Keep only the nodes that are the endpoint of some segment: a junction
	// exists because roads meet there. Intermediate shape vertices belong in an
	// edge's geometry, not in the junction list — emitting them would create
	// thousands of junctions with no road attached.
	used := make(map[string]bool)
	for _, seg := range segments {
		if seg.road.length < 2 {
			continue
		}
		used[seg.road.nodeIDs[0]] = true
		used[seg.road.nodeIDs[seg.road.length-1]] = true
	}
	if len(used) == 0 {
		return nil, ErrNoRoads
	}

	proj := newEquirectangular(coords, used)

	var out bytes.Buffer
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&out, "<net version=\"1.20\">\n")
	fmt.Fprintf(&out, "  <location netOffset=\"0,0\" convBoundary=\"%s\" origBoundary=\"%s\" projParameter=\"%s\"/>\n",
		proj.convBoundary(), proj.origBoundary(), proj.projParameter())

	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		x, y := proj.project(coords[id])
		fmt.Fprintf(&out, "  <junction id=%q type=\"priority\" x=\"%.2f\" y=\"%.2f\"/>\n", id, x, y)
	}

	for _, seg := range segments {
		writeWayEdges(&out, seg.id, seg.road, proj, coords)
	}

	out.WriteString("</net>\n")
	return out.Bytes(), nil
}

// segment is one piece of an OSM way between two junctions.
type segment struct {
	// id is the SUMO edge id, unique across all segments.
	id   string
	road wayRoad
}

// splitAtJunctions breaks ways into the stretches between junctions.
//
// A node is a junction when more than one way references it. Each way is then
// cut at those nodes, so every junction becomes the endpoint of the segments
// that meet there and the resulting graph is connected the way the roads are.
func splitAtJunctions(ways []osmWay, coords map[string]osmCoord) []segment {
	// Count how many ways use each node. A node used twice is a meeting point.
	refs := make(map[string]int)
	for _, w := range ways {
		// A way can reference the same node twice (a loop); count it once so a
		// loop's own endpoints are not mistaken for a junction.
		seen := make(map[string]bool, len(w.Nodes))
		for _, nd := range w.Nodes {
			if seen[nd.Ref] {
				continue
			}
			seen[nd.Ref] = true
			refs[nd.Ref]++
		}
	}

	var segments []segment
	for _, w := range ways {
		road := readWay(w, coords)
		if road.length < 2 {
			continue
		}

		start := 0
		for i := 1; i < len(road.nodeIDs); i++ {
			last := i == len(road.nodeIDs)-1
			if !last && refs[road.nodeIDs[i]] < 2 {
				continue
			}
			part := road
			part.nodeIDs = road.nodeIDs[start : i+1]
			part.length = len(part.nodeIDs)
			segments = append(segments, segment{
				// The slice index keeps the id unique when a way is cut more
				// than once.
				id:   fmt.Sprintf("%s_%d", w.ID, len(segments)),
				road: part,
			})
			start = i
		}
	}
	return segments
}

// wayRoad describes the routable content of one OSM way.
type wayRoad struct {
	class    string
	speedMPS float64
	lanes    int
	oneway   bool
	nodeIDs  []string
	length   int
}

// highwayDefaults maps an OSM highway class to a speed limit and lane count.
// These are the assumptions netconvert would otherwise infer; they are stated
// here rather than hidden so the numbers a run reports can be traced back.
var highwayDefaults = map[string]struct {
	speedKMH float64
	lanes    int
}{
	"motorway":       {100, 3},
	"motorway_link":  {60, 1},
	"trunk":          {80, 2},
	"trunk_link":     {50, 1},
	"primary":        {60, 2},
	"primary_link":   {40, 1},
	"secondary":      {50, 2},
	"secondary_link": {40, 1},
	"tertiary":       {40, 1},
	"tertiary_link":  {30, 1},
	"residential":    {30, 1},
	"unclassified":   {30, 1},
}

// readWay reads the tags that matter and the node chain, dropping nodes with no
// coordinates so a partial extract still converts.
func readWay(w osmWay, coords map[string]osmCoord) wayRoad {
	road := wayRoad{}
	tags := make(map[string]string, len(w.Tags))
	for _, t := range w.Tags {
		tags[t.K] = t.V
	}
	road.class = tags["highway"]

	def, ok := highwayDefaults[road.class]
	if !ok {
		return road
	}
	road.speedMPS = def.speedKMH / 3.6
	road.lanes = def.lanes

	// maxspeed is authoritative when it is a plain number in km/h. Values like
	// "50 mph" or "signals" are left to the class default rather than parsed
	// wrongly.
	if v, err := strconv.ParseFloat(strings.TrimSpace(tags["maxspeed"]), 64); err == nil && v > 0 && v <= 200 {
		road.speedMPS = v / 3.6
	}
	if v, err := strconv.Atoi(strings.TrimSpace(tags["lanes"])); err == nil && v > 0 && v <= 12 {
		road.lanes = v
	}
	road.oneway = tags["oneway"] == "yes" || tags["oneway"] == "true" || tags["oneway"] == "1" ||
		road.class == "motorway"

	for _, nd := range w.Nodes {
		if _, ok := coords[nd.Ref]; ok {
			road.nodeIDs = append(road.nodeIDs, nd.Ref)
		}
	}
	road.length = len(road.nodeIDs)
	return road
}

// writeWayEdges writes one directed edge per direction, unless the way is one-way.
func writeWayEdges(out *bytes.Buffer, wayID string, road wayRoad, proj equirectangular, coords map[string]osmCoord) {
	shape := make([]string, 0, road.length)
	for _, id := range road.nodeIDs {
		x, y := proj.project(coords[id])
		shape = append(shape, fmt.Sprintf("%.2f,%.2f", x, y))
	}
	shapeAttr := strings.Join(shape, " ")

	// A SUMO edge id may not contain characters that would confuse the parser,
	// and an OSM way id is numeric, so the direction suffix is enough to keep
	// the two directions apart.
	writeEdge(out, wayID+"_f", road.nodeIDs[0], road.nodeIDs[road.length-1], road, shapeAttr)
	if road.oneway {
		return
	}
	reversed := make([]string, 0, len(shape))
	for i := len(shape) - 1; i >= 0; i-- {
		reversed = append(reversed, shape[i])
	}
	writeEdge(out, wayID+"_r", road.nodeIDs[road.length-1], road.nodeIDs[0], road, strings.Join(reversed, " "))
}

func writeEdge(out *bytes.Buffer, id, from, to string, road wayRoad, shape string) {
	fmt.Fprintf(out, "  <edge id=%q from=%q to=%q priority=\"1\" type=%q>\n", id, from, to, road.class)
	for lane := 0; lane < road.lanes; lane++ {
		fmt.Fprintf(out, "    <lane id=%q index=\"%d\" speed=\"%.2f\" length=\"%.2f\" shape=%q/>\n",
			fmt.Sprintf("%s_%d", id, lane), lane, road.speedMPS, edgeLengthM(shape), shape)
	}
	out.WriteString("  </edge>\n")
}

// edgeLengthM sums the segment lengths of a "x,y x,y ..." shape in metres.
func edgeLengthM(shape string) float64 {
	var total float64
	var prevX, prevY float64
	first := true
	for _, pair := range strings.Fields(shape) {
		parts := strings.Split(pair, ",")
		if len(parts) != 2 {
			continue
		}
		x, xErr := strconv.ParseFloat(parts[0], 64)
		y, yErr := strconv.ParseFloat(parts[1], 64)
		if xErr != nil || yErr != nil {
			continue
		}
		if !first {
			total += hypot(x-prevX, y-prevY)
		}
		prevX, prevY, first = x, y, false
	}
	return total
}

func hypot(a, b float64) float64 {
	return math.Hypot(a, b)
}

// osmCoord is a latitude/longitude pair.
type osmCoord struct {
	lat float64
	lon float64
}

// equirectangular converts latitude/longitude to local metres.
//
// A city box spans a fraction of a degree, over which treating a degree of
// longitude as a constant number of metres is accurate to metres. That is well
// inside what a routing graph needs, and it avoids a projection dependency in a
// binary that is meant to stay small.
type equirectangular struct {
	originLat float64
	originLon float64
	// metresPerDegLat and metresPerDegLon scale degrees to metres.
	metresPerDegLat float64
	metresPerDegLon float64
	minLat, minLon  float64
	maxLat, maxLon  float64
	// offsetX and offsetY shift the projected frame so the area's south-west
	// corner is (0,0). SUMO's convBoundary starts there, so the junction
	// coordinates written into the file must be in the same frame or every
	// junction lands a corner's width away from where the boundary says it is.
	offsetX, offsetY float64
}

const earthRadiusM = 6371000.0

func newEquirectangular(coords map[string]osmCoord, used map[string]bool) equirectangular {
	p := equirectangular{minLat: 90, minLon: 180, maxLat: -90, maxLon: -180}
	for id := range used {
		c := coords[id]
		if c.lat < p.minLat {
			p.minLat = c.lat
		}
		if c.lat > p.maxLat {
			p.maxLat = c.lat
		}
		if c.lon < p.minLon {
			p.minLon = c.lon
		}
		if c.lon > p.maxLon {
			p.maxLon = c.lon
		}
	}
	p.originLat = (p.minLat + p.maxLat) / 2
	p.originLon = (p.minLon + p.maxLon) / 2

	radians := math.Pi / 180
	p.metresPerDegLat = earthRadiusM * radians
	p.metresPerDegLon = earthRadiusM * radians * math.Cos(p.originLat*radians)
	if p.metresPerDegLon <= 0 {
		p.metresPerDegLon = p.metresPerDegLat
	}

	// Anchor the frame's origin at the south-west corner, which is where
	// convBoundary starts.
	p.offsetX = (p.minLon - p.originLon) * p.metresPerDegLon
	p.offsetY = (p.minLat - p.originLat) * p.metresPerDegLat
	return p
}

// project converts a coordinate to metres east and north of the area's
// south-west corner, the frame convBoundary declares.
func (p equirectangular) project(c osmCoord) (x, y float64) {
	return (c.lon-p.originLon)*p.metresPerDegLon - p.offsetX,
		(c.lat-p.originLat)*p.metresPerDegLat - p.offsetY
}

// convBoundary is SUMO's projected extent: zero at the south-west corner, as
// netconvert writes it.
func (p equirectangular) convBoundary() string {
	x, y := p.project(osmCoord{lat: p.maxLat, lon: p.maxLon})
	return fmt.Sprintf("%.2f,%.2f,%.2f,%.2f", 0.0, 0.0, x, y)
}

// origBoundary is the same box in geographic coordinates, which is what the
// importer uses to recover latitude and longitude.
func (p equirectangular) origBoundary() string {
	return fmt.Sprintf("%s,%s,%s,%s",
		trimFloat(p.minLon), trimFloat(p.minLat), trimFloat(p.maxLon), trimFloat(p.maxLat))
}

func (p equirectangular) projParameter() string {
	return "+proj=longlat +ellps=WGS84 +datum=WGS84 +no_defs"
}

// cos is deliberately not reimplemented: math.Cos is exact enough and correct
// for every latitude, including the poles, where a range-reduced series is not.
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}
