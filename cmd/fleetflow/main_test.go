package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGraphImportsSUMOXML(t *testing.T) {
	const network = `<net version="1.20">
  <location netOffset="0,0" convBoundary="0,0,1000,1000" origBoundary="106.80,-6.25,106.82,-6.23" projParameter="+proj=utm +zone=48 +ellps=WGS84"/>
  <junction id="A" type="priority" x="0" y="0"/><junction id="B" type="priority" x="1000" y="1000"/>
  <edge id="AB" from="A" to="B"><lane id="AB_0" index="0" speed="13.89" length="1414.2" shape="0,0 1000,1000"/></edge>
</net>`
	path := filepath.Join(t.TempDir(), "jakarta.net.xml")
	if err := os.WriteFile(path, []byte(network), 0600); err != nil {
		t.Fatal(err)
	}

	g, err := loadGraph(path)
	if err != nil {
		t.Fatalf("loadGraph(%s): %v", path, err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 || len(g.Edges[0].Shape) != 2 {
		t.Fatalf("loaded graph has %d nodes and %d shaped edges; want 2 nodes and one 2-point edge", len(g.Nodes), len(g.Edges))
	}
}
