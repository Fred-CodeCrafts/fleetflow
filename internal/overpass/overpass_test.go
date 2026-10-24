package overpass

import (
	"errors"
	"math"
	"strings"
	"testing"

	"fleetflow/internal/graph"
)

// twoCrossingWays is the smallest document that exercises the interesting part:
// a way that runs through a junction in the middle of it.
//
// Way 100 runs west to east through node 2. Way 200 runs north to south and
// meets it at node 2. For the result to be a road network, way 100 must be cut
// at node 2, or no edge would touch the junction and nothing could turn onto it.
const twoCrossingWays = `<?xml version="1.0" encoding="UTF-8"?>
<osm version="0.6">
  <node id="1" lat="-6.2000" lon="106.8000"/>
  <node id="2" lat="-6.2000" lon="106.8010"/>
  <node id="3" lat="-6.2000" lon="106.8020"/>
  <node id="4" lat="-6.2010" lon="106.8010"/>
  <node id="5" lat="-6.1990" lon="106.8010"/>
  <way id="100">
    <nd ref="1"/><nd ref="2"/><nd ref="3"/>
    <tag k="highway" v="primary"/>
  </way>
  <way id="200">
    <nd ref="4"/><nd ref="2"/><nd ref="5"/>
    <tag k="highway" v="secondary"/>
  </way>
</osm>`

func TestNetXMLSplitsWaysAtJunctions(t *testing.T) {
	net, err := NetXML([]byte(twoCrossingWays))
	if err != nil {
		t.Fatalf("NetXML: %v", err)
	}

	g, err := graph.ImportSUMONetwork(strings.NewReader(string(net)), graph.ImportOptions{MinLengthM: 1})
	if err != nil {
		t.Fatalf("the generated network does not import: %v\n%s", err, net)
	}

	// Five nodes in the source, and all five are junctions.
	if len(g.Nodes) != 5 {
		t.Errorf("junctions = %d, want 5", len(g.Nodes))
	}

	// Way 100 is cut at node 2 into two segments, way 200 into two more, and
	// every segment is two-way: eight directed edges.
	if len(g.Edges) != 8 {
		t.Errorf("edges = %d, want 8\n%s", len(g.Edges), net)
	}

	// The junction is reachable from every direction, which is the property the
	// split exists to guarantee.
	out := g.OutgoingEdgeIndices("2")
	if len(out) != 4 {
		t.Errorf("junction 2 has %d outgoing edges, want 4 (one per direction of both roads)", len(out))
	}
}

func TestNetXMLPreservesGeometryAndPlacement(t *testing.T) {
	net, err := NetXML([]byte(twoCrossingWays))
	if err != nil {
		t.Fatalf("NetXML: %v", err)
	}

	g, err := graph.ImportSUMONetwork(strings.NewReader(string(net)), graph.ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	shaped := 0
	for _, e := range g.Edges {
		if len(e.Shape) >= 2 {
			shaped++
		}
		if e.LengthM <= 0 || e.SpeedLimitMPS <= 0 || e.Lanes <= 0 {
			t.Fatalf("edge %s has length=%v speed=%v lanes=%v", e.ID, e.LengthM, e.SpeedLimitMPS, e.Lanes)
		}
	}
	if shaped != len(g.Edges) {
		t.Errorf("%d of %d edges carry geometry, want all", shaped, len(g.Edges))
	}

	// The coordinates must survive the round trip: a network that imports but
	// lands in the wrong place is worse than one that fails. The source nodes
	// span -6.2010..-6.1990 and 106.8000..106.8020.
	const eps = 1e-5
	for _, n := range g.Nodes {
		if n.Lat < -6.2010-eps || n.Lat > -6.1990+eps {
			t.Errorf("node %s latitude %v is outside the source area", n.ID, n.Lat)
		}
		if n.Lon < 106.8000-eps || n.Lon > 106.8020+eps {
			t.Errorf("node %s longitude %v is outside the source area", n.ID, n.Lon)
		}
	}

	// A metre of error would be invisible at city scale but would quietly shift
	// every junction, so pin the frame exactly: node 1 is the west end.
	west := g.Nodes["1"]
	if math.Abs(west.Lon-106.8000) > 1e-6 || math.Abs(west.Lat-(-6.2000)) > 1e-6 {
		t.Errorf("node 1 round-tripped to %v,%v want 106.8000,-6.2000", west.Lon, west.Lat)
	}
}

func TestNetXMLHonoursOneway(t *testing.T) {
	const doc = `<osm version="0.6">
  <node id="1" lat="-6.20" lon="106.80"/>
  <node id="2" lat="-6.21" lon="106.81"/>
  <way id="9">
    <nd ref="1"/><nd ref="2"/>
    <tag k="highway" v="primary"/>
    <tag k="oneway" v="yes"/>
  </way>
</osm>`

	net, err := NetXML([]byte(doc))
	if err != nil {
		t.Fatalf("NetXML: %v", err)
	}
	g, err := graph.ImportSUMONetwork(strings.NewReader(string(net)), graph.ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("edges = %d, want 1 for a one-way road", len(g.Edges))
	}
	if len(g.OutgoingEdgeIndices("2")) != 0 {
		t.Error("the reverse direction exists on a one-way road")
	}
}

func TestNetXMLReadsTags(t *testing.T) {
	const doc = `<osm version="0.6">
  <node id="1" lat="-6.20" lon="106.80"/>
  <node id="2" lat="-6.21" lon="106.81"/>
  <way id="9">
    <nd ref="1"/><nd ref="2"/>
    <tag k="highway" v="secondary"/>
    <tag k="maxspeed" v="72"/>
    <tag k="lanes" v="3"/>
  </way>
</osm>`

	net, err := NetXML([]byte(doc))
	if err != nil {
		t.Fatalf("NetXML: %v", err)
	}
	g, err := graph.ImportSUMONetwork(strings.NewReader(string(net)), graph.ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	e := g.Edges[0]
	if want := 72 / 3.6; e.SpeedLimitMPS < want-0.01 || e.SpeedLimitMPS > want+0.01 {
		t.Errorf("speed = %v m/s, want %v from maxspeed=72", e.SpeedLimitMPS, want)
	}
	if e.Lanes != 3 {
		t.Errorf("lanes = %d, want 3", e.Lanes)
	}
}

func TestNetXMLRejectsDocumentsWithoutRoads(t *testing.T) {
	const doc = `<osm version="0.6"><node id="1" lat="-6.20" lon="106.80"/></osm>`
	if _, err := NetXML([]byte(doc)); !errors.Is(err, ErrNoRoads) {
		t.Fatalf("err = %v, want ErrNoRoads", err)
	}
}

func TestAreaValidate(t *testing.T) {
	ok := Area{MinLat: -6.22, MinLon: 106.81, MaxLat: -6.20, MaxLon: 106.83}
	if err := ok.Validate(); err != nil {
		t.Errorf("a normal city box was rejected: %v", err)
	}

	cases := []struct {
		name string
		area Area
		want error
	}{
		{"inverted latitude", Area{MinLat: -6.20, MinLon: 106.81, MaxLat: -6.22, MaxLon: 106.83}, nil},
		{"inverted longitude", Area{MinLat: -6.22, MinLon: 106.83, MaxLat: -6.20, MaxLon: 106.81}, nil},
		{"outside the world", Area{MinLat: -100, MinLon: 106.81, MaxLat: -6.20, MaxLon: 106.83}, nil},
		{"too large", Area{MinLat: -7, MinLon: 106, MaxLat: -5, MaxLon: 108}, ErrAreaTooLarge},
	}
	for _, tc := range cases {
		err := tc.area.Validate()
		if err == nil {
			t.Errorf("%s: accepted, want an error", tc.name)
			continue
		}
		if tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestAreaQueryIncludesRecursion(t *testing.T) {
	a := Area{MinLat: -6.22, MinLon: 106.81, MaxLat: -6.20, MaxLon: 106.83}
	q := a.Query([]string{"primary"})

	for _, want := range []string{"[out:xml]", `way["highway"~"^(primary)(_link)?$"]`, "(._;>;)", "-6.220000,106.810000,-6.200000,106.830000"} {
		if !strings.Contains(q, want) {
			t.Errorf("query is missing %q:\n%s", want, q)
		}
	}
}
