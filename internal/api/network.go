package api

import (
	"context"
	"errors"
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
)

// maxNetworkUpload bounds an uploaded network. A metropolitan SUMO extract is
// tens of megabytes; past this the graph is too large for a single-instance
// dashboard to draw, and reading it would only delay the refusal.
const maxNetworkUpload = 200 << 20

// networkResponse describes the road network the server is routing over.
//
// It is what the console's Network screen reads to show which map is loaded,
// how big it is and where it came from. The node and edge lists themselves are
// served separately by GET /api/graph, so this stays small.
type networkResponse struct {
	// Name is what the network is called: the source file's name, or the
	// configured path's for the graph loaded at startup.
	Name string `json:"name"`
	// Source says how it got here: "config" for the graph loaded at startup, or
	// "upload" for one imported through this endpoint.
	Source string `json:"source"`
	Nodes  int    `json:"nodes"`
	Edges  int    `json:"edges"`
	// ShapedEdges counts edges carrying geometry, which is what tells a city
	// import apart from the generated grid.
	ShapedEdges int `json:"shaped_edges"`
	// Capacity is the sum of edge capacities, in vehicles: the total the graph
	// can hold before every road is saturated.
	Capacity float64 `json:"capacity"`
	// LengthKM is the total road length, which is the denominator a per-kilometre
	// figure needs.
	LengthKM float64 `json:"length_km"`
	// Bounds is the geographic box the network occupies, for fitting a map.
	Bounds networkBounds `json:"bounds"`
}

// networkBounds is the geographic extent of a network.
type networkBounds struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

// networkOf describes g.
func networkOf(g *graph.Graph, name, source string) networkResponse {
	out := networkResponse{Name: name, Source: source, Nodes: len(g.Nodes), Edges: len(g.Edges)}

	first := true
	for _, n := range g.Nodes {
		if first {
			out.Bounds = networkBounds{MinLat: n.Lat, MaxLat: n.Lat, MinLon: n.Lon, MaxLon: n.Lon}
			first = false
			continue
		}
		if n.Lat < out.Bounds.MinLat {
			out.Bounds.MinLat = n.Lat
		}
		if n.Lat > out.Bounds.MaxLat {
			out.Bounds.MaxLat = n.Lat
		}
		if n.Lon < out.Bounds.MinLon {
			out.Bounds.MinLon = n.Lon
		}
		if n.Lon > out.Bounds.MaxLon {
			out.Bounds.MaxLon = n.Lon
		}
	}
	for _, e := range g.Edges {
		out.Capacity += e.Capacity
		out.LengthKM += e.LengthM / 1000
		if len(e.Shape) >= 2 {
			out.ShapedEdges++
		}
	}
	return out
}

// handleNetwork answers GET /api/network with a description of the loaded
// network.
func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	s.networkMu.RLock()
	g, name, source := s.graph, s.networkName, s.networkSource
	s.networkMu.RUnlock()

	writeJSON(w, http.StatusOK, networkOf(g, name, source))
}

// handleImportNetwork answers POST /api/network with the description of the
// network it just loaded.
//
// The body is a SUMO .net.xml file, sent as the raw request body rather than as
// multipart form data: there is exactly one thing in the request, and a
// multipart wrapper would only add a parser and a boundary to get wrong.
//
// The import is refused while a comparison run is going. A run holds routes as
// edge indices into the graph it started on, so swapping the graph underneath it
// would make every in-flight vehicle reference an edge that no longer exists.
// Refusing is the honest answer; the alternative is a run that silently reports
// nonsense.
func (s *Server) handleImportNetwork(w http.ResponseWriter, r *http.Request) {
	if s.runInUse() {
		writeError(w, r, domain.Conflict(codeNetworkBusy,
			"a comparison run is going or starting: stop it before loading a different network"))
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxNetworkUpload)
	defer body.Close()

	// The importer reads the body once. MinLengthM drops the sub-metre connector
	// stubs netconvert leaves behind, which add nothing to routing.
	g, err := graph.ImportSUMONetwork(body, graph.ImportOptions{MinLengthM: 1, KeepLargestComponent: true})
	if err != nil {
		writeError(w, r, networkImportError(err))
		return
	}

	name := r.Header.Get("X-Network-Name")
	if name == "" {
		name = "uploaded network"
	}

	if err := s.installGraph(g, name, "upload"); err != nil {
		writeError(w, r, err)
		return
	}

	loggerFromContext(r.Context()).Info("loaded a road network",
		"nodes", len(g.Nodes), "edges", len(g.Edges), "name", name)
	writeJSON(w, http.StatusOK, networkOf(g, name, "upload"))
}

// networkImportError maps an importer error to the domain error the transport
// writes. A file that is not a network at all, or one with no usable roads, is
// the caller's mistake; a file with no projection cannot be placed on a map,
// which is also theirs to fix.
func networkImportError(err error) error {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr):
		return domain.Invalid(codeNetworkTooLarge, "uploaded network exceeds the 200 MiB limit")
	case errors.Is(err, graph.ErrNotSUMONetwork):
		return domain.Invalid(codeNetworkNotSumo,
			"the file is not a SUMO network: expected a <net> root element from netconvert")
	case errors.Is(err, graph.ErrNoRoutableEdges):
		return domain.Invalid(codeNetworkNoRoads,
			"the network has no routable road edges")
	case errors.Is(err, graph.ErrMissingProjection):
		return domain.Invalid(codeNetworkNoProjection,
			"the network has no usable projection, so its coordinates cannot be placed on a map")
	case errors.Is(err, graph.ErrTooManyElements):
		return domain.Invalid(codeNetworkTooLarge,
			"the network has more junctions or edges than this service accepts")
	default:
		return domain.Invalid(codeNetworkUnreadable, "could not read the network: %v", err)
	}
}

// installGraph swaps the graph the server routes over, together with the engines
// built on it, and returns once readers can see the new one.
//
// It is a separate method because two callers need it: the import endpoint and,
// in a later step, any other source of a network. The locking order is fixed:
// networkMu guards the graph, the engines and the name together, so a reader
// can never see the new graph with the old engine.
func (s *Server) installGraph(g *graph.Graph, name, source string) error {
	eng, err := engineForBuilds(g)
	if err != nil {
		return err
	}

	s.networkMu.Lock()
	defer s.networkMu.Unlock()
	if s.runInUse() {
		return domain.Conflict(codeNetworkBusy,
			"a comparison run is going or starting: stop it before loading a different network")
	}

	old := s.eng
	oldStop := s.stopCustomizer

	s.graph, s.eng = g, eng
	s.networkName, s.networkSource = name, source

	// The replacement engine needs its own customizer, or reports would arrive
	// and nothing would ever publish a new metric.
	ctx, stop := context.WithCancel(context.Background())
	s.stopCustomizer = stop
	go eng.Run(ctx)

	// The old customizer is stopped after the swap, so no reader ever sees a
	// goroutine writing to a metric the server has stopped publishing.
	if old != nil && oldStop != nil {
		oldStop()
	}
	return nil
}

// networkSnapshot returns the graph and engine that were installed together.
// The graph is immutable after construction, and the engine owns its own
// synchronized state, so callers can safely use the returned pair after unlock.
func (s *Server) networkSnapshot() (*graph.Graph, *engine.Engine) {
	s.networkMu.RLock()
	g, eng := s.graph, s.eng
	s.networkMu.RUnlock()
	return g, eng
}

func (s *Server) networkRunSnapshot() (*graph.Graph, *engine.Engine, *run) {
	s.networkMu.RLock()
	g, eng := s.graph, s.eng
	live := s.currentRun()
	s.networkMu.RUnlock()
	return g, eng, live
}

// runInUse reports a running run or a start that has claimed the run slot but
// has not installed its run yet.
func (s *Server) runInUse() bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.run != nil || s.claiming
}

// codeNetworkBusy is reported when a network is loaded while a run is going.
const codeNetworkBusy = "network_busy"

// Network import error codes. They travel in API error bodies, so they are part
// of the public contract and must stay stable.
const (
	codeNetworkNotSumo      = "network_not_sumo"
	codeNetworkNoRoads      = "network_no_roads"
	codeNetworkNoProjection = "network_no_projection"
	codeNetworkTooLarge     = "network_too_large"
	codeNetworkUnreadable   = "network_unreadable"
)
