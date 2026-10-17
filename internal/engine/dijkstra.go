package engine

import (
	"container/heap"
	"errors"
	"fmt"

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

// dijkstraRoute finds the least-cost path from fromID to toID using the edge
// costs in m. Costs must be non-negative, which the cost function guarantees.
// It is reached through the Router interface.
func dijkstraRoute(g *graph.Graph, m *Metric, fromID, toID string) (*Path, error) {
	if _, ok := g.Nodes[fromID]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, fromID)
	}
	if _, ok := g.Nodes[toID]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, toID)
	}
	if fromID == toID {
		return &Path{}, nil
	}

	dist := map[string]float64{fromID: 0}
	// prevEdge[to] is the index of the edge used to reach that node.
	prevEdge := make(map[string]int, len(g.Nodes))
	visited := make(map[string]bool, len(g.Nodes))

	frontier := &priorityQueue{{nodeID: fromID, dist: 0}}
	heap.Init(frontier)

	for frontier.Len() > 0 {
		current := heap.Pop(frontier).(*queueItem)
		if visited[current.nodeID] {
			continue // a shorter route to this node was already settled
		}
		visited[current.nodeID] = true
		if current.nodeID == toID {
			break
		}

		for _, edgeIndex := range g.OutgoingEdgeIndices(current.nodeID) {
			neighbour := g.Edges[edgeIndex].To
			candidate := current.dist + m.Cost(edgeIndex)
			if best, seen := dist[neighbour]; seen && best <= candidate {
				continue
			}
			dist[neighbour] = candidate
			prevEdge[neighbour] = edgeIndex
			heap.Push(frontier, &queueItem{nodeID: neighbour, dist: candidate})
		}
	}

	total, reachable := dist[toID]
	if !reachable {
		return nil, fmt.Errorf("%w: %s -> %s", ErrNoPath, fromID, toID)
	}

	edges := make([]int, 0, 8)
	for node := toID; node != fromID; {
		edgeIndex, ok := prevEdge[node]
		if !ok {
			return nil, fmt.Errorf("%w: %s -> %s", ErrNoPath, fromID, toID)
		}
		edges = append(edges, edgeIndex)
		node = g.Edges[edgeIndex].From
	}
	reverseInts(edges)

	return &Path{Edges: edges, CostS: total}, nil
}

// reverseInts reverses s in place.
func reverseInts(s []int) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// queueItem is one node in the Dijkstra frontier.
type queueItem struct {
	nodeID string
	dist   float64
}

// priorityQueue is a min-heap of frontier nodes, ordered by distance.
type priorityQueue []*queueItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].dist < pq[j].dist }
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
