package postgres_test

import (
	"context"
	"testing"

	"fleetflow/internal/trip"
)

// TestSetStatusRecordsTimingInTheDatabase is the test the memory store cannot
// write: it proves the SQL behind PRD US-9 actually stores a start and a finish,
// that a repeated request does not move them, and that a trip which never
// started reports no duration rather than a zero one.
func TestSetStatusRecordsTimingInTheDatabase(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	// A planned trip has no timing yet.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !list[0].StartedAt.IsZero() || !list[0].FinishedAt.IsZero() {
		t.Fatalf("a planned trip already carries timing: %+v", list[0])
	}

	started, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if started.StartedAt.IsZero() {
		t.Fatal("starting the trip recorded no start time")
	}
	if !started.FinishedAt.IsZero() {
		t.Fatal("an in-progress trip recorded a finish time")
	}

	// Repeating the same transition must not move the start.
	again, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("repeating the start: %v", err)
	}
	if !again.StartedAt.Equal(started.StartedAt) {
		t.Errorf("a repeated request moved the start from %v to %v", started.StartedAt, again.StartedAt)
	}

	finished, err := repo.SetStatus(ctx, "t1", trip.StatusCompleted)
	if err != nil {
		t.Fatalf("finishing: %v", err)
	}
	if finished.FinishedAt.IsZero() {
		t.Fatal("completing the trip recorded no finish time")
	}
	if _, ok := finished.Duration(); !ok {
		t.Error("a completed trip with both timestamps reported no duration")
	}

	// The stored row is what changed, not only the returned value: a restart
	// reads these back through List.
	stored, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List after finishing: %v", err)
	}
	if stored[0].StartedAt.IsZero() || stored[0].FinishedAt.IsZero() {
		t.Fatalf("the database did not keep the timing: %+v", stored[0])
	}
	if !stored[0].FinishedAt.Equal(finished.FinishedAt) {
		t.Errorf("stored finish %v differs from returned %v", stored[0].FinishedAt, finished.FinishedAt)
	}
}

// TestSetStatusSkippingToCompletedLeavesNoStart pins the honest case at the
// storage layer: a trip moved straight from planned to completed never started,
// so started_at stays null and the API reports a null duration.
func TestSetStatusSkippingToCompletedLeavesNoStart(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	done, err := repo.SetStatus(ctx, "t1", trip.StatusCompleted)
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if !done.StartedAt.IsZero() {
		t.Errorf("a trip that never started got start time %v", done.StartedAt)
	}
	if done.FinishedAt.IsZero() {
		t.Error("a completed trip has no finish time")
	}
	if _, ok := done.Duration(); ok {
		t.Error("a trip with no recorded start reported a duration")
	}
}
