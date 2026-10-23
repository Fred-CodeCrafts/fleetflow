package engine

import (
	"container/heap"
	"errors"
	"fmt"
	"sync"

	"fleetflow/internal/graph"
)

var (
	// ErrUnknownNode is returned when a query names a node that is not in the graph.
	ErrUnknownNode = errors.New("unknown node")
	// ErrNoPath is returned when the destination cannot be reached from the origin.
	ErrNoPath = errors.New("no path between nodes")
)

// Path is a routing result: the edge indices to walk in order and the total
// cost of the trip in seconds.
type Path struct {
	Edges []int
	CostS float64
}

// dijkstraScratch is the working memory one route query needs, sized once and
// reused.
//
// A query touches a few thousand nodes, and allocating three slices of that size
// per query showed up as a third of the step time in a 60,000-vehicle run. The
// scratch is pooled instead, and `stamp` marks which entries belong to the
// current query so nothing has to be cleared between them: an entry is live only
// when its stamp matches the query's.
type dijkstraScratch struct {
	dist     []float64
	prevEdge []int
	stamp    []uint32
	// settledStamp marks which nodes this query has already closed. It carries a
	// generation like stamp does, rather than being a []bool that would need
	// clearing between queries.
	settledStamp []uint32
	generation   uint32
}

var scratchPool = sync.Pool{New: func() any { return &dijkstraScratch{} }}

// reset prepares the scratch for a graph of n nodes and returns its generation.
func (s *dijkstraScratch) reset(n int) uint32 {
	if cap(s.dist) < n {
		s.dist = make([]float64, n)
		s.prevEdge = make([]int, n)
		s.stamp = make([]uint32, n)
		s.settledStamp = make([]uint32, n)
	}
	s.dist = s.dist[:n]
	s.prevEdge = s.prevEdge[:n]
	s.stamp = s.stamp[:n]
	s.settledStamp = s.settledStamp[:n]

	// Bumping the generation is cheaper than clearing, and the only cost of a
	// wraparound is that every entry is treated as stale once, which is correct.
	s.generation++
	if s.generation == 0 {
		for i := range s.stamp {
			s.stamp[i] = 0
		}
		for i := range s.settledStamp {
			s.settledStamp[i] = 0
		}
		s.generation = 1
	}
	return s.generation
}

// dijkstraRoute finds the least-cost path from fromID to toID using the edge
// costs in m. Costs must be non-negative, which the cost function guarantees.
// It is reached through the Router interface.
//
// It is A* rather than plain Dijkstra: the frontier is ordered by cost so far
// plus a lower bound on the cost still to come. The bound is the great-circle
// distance to the destination divided by the fastest speed any edge in the graph
// allows, which no real route can beat, so the first path popped is still the
// cheapest one — the answer is identical and only the work differs.
//
// On a city network the saving is large because the graph is spread over 30 km
// while trips cross a fraction of it: Dijkstra settles every junction closer to
// the origin than the destination, A* settles the ones between them.
//
// Nodes are addressed by their integer index rather than their ID: the maps this
// used to keep — distances, predecessors, visited — were the largest single cost
// in the simulation, because a city-scale run expands them tens of thousands of
// times a step.
func dijkstraRoute(g *graph.Graph, m *Metric, fromID, toID string) (*Path, error) {
	from, ok := g.NodeIndex(fromID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, fromID)
	}
	to, ok := g.NodeIndex(toID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, toID)
	}
	if from == to {
		return &Path{}, nil
	}

	scratch := scratchPool.Get().(*dijkstraScratch)
	defer scratchPool.Put(scratch)

	gen := scratch.reset(g.NodeCount())
	dist, prevEdge, stamp, settled := scratch.dist, scratch.prevEdge, scratch.stamp, scratch.settledStamp

	toLat, toLon := g.NodeLatLonAt(to)
	maxSpeed := m.MaxSpeed()
	// estimate is the heuristic: straight-line distance to the destination,
	// discounted by how far the graph's coordinates can overstate its roads, and
	// divided by the fastest speed any edge allows. See graph.GeometrySlack.
	slack := m.GeometrySlack()
	estimate := func(node int) float64 {
		if maxSpeed <= 0 || slack <= 0 {
			return 0
		}
		lat, lon := g.NodeLatLonAt(node)
		return graph.MetresBetween(lat, lon, toLat, toLon) / (slack * maxSpeed)
	}

	dist[from] = 0
	stamp[from] = gen

	frontier := &priorityQueue{{node: from, dist: 0, cost: estimate(from)}}
	heap.Init(frontier)

	for frontier.Len() > 0 {
		current := heap.Pop(frontier).(*queueItem)
		if settled[current.node] == gen {
			continue // a shorter route to this node was already settled
		}
		settled[current.node] = gen
		if current.node == to {
			break
		}

		for _, edgeIndex := range g.OutgoingEdgeIndicesByIndex(current.node) {
			neighbour := g.EdgeToIndex(edgeIndex)
			candidate := dist[current.node] + m.Cost(edgeIndex)
			if stamp[neighbour] == gen && dist[neighbour] <= candidate {
				continue
			}
			stamp[neighbour] = gen
			dist[neighbour] = candidate
			prevEdge[neighbour] = edgeIndex
			heap.Push(frontier, &queueItem{node: neighbour, dist: candidate, cost: candidate + estimate(neighbour)})
		}
	}

	if stamp[to] != gen || settled[to] != gen {
		return nil, fmt.Errorf("%w: %s -> %s", ErrNoPath, fromID, toID)
	}

	edges := make([]int, 0, 8)
	for node := to; node != from; {
		edgeIndex := prevEdge[node]
		edges = append(edges, edgeIndex)
		node = g.EdgeFromIndex(edgeIndex)
	}
	reverseInts(edges)

	return &Path{Edges: edges, CostS: dist[to]}, nil
}

// metresBetween was moved to graph.MetresBetween, which both the router and the
// geometry check need.

// reverseInts reverses s in place.
func reverseInts(s []int) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// queueItem is one node in the A* frontier.
type queueItem struct {
	node int
	// dist is the cost from the origin to this node, which is what the answer
	// reports and what relaxations compare against.
	dist float64
	// cost is dist plus the estimate of the cost still to come, which is what
	// orders the frontier.
	cost float64
}

// priorityQueue is a min-heap of frontier nodes, ordered by estimated total cost.
type priorityQueue []*queueItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].cost < pq[j].cost }
func (pq priorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }

func (pq *priorityQueue) Push(x any) {
	*pq = append(*pq, x.(*queueItem))
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	last := old[n-1]
	old[n-1] = nil
	*pq = old[:n-1]
	return last
}
