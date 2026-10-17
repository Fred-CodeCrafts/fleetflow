package sim

// Occupancy counts how many vehicles are on each edge right now. Every vehicle
// on an edge slows the others down, so this is the input to the edge speed
// model.
//
// The simulator advances in one goroutine and publishes snapshots for readers, so
// Occupancy itself needs no locking.
type Occupancy struct {
	counts []int
	total  int
}

// NewOccupancy returns zeroed counts for the given number of edges.
func NewOccupancy(edges int) *Occupancy {
	return &Occupancy{counts: make([]int, edges)}
}

// Count returns the number of vehicles on edge e.
func (o *Occupancy) Count(e int) int { return o.counts[e] }

// Total returns the number of vehicles currently on the network.
func (o *Occupancy) Total() int { return o.total }

// Enter records a vehicle joining edge e.
func (o *Occupancy) Enter(e int) {
	o.counts[e]++
	o.total++
}

// Leave records a vehicle leaving edge e. Counting below zero is a bug in the
// caller, so it is ignored rather than corrupting the total.
func (o *Occupancy) Leave(e int) {
	if o.counts[e] <= 0 {
		return
	}
	o.counts[e]--
	o.total--
}

// Reset clears every count.
func (o *Occupancy) Reset() {
	for i := range o.counts {
		o.counts[i] = 0
	}
	o.total = 0
}

// Snapshot returns a copy of the counts, so readers can keep it while the
// simulator moves on.
func (o *Occupancy) Snapshot() []int {
	out := make([]int, len(o.counts))
	copy(out, o.counts)
	return out
}
