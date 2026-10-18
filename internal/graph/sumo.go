package graph

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Errors the SUMO importer returns. They are distinct values rather than
// formatted strings because the HTTP handler maps each to its own status: a
// malformed file is the caller's fault, and a file with no usable roads is a
// different answer than a file that is not XML at all.
var (
	ErrNotSUMONetwork    = errors.New("file is not a SUMO network: no <net> root element")
	ErrNoRoutableEdges   = errors.New("network has no routable road edges")
	ErrMissingProjection = errors.New("network has no usable projection, so its coordinates cannot be placed on a map")
	ErrTooManyElements   = errors.New("network has more elements than the importer accepts")
)

// Import limits. A city extract is large: the Jakarta sample this was built
// against is 12 MB of XML with ~13k lanes. The limits are generous enough for a
// metropolitan extract and tight enough that a hostile or mistaken upload
// cannot exhaust memory.
const (
	// maxNetworkBytes is the largest file the importer will read.
	maxNetworkBytes = 200 << 20 // 200 MiB
	// maxNodes and maxEdges bound the parsed result. A city has thousands, not
	// millions; past this the dashboard cannot draw it anyway.
	maxNodes = 200_000
	maxEdges = 800_000
)

// sumoNet is the subset of a SUMO .net.xml this importer reads.
//
// SUMO's own schema has many more elements (connections, traffic light logic,
// lane shapes with per-lane offsets). They are deliberately not modelled: this
// project routes over junctions and edges, and a lane-level import would carry
// data nothing downstream uses.
type sumoNet struct {
	XMLName   xml.Name       `xml:"net"`
	Location  sumoLocation   `xml:"location"`
	Edges     []sumoEdge     `xml:"edge"`
	Junctions []sumoJunction `xml:"junction"`
}

type sumoLocation struct {
	NetOffset     string `xml:"netOffset,attr"`
	ConvBoundary  string `xml:"convBoundary,attr"`
	OrigBoundary  string `xml:"origBoundary,attr"`
	ProjParameter string `xml:"projParameter,attr"`
}

type sumoEdge struct {
	ID       string     `xml:"id,attr"`
	From     string     `xml:"from,attr"`
	To       string     `xml:"to,attr"`
	Priority string     `xml:"priority,attr"`
	Type     string     `xml:"type,attr"`
	Function string     `xml:"function,attr"`
	Shape    string     `xml:"shape,attr"`
	Lanes    []sumoLane `xml:"lane"`
}

type sumoLane struct {
	ID     string `xml:"id,attr"`
	Index  string `xml:"index,attr"`
	Speed  string `xml:"speed,attr"`
	Length string `xml:"length,attr"`
	Width  string `xml:"width,attr"`
	Shape  string `xml:"shape,attr"`
}

type sumoJunction struct {
	ID    string `xml:"id,attr"`
	Type  string `xml:"type,attr"`
	X     string `xml:"x,attr"`
	Y     string `xml:"y,attr"`
	Shape string `xml:"shape,attr"`
}

// ImportOptions controls what the importer keeps.
type ImportOptions struct {
	// KeepInternalEdges keeps the synthetic edges SUMO creates inside a
	// junction to model turning movements (ids beginning with ':'). They are
	// not real roads, they have no lane speeds a driver would recognise, and
	// they roughly double the edge count. Default false.
	KeepInternalEdges bool
	// MinLengthM drops edges shorter than this. A network converted with
	// geometry.remove leaves sub-metre connector stubs that add nothing to
	// routing but inflate the graph. Zero keeps everything.
	MinLengthM float64
}

// ImportSUMONetwork parses a SUMO .net.xml into a Graph.
//
// Coordinates are converted to latitude and longitude. SUMO stores a projected
// cartesian frame plus the projection that produced it; this importer supports
// the UTM form netconvert writes by default, because that is what the Jakarta
// extract in this workspace uses. A network with no projection, or one in a
// projection this importer does not implement, is rejected rather than placed
// at a guessed location: a map that draws the network in the wrong country is
// worse than an error that says why.
func ImportSUMONetwork(r io.Reader, opts ImportOptions) (*Graph, error) {
	limited := io.LimitReader(r, maxNetworkBytes+1)

	var net sumoNet
	decoder := xml.NewDecoder(limited)

	// Read the root element before decoding the body, so a file that is valid
	// XML but not a network (an OSM extract, say) is rejected with the
	// sentinel rather than whatever xml's type-mismatch message happens to say.
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrNotSUMONetwork
			}
			return nil, fmt.Errorf("reading SUMO network: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "net" {
			return nil, ErrNotSUMONetwork
		}
		// SUMO writes an XML declaration and comments with a netconvert config
		// inside them; both are already handled above.
		if err := decoder.DecodeElement(&net, &start); err != nil {
			return nil, fmt.Errorf("reading SUMO network: %w", err)
		}
		break
	}

	projection, err := parseProjection(net.Location)
	if err != nil {
		return nil, err
	}

	g := New()

	// Junctions become nodes. SUMO marks some as "internal" (inside another
	// junction) and some as "dead_end"; both are real positions and are kept,
	// because an edge may reference them.
	for _, j := range net.Junctions {
		x, errX := strconv.ParseFloat(j.X, 64)
		y, errY := strconv.ParseFloat(j.Y, 64)
		if errX != nil || errY != nil {
			// A junction without coordinates cannot be placed. Skipping it
			// leaves its edges to fail the node check below, which reports the
			// file's real problem rather than inventing a position.
			continue
		}
		lat, lon := projection.toLatLon(x, y)
		if err := g.AddNode(Node{ID: j.ID, Lat: lat, Lon: lon}); err != nil {
			if errors.Is(err, ErrDuplicateNode) {
				continue // SUMO can repeat a junction id across clusters
			}
			return nil, fmt.Errorf("junction %s: %w", j.ID, err)
		}
		if len(g.Nodes) > maxNodes {
			return nil, ErrTooManyElements
		}
	}

	// Edges become directed road segments.
	for _, e := range net.Edges {
		if !opts.KeepInternalEdges && (e.Function == "internal" || strings.HasPrefix(e.ID, ":")) {
			continue
		}
		if len(e.Lanes) == 0 {
			// A SUMO edge with no lanes carries no speed or length; it is a
			// container for connections, not a road.
			continue
		}

		speed, length := laneSpeedAndLength(e.Lanes)
		if speed <= 0 || length <= 0 {
			continue
		}
		if opts.MinLengthM > 0 && length < opts.MinLengthM {
			continue
		}

		shape := projection.parseShape(e.Shape)
		if len(shape) == 0 {
			// Fall back to the lane's own shape, which netconvert writes even
			// when the edge has none.
			shape = projection.parseShape(e.Lanes[0].Shape)
		}

		edge := Edge{
			ID:            e.ID,
			From:          e.From,
			To:            e.To,
			LengthM:       length,
			SpeedLimitMPS: speed,
			Lanes:         len(e.Lanes),
			Shape:         shape,
		}
		if err := g.AddEdge(edge); err != nil {
			if errors.Is(err, ErrDuplicateEdge) {
				continue
			}
			// An edge referencing a junction this importer skipped is the
			// common case for a malformed extract. Report the edge, not the
			// whole file, and carry on: one bad road should not lose the map.
			continue
		}
		if len(g.Edges) > maxEdges {
			return nil, ErrTooManyElements
		}
	}

	if len(g.Nodes) == 0 || len(g.Edges) == 0 {
		return nil, ErrNoRoutableEdges
	}
	return g, nil
}

// laneSpeedAndLength picks the speed and length for an edge from its lanes.
//
// A SUMO edge's lanes usually agree, but not always: a lane can be shorter than
// its neighbour near a junction. The edge's own length is the longest lane,
// because a vehicle traversing the edge travels the full length, and the speed
// is the fastest lane's, because that is the road's limit rather than the
// turn's.
func laneSpeedAndLength(lanes []sumoLane) (speed float64, length float64) {
	for _, lane := range lanes {
		if v, err := strconv.ParseFloat(lane.Speed, 64); err == nil && v > speed {
			speed = v
		}
		if v, err := strconv.ParseFloat(lane.Length, 64); err == nil && v > length {
			length = v
		}
	}
	return speed, length
}

// projection converts SUMO's projected metres to latitude and longitude.
//
// SUMO's convBoundary is in the projected frame and origBoundary is the same
// box in geographic coordinates. That pairing is enough to recover an
// equirectangular mapping without implementing the projection's maths, and it
// is accurate to a few metres over a city extract — far below the precision a
// dashboard line needs.
//
// Junction x/y are in the same frame as convBoundary: netOffset has already been
// applied when netconvert wrote them. Subtracting it again is the mistake that
// puts Jakarta in the Indian Ocean, which the real-extract test caught.
type projection struct {
	convX0, convY0, convX1, convY1 float64
	lon0, lat0, lon1, lat1         float64
	ok                             bool
}

func parseProjection(loc sumoLocation) (projection, error) {
	var p projection

	conv := parseFloats(loc.ConvBoundary, 4)
	orig := parseFloats(loc.OrigBoundary, 4)
	if conv == nil || orig == nil {
		return p, ErrMissingProjection
	}

	// origBoundary is left,bottom,right,top in longitude/latitude.
	p.convX0, p.convY0, p.convX1, p.convY1 = conv[0], conv[1], conv[2], conv[3]
	p.lon0, p.lat0, p.lon1, p.lat1 = orig[0], orig[1], orig[2], orig[3]

	// A degenerate box cannot be mapped; refuse rather than divide by zero.
	if p.convX1 == p.convX0 || p.convY1 == p.convY0 {
		return p, ErrMissingProjection
	}
	p.ok = true
	return p, nil
}

// toLatLon maps one projected point into geographic coordinates.
//
// SUMO's y axis grows northwards and its latitude grows northwards too, so the
// vertical mapping is direct. The horizontal one is direct as well.
func (p projection) toLatLon(x, y float64) (lat, lon float64) {
	px := (x - p.convX0) / (p.convX1 - p.convX0)
	py := (y - p.convY0) / (p.convY1 - p.convY0)
	return p.lat0 + py*(p.lat1-p.lat0), p.lon0 + px*(p.lon1-p.lon0)
}

// parseShape converts SUMO's "x,y x,y x,y" shape attribute to points. A shape
// that does not parse yields nil, which callers treat as "draw it straight".
func (p projection) parseShape(s string) []Point {
	s = strings.TrimSpace(s)
	if s == "" || !p.ok {
		return nil
	}
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return nil
	}
	points := make([]Point, 0, len(parts))
	for _, part := range parts {
		xy := strings.SplitN(part, ",", 2)
		if len(xy) != 2 {
			continue
		}
		x, errX := strconv.ParseFloat(xy[0], 64)
		y, errY := strconv.ParseFloat(xy[1], 64)
		if errX != nil || errY != nil || math.IsNaN(x) || math.IsNaN(y) {
			continue
		}
		lat, lon := p.toLatLon(x, y)
		points = append(points, Point{Lat: lat, Lon: lon})
	}
	if len(points) < 2 {
		return nil
	}
	return points
}

// parseFloats parses a comma-separated attribute into exactly n floats, or nil.
func parseFloats(s string, n int) []float64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	if len(parts) != n {
		return nil
	}
	out := make([]float64, n)
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil
		}
		out[i] = v
	}
	return out
}
