package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const apiTestNetworkXML = `<?xml version="1.0"?>
<net version="1.20">
  <location netOffset="0,0" convBoundary="0,0,1000,1000" origBoundary="106.80,-6.25,106.82,-6.23" projParameter="+proj=utm +zone=48 +ellps=WGS84"/>
  <junction id="A" type="priority" x="0" y="0"/>
  <junction id="B" type="priority" x="1000" y="1000"/>
  <edge id="AB" from="A" to="B" type="highway.primary"><lane id="AB_0" index="0" speed="13.89" length="1414.2" shape="0,0 500,500 1000,1000"/></edge>
  <edge id="BA" from="B" to="A" type="highway.primary"><lane id="BA_0" index="0" speed="13.89" length="1414.2" shape="1000,1000 500,500 0,0"/></edge>
</net>`

func networkServer(t *testing.T) *Server {
	t.Helper()
	server := simServer(t)
	t.Cleanup(func() {
		server.networkMu.RLock()
		stop := server.stopCustomizer
		server.networkMu.RUnlock()
		if stop != nil {
			stop()
		}
	})
	return server
}

func postNetwork(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/network", strings.NewReader(body))
	request.Header.Set("X-Network-Name", "jakarta.net.xml")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestNetwork_DescribesAndImportsSUMONetwork(t *testing.T) {
	server := networkServer(t)

	before := getJSON(t, server, "/api/network")
	if before.Code != http.StatusOK {
		t.Fatalf("GET /api/network = %d: %s", before.Code, before.Body)
	}
	if got := field(t, decodeBody(t, before), "source"); got != "config" {
		t.Errorf("initial source = %q, want config", got)
	}

	rec := postNetwork(t, server, apiTestNetworkXML)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/network = %d: %s", rec.Code, rec.Body)
	}
	body := decodeBody(t, rec)
	if got := field(t, body, "name"); got != "jakarta.net.xml" {
		t.Errorf("network name = %q", got)
	}
	if got := field(t, body, "source"); got != "upload" {
		t.Errorf("source = %q, want upload", got)
	}
	if got := number(t, body, "shaped_edges"); got != 2 {
		t.Errorf("shaped edges = %v, want 2", got)
	}

	graph := decodeBody(t, getJSON(t, server, "/api/graph"))
	edges := list(t, graph, "edges")
	edge := edges[0].(map[string]any)
	if shape, ok := edge["shape"].([]any); !ok || len(shape) != 3 {
		t.Errorf("edge shape = %v, want three geographic points", edge["shape"])
	}
}

func TestNetwork_RejectsUnusableXMLWithSpecificError(t *testing.T) {
	server := networkServer(t)
	rec := postNetwork(t, server, `<not-a-net/>`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid network = %d, want 400: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != codeNetworkNotSumo {
		t.Errorf("error code = %q, want %q", code, codeNetworkNotSumo)
	}
}

func TestNetwork_RefusesReplacementWhileRunIsClaimed(t *testing.T) {
	server := simServer(t)
	if rec := postJSON(t, server, "/api/sim/start", `{"vehicles":4,"window_s":600}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d, want 202: %s", rec.Code, rec.Body)
	}

	rec := postNetwork(t, server, apiTestNetworkXML)
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST network during a run = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != codeNetworkBusy {
		t.Errorf("error code = %q, want %q", code, codeNetworkBusy)
	}
	server.stopRun(context.Background())
}
