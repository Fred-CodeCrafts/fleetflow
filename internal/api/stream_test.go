package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseEvent is one Server-Sent Event as a client sees it.
type sseEvent struct {
	Event string
	Data  map[string]any
}

// openStream connects to the event stream and returns a reader over it, plus a
// function that closes it early. The connection is also closed when the test ends,
// so a test that fails part way through does not leave a stream open.
func openStream(t *testing.T, s *Server) (*bufio.Reader, context.CancelFunc) {
	t.Helper()

	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream", nil)
	if err != nil {
		t.Fatalf("failed to build the stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to open the stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	return bufio.NewReader(resp.Body), cancel
}

// waitForMovement reads vehicle frames until the run's clock has moved, which is
// what a viewer sees while a run is going.
func waitForMovement(t *testing.T, r *bufio.Reader) float64 {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event, ok := readEvent(t, r, "vehicles")
		if !ok {
			t.Fatal("the stream ended before the run moved")
		}
		if now, _ := event.Data["now_s"].(float64); now > 0 {
			return now
		}
	}
	t.Fatal("the run never moved while a client was watching")
	return 0
}

// readEvent reads events until it finds one named want, or the stream ends. It
// is how a test skips the frames it does not care about without depending on the
// exact order they arrive in.
func readEvent(t *testing.T, r *bufio.Reader, want string) (sseEvent, bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event, ok := nextEvent(t, r)
		if !ok {
			return sseEvent{}, false
		}
		if event.Event == want {
			return event, true
		}
	}
	return sseEvent{}, false
}

// nextEvent reads one event, skipping comments and blank lines.
func nextEvent(t *testing.T, r *bufio.Reader) (sseEvent, bool) {
	t.Helper()

	var event sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return sseEvent{}, false
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case strings.HasPrefix(line, ":"):
			// A comment, which is what the keep-alive frame is.
		case strings.HasPrefix(line, "event: "):
			event.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			raw := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(raw), &event.Data); err != nil {
				t.Fatalf("event %q carries data that is not JSON: %v", event.Event, err)
			}
		case line == "":
			if event.Event != "" {
				return event, true
			}
		}
	}
}

// vehiclesOf reads the next vehicles event and returns its list.
func vehiclesOf(t *testing.T, r *bufio.Reader) []map[string]any {
	t.Helper()

	event, ok := readEvent(t, r, "vehicles")
	if !ok {
		t.Fatal("no vehicles event arrived")
	}
	raw, ok := event.Data["vehicles"].([]any)
	if !ok {
		t.Fatalf("vehicles event carries no list: %v", event.Data)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		vehicle, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("vehicle entry is %T, want an object", entry)
		}
		out = append(out, vehicle)
	}
	return out
}

func TestStream_OpensWithEveryEventKind(t *testing.T) {
	s := simServer(t)
	r, _ := openStream(t, s)

	// A client that has just connected is given the whole picture at once, rather
	// than having to wait an interval for each part of it.
	first, ok := nextEvent(t, r)
	if !ok || first.Event != "metrics" {
		t.Fatalf("first event = %q, want metrics", first.Event)
	}
	object(t, first.Data, "engine")

	second, ok := nextEvent(t, r)
	if !ok || second.Event != "edges" {
		t.Fatalf("second event = %q, want edges", second.Event)
	}
	edges, ok := second.Data["edges"].([]any)
	if !ok {
		t.Fatalf("edges event carries no list: %v", second.Data)
	}
	if got, want := len(edges), len(testGraph(t).Edges); got != want {
		t.Errorf("edges event carries %d edges, want every edge of the graph (%d)", got, want)
	}
	edge, _ := edges[0].(map[string]any)
	if _, ok := edge["free_flow_s"]; !ok {
		t.Errorf("an edge carries no free-flow time: %v", edge)
	}

	third, ok := nextEvent(t, r)
	if !ok || third.Event != "vehicles" {
		t.Fatalf("third event = %q, want vehicles", third.Event)
	}
	if running, _ := third.Data["running"].(bool); running {
		t.Error("running = true with no run going")
	}
}

func TestStream_LabelsEveryVehicleSimulatedOrLive(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":4,"vehicles":6,"fleet_share":0.5,"window_s":600}`)

	r, _ := openStream(t, s)

	// Every simulated vehicle carries the arm it belongs to and the label that
	// says it is simulated. Both arms are streamed, because both panels are drawn.
	simulated := vehiclesOf(t, r)
	if len(simulated) == 0 {
		t.Fatal("a run is going but the vehicles event is empty")
	}

	arms := map[string]int{}
	for _, v := range simulated {
		if v["kind"] != kindSimulated {
			t.Errorf("vehicle %v has kind %v, want %s", v["id"], v["kind"], kindSimulated)
		}
		arm, _ := v["arm"].(string)
		if arm != armBaseline && arm != armFleetFlow {
			t.Errorf("vehicle %v has arm %q, want one of the two panels", v["id"], arm)
		}
		arms[arm]++
		if lat, _ := v["lat"].(float64); lat == 0 {
			t.Errorf("vehicle %v has no latitude", v["id"])
		}
		if state, _ := v["state"].(string); state != "pending" && state != "running" {
			t.Errorf("vehicle %v has state %q, want pending or running", v["id"], state)
		}
	}
	for _, arm := range []string{armBaseline, armFleetFlow} {
		if arms[arm] == 0 {
			t.Errorf("no vehicle was streamed for the %s arm", arm)
		}
	}

	// A real vehicle that reports over the public API appears as live traffic, and
	// is not attributed to either arm of the comparison.
	if rec := postJSON(t, s, "/api/reports",
		`{"vehicle_id":"truck-9","edge_id":"e_AB","speed_mps":8.5}`); rec.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want 202: %s", rec.Code, rec.Body)
	}

	for _, v := range vehiclesOf(t, r) {
		if v["id"] != "truck-9" {
			continue
		}
		if v["kind"] != kindLive {
			t.Errorf("a reported vehicle has kind %v, want %s", v["kind"], kindLive)
		}
		if arm, ok := v["arm"]; ok {
			t.Errorf("live vehicle carries arm %v, but it is in neither panel", arm)
		}
		if v["edge_id"] != "e_AB" {
			t.Errorf("live vehicle reports edge %v, want the edge it reported on", v["edge_id"])
		}
		if speed, _ := v["speed_mps"].(float64); speed != 8.5 {
			t.Errorf("live vehicle speed = %v, want the reported 8.5", speed)
		}
		return
	}
	t.Error("a vehicle that reported over the API never appeared on the stream")
}

// The stream is what a run is tied to (T44), so it has to be countable.
func TestStream_CountsItsOpenClients(t *testing.T) {
	s := simServer(t)
	before := s.streams.count()
	r, _ := openStream(t, s)

	// Read one event so the connection is certainly established and counted.
	if _, ok := nextEvent(t, r); !ok {
		t.Fatal("the stream sent nothing")
	}
	if got := s.streams.count(); got != before+1 {
		t.Errorf("streams = %d with one client connected, want %d", got, before+1)
	}
}

// T44's own claim, over a real connection: closing the browser tab stops the
// simulation. The run is paused rather than discarded, so opening the page again
// picks up where it was instead of starting over.
func TestRun_StopsWhenTheLastViewerLeaves(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":3,"vehicles":4,"window_s":0,"step_s":1}`)

	r, close := openStream(t, s)
	if now := waitForMovement(t, r); now <= 0 {
		t.Fatalf("the run moved to %v, want it to have moved while watched", now)
	}

	close()

	// The clock may already be inside a step when the viewer leaves, so it is
	// allowed to finish that one. What it must not do is keep going.
	time.Sleep(4 * tickInterval)
	before := s.currentRun().snapshot().NowS
	time.Sleep(6 * tickInterval)
	after := s.currentRun().snapshot().NowS

	if after != before {
		t.Errorf("NowS went from %v to %v with nobody watching", before, after)
	}
}

func TestStream_WithoutAnEngineIsUnavailable(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stream", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["code"]; got != "engine_unavailable" {
		t.Errorf("code = %v, want engine_unavailable", got)
	}
}

func TestStream_RejectsOtherMethods(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/stream", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
