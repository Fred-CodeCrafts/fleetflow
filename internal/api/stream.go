package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
)

// The event stream (DESIGN 3.5, 3.6). One connection carries three kinds of
// event, each at the cadence its contents change at:
//
//	event: vehicles  every streamVehicleInterval  positions, simulated and live
//	event: edges     every streamSummaryInterval  congestion, one entry per edge
//	event: metrics   every streamSummaryInterval  the body of GET /api/metrics
//
// A vehicle frame is small and moves constantly; congestion and metrics move
// slowly, so sending them at the same rate as positions would spend most of the
// bandwidth repeating numbers that had not changed.
const (
	// streamVehicleInterval is how often positions are sent. It is five frames a
	// second: enough for movement to read as movement, and far below the run
	// clock's twenty steps a second, which nothing visual needs.
	streamVehicleInterval = 200 * time.Millisecond
	// streamSummaryInterval is how often congestion and metrics are sent.
	streamSummaryInterval = time.Second
	// streamKeepAlive is how often a comment frame goes out when nothing else
	// has. An idle connection is what proxies and load balancers close first, and
	// a closed stream the client has to notice and re-open is a worse experience
	// than a few bytes a minute.
	streamKeepAlive = 15 * time.Second
)

// Kinds and arms as they appear in a frame. A client labels what it draws from
// these, rather than assuming.
const (
	// kindSimulated is a vehicle belonging to a comparison run.
	kindSimulated = "simulated"
	// kindLive is a real vehicle that has reported over the public API. Its
	// position is the edge it last reported on, not a GPS point: the API carries
	// speeds, not coordinates (T43).
	kindLive = "live"

	// armBaseline is the static arm's panel.
	armBaseline = "baseline"
	// armFleetFlow is the dynamic arm's panel.
	armFleetFlow = "fleetflow"
)

// streamVehicle is one vehicle, simulated or live, placed on the map.
type streamVehicle struct {
	ID string `json:"id"`
	// Kind is kindSimulated or kindLive, and is what the dashboard labels the
	// marker with.
	Kind string `json:"kind"`
	// Arm names the panel the vehicle belongs to. Live vehicles have none: they
	// are not part of either arm of a comparison.
	Arm string `json:"arm,omitempty"`
	// Fleet marks a simulated vehicle that chooses its own route, as opposed to
	// background traffic that drives free flow.
	Fleet bool `json:"fleet,omitempty"`

	// State is pending or running for a simulated vehicle. A finished vehicle is
	// not sent: it is no longer on the road, and the counts carry it.
	State string `json:"state"`

	EdgeID string  `json:"edge_id,omitempty"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	// SpeedMPS is the speed of the edge the vehicle is on, from the same
	// occupancy model the simulation drives with.
	SpeedMPS float64 `json:"speed_mps,omitempty"`
	// AgeS is how long ago a live vehicle reported. It is zero for simulated
	// vehicles, whose positions are as current as the frame.
	AgeS float64 `json:"age_s,omitempty"`
}

// streamEdge is one edge's congestion: what it costs now against what it would
// cost empty. Both figures are in simulated seconds for a run and in modelled
// seconds otherwise; the ratio is what a client colours by.
type streamEdge struct {
	EdgeID    string  `json:"edge_id"`
	CostS     float64 `json:"cost_s"`
	FreeFlowS float64 `json:"free_flow_s"`
}

// streamVehiclesFrame is the body of a vehicles event.
type streamVehiclesFrame struct {
	// NowS is the simulated time of the run, or 0 when none is going.
	NowS     float64         `json:"now_s"`
	Running  bool            `json:"running"`
	Vehicles []streamVehicle `json:"vehicles"`
}

// streamEdgesFrame is the body of an edges event. Every edge is sent rather than
// only the congested ones, so one frame is enough to paint the whole map and a
// client that joins late needs no history. The graph this project ships has 120
// edges; a country-sized one would need deltas, and that is noted in DESIGN.
type streamEdgesFrame struct {
	// MetricVersion is the snapshot these costs came from, so a client can tell
	// whether what it is drawing is the current weights.
	MetricVersion uint64       `json:"metric_version"`
	Edges         []streamEdge `json:"edges"`
}

// streamClients counts the connections holding a stream open. It is what lets a
// run be tied to somebody watching it (T44), rather than burning CPU with the
// tab closed.
type streamClients struct {
	n atomic.Int64
}

// add records a new stream.
func (c *streamClients) add() { c.n.Add(1) }

// remove records a stream that has ended.
func (c *streamClients) remove() { c.n.Add(-1) }

// count reports how many streams are open.
func (c *streamClients) count() int64 { return c.n.Load() }

// stream wraps the event stream: it takes the public rate limit but deliberately
// not the request deadline every other endpoint gets. A response that is meant to
// stay open must not be given a deadline, or every stream would be severed when
// the deadline passed.
func (s *Server) stream(h http.HandlerFunc) http.Handler {
	return s.rateLimited(h)
}

// handleStream serves GET /api/stream: vehicle positions, edge congestion and
// metrics, as Server-Sent Events, until the client goes away.
//
// It holds no lock on the simulation. Positions come from the snapshot the run's
// clock publishes after every step, congestion comes from the engine's immutable
// metric, and the live register has a lock of its own, so a slow client cannot
// slow a run down and a run cannot make a frame inconsistent.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if s.eng == nil {
		writeError(w, r, errNoEngine)
		return
	}

	// The stream is reached through the response controller rather than a direct
	// http.Flusher assertion, because the logging middleware wraps every response,
	// and a wrapper that embedded http.ResponseWriter would hide the flusher from
	// the handler that needs it. The controller follows Unwrap to the real one.
	controller := http.NewResponseController(w)

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	// A buffering proxy would hold frames until it had a full buffer, which for a
	// stream is never. This is the header that asks the common ones not to.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := controller.Flush(); err != nil {
		// A writer that cannot flush would turn the stream into one buffered
		// response delivered at the end, or never. Nothing useful can be said on a
		// response that has already started, so it ends here; through net/http,
		// which can always flush, this is unreachable.
		s.logger.Warn("event stream cannot be flushed", slog.String("error", err.Error()))
		return
	}

	s.streams.add()
	defer s.streams.remove()

	// Paint immediately rather than after the first interval, so opening the page
	// shows the current state at once.
	if err := s.writeStreamFrame(w, controller); err != nil {
		return
	}

	positions := time.NewTicker(streamVehicleInterval)
	defer positions.Stop()
	summaries := time.NewTicker(streamSummaryInterval)
	defer summaries.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-positions.C:
			if err := s.writeEvent(w, controller, "vehicles", s.vehiclesFrame()); err != nil {
				return
			}
		case <-summaries.C:
			// Congestion first, then the numbers that describe it.
			if err := s.writeEvent(w, controller, "edges", s.edgesFrame()); err != nil {
				return
			}
			if err := s.writeEvent(w, controller, "metrics", s.metricsFrame()); err != nil {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

// writeStreamFrame sends one of each event, which is how a connection starts.
func (s *Server) writeStreamFrame(w http.ResponseWriter, controller *http.ResponseController) error {
	if err := s.writeEvent(w, controller, "metrics", s.metricsFrame()); err != nil {
		return err
	}
	if err := s.writeEvent(w, controller, "edges", s.edgesFrame()); err != nil {
		return err
	}
	return s.writeEvent(w, controller, "vehicles", s.vehiclesFrame())
}

// writeEvent sends one Server-Sent Event and flushes it, so the client sees it
// now rather than when some buffer decides to hand it over. Encoding is JSON,
// which never contains a bare newline, so a payload cannot break the framing.
func (s *Server) writeEvent(w http.ResponseWriter, controller *http.ResponseController, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		// The payloads here are plain structs of numbers and strings, so this is
		// unreachable; returning it keeps the signature honest.
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	return controller.Flush()
}

// metricsFrame is the metrics event. It is the whole body of GET /api/metrics,
// so a client parses one shape whether it asks once or streams.
func (s *Server) metricsFrame() metricsResponse {
	return s.metricsResponse()
}

// edgesFrame is the congestion event, from the engine's active snapshot: the
// costs every route query is reading, which is what the map is showing.
func (s *Server) edgesFrame() streamEdgesFrame {
	metric := s.eng.ActiveMetric()
	frame := streamEdgesFrame{MetricVersion: metric.Version(), Edges: make([]streamEdge, 0, len(s.graph.Edges))}

	for i, edge := range s.graph.Edges {
		frame.Edges = append(frame.Edges, streamEdge{
			EdgeID:    edge.ID,
			CostS:     metric.Cost(i),
			FreeFlowS: edge.FreeFlowS,
		})
	}
	return frame
}

// vehiclesFrame is the positions event: the vehicles of both arms of a run, and
// every real vehicle that has reported recently, each labelled with where it came
// from.
func (s *Server) vehiclesFrame() streamVehiclesFrame {
	frame := streamVehiclesFrame{Vehicles: []streamVehicle{}}

	if live := s.currentRun(); live != nil {
		picture := live.snapshot()
		frame.NowS = picture.NowS
		frame.Running = true
		frame.Vehicles = append(frame.Vehicles, picture.Vehicles...)
	}

	for _, v := range s.live.snapshot(time.Now()) {
		vehicle := streamVehicle{
			ID:     v.ID,
			Kind:   kindLive,
			State:  "running",
			EdgeID: v.EdgeID,
			AgeS:   time.Since(v.At).Seconds(),
			// The report carries a speed, and it is the speed the weights were
			// told about, so the map shows what the model believes.
			SpeedMPS: v.SpeedMPS,
		}
		if _, idx, ok := s.graph.EdgeByID(v.EdgeID); ok {
			vehicle.Lat, vehicle.Lon = edgeMidpoint(s.graph, idx, 0.5)
		}
		frame.Vehicles = append(frame.Vehicles, vehicle)
	}

	return frame
}

// runSnapshot is one published picture of a run: what both arms report, and where
// every vehicle is. The clock builds it while it holds the simulation and readers
// take it whole, so no reader ever touches a vehicle while the clock is moving it.
type runSnapshot struct {
	Metrics sim.Comparison
	// NowS is the simulated time the picture was taken at.
	NowS     float64
	Vehicles []streamVehicle
}

// picture builds the run's current picture. It is called by the clock goroutine
// that owns the simulators, and by nothing else.
func (r *run) picture() runSnapshot {
	return runSnapshot{
		Metrics: sim.Comparison{
			Baseline:  r.baseline.Collect(),
			FleetFlow: r.fleetflow.Collect(),
		},
		NowS:     r.fleetflow.NowS(),
		Vehicles: r.vehicles(),
	}
}

// vehicles places every vehicle of both arms that is still to be drawn: one that
// has not left yet sits at its origin, and one on the road sits along its edge.
// Finished and unroutable vehicles are left out; the counts in the metrics frame
// are what report them.
func (r *run) vehicles() []streamVehicle {
	placed := make([]streamVehicle, 0, 2*len(r.fleetflow.Vehicles()))
	placed = append(placed, r.armVehicles(armBaseline, r.baseline)...)
	return append(placed, r.armVehicles(armFleetFlow, r.fleetflow)...)
}

// armVehicles places one arm's vehicles.
func (r *run) armVehicles(arm string, s *sim.Sim) []streamVehicle {
	vehicles := s.Vehicles()
	placed := make([]streamVehicle, 0, len(vehicles))
	occupancy := s.Occupancy()

	for _, v := range vehicles {
		state := v.State()
		if state != sim.VehiclePending && state != sim.VehicleRunning {
			continue
		}

		vehicle := streamVehicle{
			ID:    v.ID(),
			Kind:  kindSimulated,
			Arm:   arm,
			Fleet: v.IsFleet(),
			State: state.String(),
		}

		edgeIndex := v.CurrentEdge()
		if edgeIndex < 0 {
			vehicle.Lat, vehicle.Lon = nodePosition(r.g, v.Origin())
		} else {
			edge := r.g.Edges[edgeIndex]
			vehicle.EdgeID = edge.ID
			vehicle.Lat, vehicle.Lon = edgeMidpoint(r.g, edgeIndex, v.Progress(edge.LengthM))
			vehicle.SpeedMPS = sim.EdgeSpeed(edge.SpeedLimitMPS, edge.Capacity, occupancy.Count(edgeIndex))
		}

		placed = append(placed, vehicle)
	}
	return placed
}

// nodePosition returns where a node sits, or zero when the graph does not have
// it. A vehicle whose node is missing is a broken graph, and the run's metrics
// are where that shows; a frame is not the place to fail.
func nodePosition(g *graph.Graph, nodeID string) (lat, lon float64) {
	node, ok := g.Nodes[nodeID]
	if !ok {
		return 0, 0
	}
	return node.Lat, node.Lon
}

// edgeMidpoint returns the point fraction of the way along an edge, which is
// where a vehicle on that edge is drawn. The edges are the graph's own
// coordinates, so the dashboard needs no projection beyond the one it draws with.
func edgeMidpoint(g *graph.Graph, edgeIndex int, fraction float64) (lat, lon float64) {
	if edgeIndex < 0 || edgeIndex >= len(g.Edges) {
		return 0, 0
	}
	edge := g.Edges[edgeIndex]
	from, fromOK := g.Nodes[edge.From]
	to, toOK := g.Nodes[edge.To]
	if !fromOK || !toOK {
		return 0, 0
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return from.Lat + (to.Lat-from.Lat)*fraction, from.Lon + (to.Lon-from.Lon)*fraction
}
