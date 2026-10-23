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
	LoadRatio float64 `json:"load_ratio"`
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
	// wake carries one pending notification whenever the count changes, so a
	// paused run can wait to be watched instead of polling for it.
	wake chan struct{}
}

// newStreamClients returns a counter ready to be used. The channel is buffered by
// one and drained by at most one waiter, so a notification that arrives while a
// run is between checks is not lost, and none of them can block.
func newStreamClients() *streamClients {
	return &streamClients{wake: make(chan struct{}, 1)}
}

// add records a new stream.
func (c *streamClients) add() {
	c.n.Add(1)
	c.notify()
}

// remove records a stream that has ended.
func (c *streamClients) remove() {
	c.n.Add(-1)
	c.notify()
}

// count reports how many streams are open.
func (c *streamClients) count() int64 { return c.n.Load() }

// notify wakes anything waiting on a change, without blocking when nothing is.
func (c *streamClients) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// stream wraps the event stream: it takes the public rate limit but deliberately
// not the request deadline every other endpoint gets. A response that is meant to
// stay open must not be given a deadline, or every stream would be severed when
// the deadline passed.
func (s *Server) stream(h http.HandlerFunc) http.Handler {
	return s.rateLimited(h)
}

// watched reports whether anybody is watching the run. It is what the clock asks
// before it spends a step (T44).
func (s *Server) watched() bool { return s.streams.count() > 0 }

// handleStream serves GET /api/stream: vehicle positions, edge congestion and
// metrics, as Server-Sent Events, until the client goes away.
//
// It holds no lock on the simulation. Positions come from the snapshot the run's
// clock publishes after every step, congestion comes from the engine's immutable
// metric, and the live register has a lock of its own, so a slow client cannot
// slow a run down and a run cannot make a frame inconsistent.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	_, eng := s.networkSnapshot()
	if eng == nil {
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
	_, eng, live := s.networkRunSnapshot()
	return s.metricsResponse(eng, live)
}

// edgesFrame is the congestion event, from the engine's active snapshot: the
// costs every route query is reading, which is what the map is showing.
func (s *Server) edgesFrame() streamEdgesFrame {
	g, eng, live := s.networkRunSnapshot()
	metric := eng.ActiveMetric()
	frame := streamEdgesFrame{MetricVersion: metric.Version(), Edges: make([]streamEdge, 0, len(g.Edges))}
	var loads []int
	if live != nil {
		loads = live.snapshot().FleetFlowLoads
	}

	for i, edge := range g.Edges {
		loadRatio := 0.0
		if i < len(loads) && edge.Capacity > 0 {
			loadRatio = float64(loads[i]) / edge.Capacity
		}
		frame.Edges = append(frame.Edges, streamEdge{
			EdgeID:    edge.ID,
			CostS:     metric.Cost(i),
			FreeFlowS: edge.FreeFlowS,
			LoadRatio: loadRatio,
		})
	}
	return frame
}

// vehiclesFrame is the positions event: the vehicles of both arms of a run, and
// every real vehicle that has reported recently, each labelled with where it came
// from.
func (s *Server) vehiclesFrame() streamVehiclesFrame {
	g, _, live := s.networkRunSnapshot()
	frame := streamVehiclesFrame{Vehicles: []streamVehicle{}}

	if live != nil {
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
		if _, idx, ok := g.EdgeByID(v.EdgeID); ok {
			vehicle.Lat, vehicle.Lon = edgeMidpoint(g, idx, 0.5)
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
	NowS           float64
	Vehicles       []streamVehicle
	FleetFlowLoads []int
}

// picture builds the run's current picture. It is called by the clock goroutine
// that owns the simulators, and by nothing else.
func (r *run) picture() runSnapshot {
	return runSnapshot{
		Metrics: sim.Comparison{
			Baseline:  r.baseline.Collect(),
			FleetFlow: r.fleetflow.Collect(),
		},
		NowS:           r.fleetflow.NowS(),
		Vehicles:       r.vehicles(),
		FleetFlowLoads: r.fleetflow.Occupancy().Snapshot(),
	}
}

// maxDrawnVehicles bounds how many vehicle markers one arm sends in a frame.
//
// A frame carries both arms, so this is half of what reaches a browser. The map
// cannot usefully draw more: at a city zoom a marker is a pixel or two, and six
// thousand of them already overlap. What a run is judged on is the congestion
// picture and the metrics, and both stay exact — only the markers are sampled.
//
// It exists because a 60,000-vehicle run would otherwise put 120,000 records on
// the wire five times a second, which is tens of megabytes a second and a
// browser that cannot keep up. Sampling keeps the frame the same size whatever
// the fleet, so the console behaves the same at 2,000 vehicles and at 60,000.
const maxDrawnVehicles = 3000

// vehicles places every vehicle of both arms that is still to be drawn: one that
// has not left yet sits at its origin, and one on the road sits along its edge.
// Finished and unroutable vehicles are left out; the counts in the metrics frame
// are what report them.
func (r *run) vehicles() []streamVehicle {
	placed := make([]streamVehicle, 0, 2*maxDrawnVehicles)
	placed = append(placed, r.armVehicles(armBaseline, r.baseline)...)
	return append(placed, r.armVehicles(armFleetFlow, r.fleetflow)...)
}

// armVehicles places one arm's vehicles.
//
// When the arm has more vehicles on the road than maxDrawnVehicles, the frame is
// budgeted: the fleet gets at most half the budget, because the fleet is what the
// comparison measures, and the remaining places go to background traffic on an
// even stride. An even stride rather than the first N keeps the sample from being
// every early-departing vehicle, which would draw the fleet bunched at the origin
// instead of moving across the map.
//
// The budget is a hard cap. A fleet larger than half of it is sampled like
// anything else: the metrics carry the true counts, and the map is a picture.
func (r *run) armVehicles(arm string, s *sim.Sim) []streamVehicle {
	vehicles := s.Vehicles()
	occupancy := s.Occupancy()

	// Count what is drawable first, so the stride is decided before placing.
	drawable, drawableFleet := 0, 0
	for _, v := range vehicles {
		state := v.State()
		if state != sim.VehiclePending && state != sim.VehicleRunning {
			continue
		}
		drawable++
		if v.IsFleet() {
			drawableFleet++
		}
	}
	if drawable <= maxDrawnVehicles {
		return r.placeVehicles(arm, s, occupancy, vehicles, 0, 0)
	}

	fleetBudget := drawableFleet
	if fleetBudget > maxDrawnVehicles/2 {
		fleetBudget = maxDrawnVehicles / 2
	}
	backgroundBudget := maxDrawnVehicles - fleetBudget
	backgroundTotal := drawable - drawableFleet
	fleetStride, backgroundStride := 1, 1
	if drawableFleet > fleetBudget && fleetBudget > 0 {
		fleetStride = (drawableFleet + fleetBudget - 1) / fleetBudget
	}
	if backgroundTotal > backgroundBudget && backgroundBudget > 0 {
		backgroundStride = (backgroundTotal + backgroundBudget - 1) / backgroundBudget
	}
	return r.placeVehicles(arm, s, occupancy, vehicles, fleetStride, backgroundStride)
}

// placeVehicles walks an arm's vehicles and places the ones the strides select.
// A stride of zero means "take every vehicle of that kind", which is the case
// when the arm is small enough to draw whole.
func (r *run) placeVehicles(arm string, s *sim.Sim, occupancy *sim.Occupancy, vehicles []*sim.Vehicle, fleetStride, backgroundStride int) []streamVehicle {
	placed := make([]streamVehicle, 0, min(len(vehicles), maxDrawnVehicles))
	fleetSeen, backgroundSeen := 0, 0

	for _, v := range vehicles {
		state := v.State()
		if state != sim.VehiclePending && state != sim.VehicleRunning {
			continue
		}
		if v.IsFleet() {
			fleetSeen++
			if fleetStride > 0 && fleetSeen%fleetStride != 0 {
				continue
			}
		} else {
			backgroundSeen++
			if backgroundStride > 0 && backgroundSeen%backgroundStride != 0 {
				continue
			}
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
