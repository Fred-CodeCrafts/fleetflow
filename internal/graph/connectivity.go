package graph

// KeepLargestStronglyConnected prunes the graph to its largest strongly
// connected component and reports how much it removed.
//
// A filtered OpenStreetMap extract is not one connected road network. Dropping
// local streets keeps the arterials but loses the side streets that joined them,
// so a city extract arrives as thousands of islands: in the Jakarta network this
// was built against, only 63% of junctions could be both reached and left again.
//
// That matters because demand is drawn uniformly over the graph's nodes
// (sim.GenerateDemand). A trip whose origin or destination sits on an island has
// no path in one direction, so it is counted as unroutable and contributes
// nothing — a third of the demand silently disappearing. Pruning the islands
// trades map coverage for trips that can actually be driven, and makes
// "unroutable" mean a real routing failure rather than a disconnected fragment.
//
// Connectivity is computed on directed edges: a node that can be reached but not
// left is useless as a destination, and vice versa.
//
// It returns the number of nodes removed. A graph that is already one component
// is returned unchanged, and so is one with no cycle at all — pruning that would
// leave nothing to route on.
func (g *Graph) KeepLargestStronglyConnected() int {
	if len(g.Nodes) == 0 {
		return 0
	}

	keep := largestSCC(g)

	// A graph with no cycle at all has no component worth keeping: pruning it
	// would leave nothing, and "no routable edges" would be a worse description
	// of a one-way road than the road itself. Leave it as given and let the
	// caller route on it.
	if len(keep) < 2 {
		return 0
	}

	removed := len(g.Nodes) - len(keep)
	if removed == 0 {
		return 0
	}

	pruned := New()
	for id, node := range g.Nodes {
		if !keep[id] {
			continue
		}
		// AddNode cannot fail here: the IDs are unique and non-empty by
		// construction, which is what put them in g in the first place.
		_ = pruned.AddNode(node)
	}
	for _, edge := range g.Edges {
		if !keep[edge.From] || !keep[edge.To] {
			continue
		}
		_ = pruned.AddEdge(edge)
	}

	*g = *pruned
	return removed
}

// largestSCC returns the node IDs in the largest strongly connected component,
// using Tarjan's algorithm iteratively so a long road does not overflow the
// goroutine stack.
func largestSCC(g *Graph) map[string]bool {
	index := make(map[string]int, len(g.Nodes))
	low := make(map[string]int, len(g.Nodes))
	onStack := make(map[string]bool, len(g.Nodes))
	var stack []string
	next := 0

	best := map[string]bool{}

	type frame struct {
		node string
		next int // index into g.OutgoingEdgeIndices(node)
	}

	for _, root := range g.sortedNodeIDs() {
		if _, visited := index[root]; visited {
			continue
		}

		index[root] = next
		low[root] = next
		next++
		stack = append(stack, root)
		onStack[root] = true

		work := []frame{{node: root}}
		for len(work) > 0 {
			top := &work[len(work)-1]
			node := top.node
			out := g.OutgoingEdgeIndices(node)

			if top.next < len(out) {
				to := g.Edges[out[top.next]].To
				top.next++

				if _, visited := index[to]; !visited {
					index[to] = next
					low[to] = next
					next++
					stack = append(stack, to)
					onStack[to] = true
					work = append(work, frame{node: to})
				} else if onStack[to] && index[to] < low[node] {
					low[node] = index[to]
				}
				continue
			}

			work = work[:len(work)-1]
			if len(work) > 0 {
				parent := &work[len(work)-1]
				if low[node] < low[parent.node] {
					low[parent.node] = low[node]
				}
			}

			// A node whose low value is its own index roots a component.
			if low[node] != index[node] {
				continue
			}
			component := make(map[string]bool)
			for {
				member := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[member] = false
				component[member] = true
				if member == node {
					break
				}
			}
			if len(component) > len(best) {
				best = component
			}
		}
	}
	return best
}

// sortedNodeIDs returns the node IDs in a stable order, so the traversal — and
// therefore which component wins a tie — does not depend on map iteration.
func (g *Graph) sortedNodeIDs() []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	// Insertion sort: a graph has thousands of nodes, and this runs once per
	// import rather than on any hot path.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	return ids
}
