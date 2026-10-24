package graph

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// jakartaPath is the real SUMO extract this importer was built against. It is
// not in this repository (12 MB of OSM-derived data), so the tests that need it
// skip when it is absent rather than failing on a clean clone.
const jakartaPath = `C:\Users\frede\documents\fleet project\sumo-routing\sumo-routing-backend\data\jakarta_sudirman.net.xml`

// TestImportSUMONetwork_RealJakartaExtract is the test that matters: it runs
// the importer over the actual file a user would upload, and asserts the
// properties the dashboard depends on rather than exact counts, which drift
// with every netconvert run.
func TestImportSUMONetwork_RealJakartaExtract(t *testing.T) {
	f, err := os.Open(jakartaPath)
	if err != nil {
		t.Skipf("Jakarta extract not present: %v", err)
	}
	defer f.Close()

	g, err := ImportSUMONetwork(f, ImportOptions{MinLengthM: 1})
	if err != nil {
		t.Fatalf("importing the Jakarta extract: %v", err)
	}

	if len(g.Nodes) < 100 {
		t.Errorf("imported %d nodes, want a city-sized network", len(g.Nodes))
	}
	if len(g.Edges) < 100 {
		t.Errorf("imported %d edges, want a city-sized network", len(g.Edges))
	}

	// Every edge must join two nodes that exist, or routing will fail later in
	// a place that is much harder to diagnose.
	for _, e := range g.Edges {
		if _, ok := g.Nodes[e.From]; !ok {
			t.Fatalf("edge %s references missing from-node %s", e.ID, e.From)
		}
		if _, ok := g.Nodes[e.To]; !ok {
			t.Fatalf("edge %s references missing to-node %s", e.ID, e.To)
		}
		if e.LengthM <= 0 || e.SpeedLimitMPS <= 0 || e.Lanes <= 0 {
			t.Fatalf("edge %s has non-positive length/speed/lanes: %+v", e.ID, e)
		}
	}

	// The Jakarta extract is in UTM zone 48S around 106.8E, -6.2S. If the
	// projection mapping were wrong the coordinates would land in the ocean or
	// another hemisphere, which is exactly the failure a map shows immediately
	// and a unit test otherwise misses.
	latSum, lonSum := 0.0, 0.0
	for _, n := range g.Nodes {
		latSum += n.Lat
		lonSum += n.Lon
	}
	latMean := latSum / float64(len(g.Nodes))
	lonMean := lonSum / float64(len(g.Nodes))

	if latMean < -6.5 || latMean > -5.9 {
		t.Errorf("mean latitude %.4f is not in Jakarta (want about -6.2)", latMean)
	}
	if lonMean < 106.6 || lonMean > 107.0 {
		t.Errorf("mean longitude %.4f is not in Jakarta (want about 106.8)", lonMean)
	}

	// Shape geometry is what makes an imported city look like a city instead of
	// a spider web of straight lines.
	withShape := 0
	for _, e := range g.Edges {
		if len(e.Shape) >= 2 {
			withShape++
		}
	}
	if withShape == 0 {
		t.Error("no edge carried shape geometry, so every road would draw as a straight line")
	}
	t.Logf("imported %d nodes, %d edges, %d with shape", len(g.Nodes), len(g.Edges), withShape)

	// Internal junction edges are turning movements, not roads. The default
	// must exclude them or the map doubles in size with lines nobody drives.
	for _, e := range g.Edges {
		if strings.HasPrefix(e.ID, ":") {
			t.Fatalf("internal edge %s was kept by default", e.ID)
		}
	}
}

// TestImportSUMONetwork_MinimalNetwork pins the format handling on a file small
// enough to read in the test.
func TestImportSUMONetwork_MinimalNetwork(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<net version="1.20">
  <location netOffset="0,0" convBoundary="0,0,1000,1000" origBoundary="106.80,-6.25,106.82,-6.23" projParameter="+proj=utm +zone=48 +ellps=WGS84"/>
  <junction id="A" type="priority" x="0" y="0"/>
  <junction id="B" type="priority" x="1000" y="1000"/>
  <edge id="AB" from="A" to="B" type="highway.primary">
    <lane id="AB_0" index="0" speed="13.89" length="1414.2" shape="0,0 500,500 1000,1000"/>
  </edge>
  <edge id="BA" from="B" to="A" type="highway.primary">
    <lane id="BA_0" index="0" speed="13.89" length="1414.2" shape="1000,1000 500,500 0,0"/>
  </edge>
</net>`

	g, err := ImportSUMONetwork(strings.NewReader(doc), ImportOptions{})
	if err != nil {
		t.Fatalf("importing: %v", err)
	}

	if len(g.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("got %d edges, want 2", len(g.Edges))
	}

	// The corners of the box map to the corners of the geographic box.
	a := g.Nodes["A"]
	if a.Lat < -6.2501 || a.Lat > -6.2499 {
		t.Errorf("node A latitude = %f, want -6.25", a.Lat)
	}
	if a.Lon < 106.7999 || a.Lon > 106.8001 {
		t.Errorf("node A longitude = %f, want 106.80", a.Lon)
	}
	b := g.Nodes["B"]
	if b.Lat < -6.2301 || b.Lat > -6.2299 {
		t.Errorf("node B latitude = %f, want -6.23", b.Lat)
	}
	if b.Lon < 106.8199 || b.Lon > 106.8201 {
		t.Errorf("node B longitude = %f, want 106.82", b.Lon)
	}

	// Derived values are computed on insert, so an imported edge routes.
	e, _, ok := g.EdgeByID("AB")
	if !ok {
		t.Fatal("edge AB missing")
	}
	if e.FreeFlowS <= 0 {
		t.Errorf("edge AB free-flow time = %f, want positive", e.FreeFlowS)
	}
	if e.Capacity <= 0 {
		t.Errorf("edge AB capacity = %f, want positive", e.Capacity)
	}
	if len(e.Shape) != 3 {
		t.Errorf("edge AB shape has %d points, want 3", len(e.Shape))
	}
}

// TestImportSUMONetwork_Rejects pins the three ways a file can be unusable, so
// the handler can answer each with a different status.
func TestImportSUMONetwork_Rejects(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want error
	}{
		{
			name: "not xml",
			doc:  "this is not a network",
			want: ErrNotSUMONetwork,
		},
		{
			name: "wrong root element",
			doc:  `<osm><node id="1"/></osm>`,
			want: ErrNotSUMONetwork,
		},
		{
			name: "no projection",
			doc: `<net>
  <junction id="A" x="0" y="0"/>
  <junction id="B" x="10" y="10"/>
  <edge id="AB" from="A" to="B"><lane id="AB_0" speed="10" length="10"/></edge>
</net>`,
			want: ErrMissingProjection,
		},
		{
			name: "no routable edges",
			doc: `<net>
  <location netOffset="0,0" convBoundary="0,0,10,10" origBoundary="106.8,-6.25,106.81,-6.24"/>
  <junction id="A" x="0" y="0"/>
</net>`,
			want: ErrNoRoutableEdges,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportSUMONetwork(strings.NewReader(tc.doc), ImportOptions{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// TestImportSUMONetwork_DropsInternalAndShortEdges checks the two filters that
// keep an imported network to the roads a vehicle actually drives.
func TestImportSUMONetwork_DropsInternalAndShortEdges(t *testing.T) {
	const doc = `<net>
  <location netOffset="0,0" convBoundary="0,0,100,100" origBoundary="106.80,-6.25,106.81,-6.24"/>
  <junction id="A" x="0" y="0"/>
  <junction id="B" x="100" y="100"/>
  <edge id=":A_0" from="A" to="B" function="internal">
    <lane id=":A_0_0" speed="10" length="50"/>
  </edge>
  <edge id="stub" from="A" to="B">
    <lane id="stub_0" speed="10" length="0.2"/>
  </edge>
  <edge id="real" from="A" to="B">
    <lane id="real_0" speed="10" length="140"/>
  </edge>
</net>`

	g, err := ImportSUMONetwork(strings.NewReader(doc), ImportOptions{MinLengthM: 1})
	if err != nil {
		t.Fatalf("importing: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("got %d edges, want only the real one: %v", len(g.Edges), edgeIDs(g))
	}
	if _, _, ok := g.EdgeByID("real"); !ok {
		t.Errorf("the real edge was dropped; kept %v", edgeIDs(g))
	}
}

// TestImportSUMONetwork_KeepsInternalWhenAsked proves the option is live rather
// than decorative.
func TestImportSUMONetwork_KeepsInternalWhenAsked(t *testing.T) {
	const doc = `<net>
  <location netOffset="0,0" convBoundary="0,0,100,100" origBoundary="106.80,-6.25,106.81,-6.24"/>
  <junction id="A" x="0" y="0"/>
  <junction id="B" x="100" y="100"/>
  <edge id=":A_0" from="A" to="B" function="internal">
    <lane id=":A_0_0" speed="10" length="50"/>
  </edge>
</net>`

	g, err := ImportSUMONetwork(strings.NewReader(doc), ImportOptions{KeepInternalEdges: true})
	if err != nil {
		t.Fatalf("importing: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("got %d edges, want the internal one kept", len(g.Edges))
	}
}

// edgeIDs is a test helper for readable failure messages.
func edgeIDs(g *Graph) []string {
	ids := make([]string, len(g.Edges))
	for i, e := range g.Edges {
		ids[i] = e.ID
	}
	return ids
}
