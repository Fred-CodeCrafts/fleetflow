package sim

import "testing"

func TestOccupancy_TracksEnterAndLeave(t *testing.T) {
	o := NewOccupancy(3)

	o.Enter(1)
	o.Enter(1)
	o.Enter(2)

	if got := o.Count(1); got != 2 {
		t.Errorf("Count(1) = %d, want 2", got)
	}
	if got := o.Count(2); got != 1 {
		t.Errorf("Count(2) = %d, want 1", got)
	}
	if got := o.Count(0); got != 0 {
		t.Errorf("Count(0) = %d, want 0", got)
	}
	if got := o.Total(); got != 3 {
		t.Errorf("Total() = %d, want 3", got)
	}

	o.Leave(1)
	if got := o.Count(1); got != 1 {
		t.Errorf("Count(1) = %d after Leave, want 1", got)
	}
	if got := o.Total(); got != 2 {
		t.Errorf("Total() = %d after Leave, want 2", got)
	}
}

func TestOccupancy_LeaveOnAnEmptyEdgeIsIgnored(t *testing.T) {
	o := NewOccupancy(2)

	o.Leave(0) // must not push the count or the total below zero

	if got := o.Count(0); got != 0 {
		t.Errorf("Count(0) = %d, want 0", got)
	}
	if got := o.Total(); got != 0 {
		t.Errorf("Total() = %d, want 0", got)
	}
}

func TestOccupancy_ResetClearsEverything(t *testing.T) {
	o := NewOccupancy(2)
	o.Enter(0)
	o.Enter(1)
	o.Enter(1)

	o.Reset()

	for i := 0; i < 2; i++ {
		if got := o.Count(i); got != 0 {
			t.Errorf("Count(%d) = %d after reset, want 0", i, got)
		}
	}
	if got := o.Total(); got != 0 {
		t.Errorf("Total() = %d after reset, want 0", got)
	}
}

func TestOccupancy_SnapshotIsACopy(t *testing.T) {
	o := NewOccupancy(2)
	o.Enter(0)

	snap := o.Snapshot()
	o.Enter(0)

	if snap[0] != 1 {
		t.Errorf("snapshot changed after a later Enter: %d", snap[0])
	}
	if got := o.Count(0); got != 2 {
		t.Errorf("Count(0) = %d, want 2", got)
	}
	if len(snap) != 2 {
		t.Errorf("snapshot length = %d, want 2", len(snap))
	}
}
