package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testOSM is a two-road area: one primary running through a junction, one
// secondary meeting it there.
const testOSM = `<?xml version="1.0" encoding="UTF-8"?>
<osm version="0.6">
  <node id="1" lat="-6.2000" lon="106.8000"/>
  <node id="2" lat="-6.2000" lon="106.8010"/>
  <node id="3" lat="-6.2000" lon="106.8020"/>
  <node id="4" lat="-6.1990" lon="106.8010"/>
  <way id="100"><nd ref="1"/><nd ref="2"/><nd ref="3"/><tag k="highway" v="primary"/></way>
  <way id="200"><nd ref="2"/><nd ref="4"/><tag k="highway" v="secondary"/></way>
</osm>`

// buildServer starts a server whose area builder talks to a stub Overpass.
func buildServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	srv := NewServer(testGraph(t), WithOverpassEndpoint(upstream.URL))
	api := httptest.NewServer(srv)
	t.Cleanup(api.Close)
	return api
}

// postBuild sends a build request and returns the response.
func postBuild(t *testing.T, base, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(base+"/api/network/build", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/network/build: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestBuildNetworkInstallsAnArea(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testOSM))
	})

	resp := postBuild(t, api.URL,
		`{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803,"install":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body buildResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OSMWays != 2 {
		t.Errorf("osm_ways = %d, want 2", body.OSMWays)
	}
	if body.Bytes == 0 {
		t.Error("bytes = 0, want the generated network size")
	}
	if body.Network.Nodes != 4 {
		t.Errorf("nodes = %d, want 4 (both roads, cut at the junction)", body.Network.Nodes)
	}
	if body.Network.Source != "area" {
		t.Errorf("source = %q, want \"area\"", body.Network.Source)
	}

	// The built network is now the active one, so the read endpoint agrees.
	got, err := http.Get(api.URL + "/api/network")
	if err != nil {
		t.Fatalf("GET /api/network: %v", err)
	}
	defer got.Body.Close()
	var active networkResponse
	if err := json.NewDecoder(got.Body).Decode(&active); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if active.Nodes != 4 || active.Source != "area" {
		t.Errorf("active network = %d nodes from %q, want 4 from \"area\"", active.Nodes, active.Source)
	}
}

func TestBuildNetworkWithoutInstallLeavesTheGraphAlone(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testOSM))
	})
	before := fetchNetwork(t, api.URL)

	resp := postBuild(t, api.URL,
		`{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803,"install":false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body buildResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Network.Source != "generated" {
		t.Errorf("source = %q, want \"generated\"", body.Network.Source)
	}

	after := fetchNetwork(t, api.URL)
	if after.Nodes != before.Nodes || after.Source != before.Source {
		t.Errorf("the active network changed from %d/%q to %d/%q without install",
			before.Nodes, before.Source, after.Nodes, after.Source)
	}
}

func TestBuildNetworkDownloadReturnsTheFile(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testOSM))
	})

	resp := postBuild(t, api.URL,
		`{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803,"install":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("build status = %d", resp.StatusCode)
	}

	dl, err := http.Get(api.URL + "/api/network/build/download")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, want 200", dl.StatusCode)
	}
	if ct := dl.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("content type = %q, want XML", ct)
	}
	if cd := dl.Header.Get("Content-Disposition"); !strings.Contains(cd, ".net.xml") {
		t.Errorf("content disposition = %q, want a .net.xml attachment", cd)
	}

	body := readAll(t, dl.Body)
	if !strings.Contains(body, "<net ") {
		t.Error("the downloaded file is not a SUMO network")
	}
}

func TestBuildNetworkDownloadBeforeAnyBuild(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testOSM))
	})
	resp, err := http.Get(api.URL + "/api/network/build/download")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(readAll(t, resp.Body), codeBuildNotBuilt) {
		t.Errorf("body does not name %q", codeBuildNotBuilt)
	}
}

func TestBuildNetworkRejectsBadAreas(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream was called for an area that should have been refused")
		_, _ = w.Write([]byte(testOSM))
	})

	cases := []struct {
		name string
		body string
		code string
	}{
		{"inverted", `{"min_lat":-6.19,"min_lon":106.80,"max_lat":-6.21,"max_lon":106.81}`, codeBuildAreaInvalid},
		{"too large", `{"min_lat":-7,"min_lon":106,"max_lat":-5,"max_lon":108}`, codeBuildAreaTooLarge},
		{"unknown class", `{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803,"classes":["motorway","goat-track"]}`, codeBuildClassUnknown},
		{"malformed", `{"min_lat":`, codeBuildBodyInvalid},
		{"unknown field", `{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803,"nope":1}`, codeBuildBodyInvalid},
	}
	for _, tc := range cases {
		resp := postBuild(t, api.URL, tc.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, resp.StatusCode)
			continue
		}
		if body := readAll(t, resp.Body); !strings.Contains(body, tc.code) {
			t.Errorf("%s: body = %s, want code %q", tc.name, body, tc.code)
		}
	}
}

func TestBuildNetworkReportsUpstreamFailure(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server at capacity", http.StatusBadGateway)
	})
	resp := postBuild(t, api.URL,
		`{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if body := readAll(t, resp.Body); !strings.Contains(body, codeBuildUpstream) {
		t.Errorf("body = %s, want code %q", body, codeBuildUpstream)
	}
}

func TestBuildNetworkReportsAnEmptyArea(t *testing.T) {
	api := buildServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<osm version="0.6"><node id="1" lat="-6.2" lon="106.8"/></osm>`))
	})
	resp := postBuild(t, api.URL,
		`{"min_lat":-6.201,"min_lon":106.799,"max_lat":-6.198,"max_lon":106.803}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if body := readAll(t, resp.Body); !strings.Contains(body, codeBuildNoRoads) {
		t.Errorf("body = %s, want code %q", body, codeBuildNoRoads)
	}
}

// fetchNetwork reads GET /api/network.
func fetchNetwork(t *testing.T, base string) networkResponse {
	t.Helper()
	resp, err := http.Get(base + "/api/network")
	if err != nil {
		t.Fatalf("GET /api/network: %v", err)
	}
	defer resp.Body.Close()
	var out networkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode /api/network: %v", err)
	}
	return out
}

// readAll reads a response body as a string.
func readAll(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			return sb.String()
		}
	}
}
