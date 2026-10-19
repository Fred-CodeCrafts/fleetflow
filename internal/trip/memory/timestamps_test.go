package memory

import (
	"context"
	"testing"
	"time"

	"fleetflow/internal/trip"
)

// TestSetStatus_RecordsStartAndFinish pins the transition timestamps, which are
// what PRD US-9's "start, end and duration" is built from. Without them a
// completed trip knows only when its row was created.
func TestSetStatus_RecordsStartAndFinish(t *testing.T) {
	repo := New()
	ctx := context.Background()

	created, err := repo.Create(ctx, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned,
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if !created.StartedAt.IsZero() || !created.FinishedAt.IsZero() {
		t.Fatalf("a planned trip already has timestamps: %+v", created)
	}
	if _, ok := created.Duration(); ok {
		t.Error("a planned trip reported a duration")
	}

	started, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if started.StartedAt.IsZero() {
		t.Fatal("in_progress did not record a start time")
	}
	if !started.FinishedAt.IsZero() {
		t.Fatal("in_progress recorded a finish time")
	}

	// Sleep a little so the duration is not zero; the store uses wall clock and
	// a test that asserts a positive duration needs two distinct instants.
	time.Sleep(2 * time.Millisecond)

	finished, err := repo.SetStatus(ctx, "t1", trip.StatusCompleted)
	if err != nil {
		t.Fatalf("finishing: %v", err)
	}
	if finished.FinishedAt.IsZero() {
		t.Fatal("completed did not record a finish time")
	}

	d, ok := finished.Duration()
	if !ok {
		t.Fatal("a completed trip with both timestamps reported no duration")
	}
	if d <= 0 {
		t.Errorf("duration = %v, want positive", d)
	}
	if !finished.FinishedAt.After(finished.StartedAt) {
		t.Errorf("finished %v is not after started %v", finished.FinishedAt, finished.StartedAt)
	}
}

// TestSetStatus_RetryDoesNotRewriteHistory keeps a repeated request idempotent
// in the timestamps as well as the status. A dispatcher clicking twice must not
// produce a different duration.
func TestSetStatus_RetryDoesNotRewriteHistory(t *testing.T) {
	repo := New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)

	again, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("repeating the same status: %v", err)
	}
	if !again.StartedAt.Equal(first.StartedAt) {
		t.Errorf("a repeated request moved the start from %v to %v", first.StartedAt, again.StartedAt)
	}
}

// TestSetStatus_SkippingToCompletedInventsNoStart is the honest case: a trip
// that jumps from planned to completed never had a start, so it must report no
// duration rather than a zero-length one.
func TestSetStatus_SkippingToCompletedInventsNoStart(t *testing.T) {
	repo := New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}); err != nil {
		t.Fatal(err)
	}
	done, err := repo.SetStatus(ctx, "t1", trip.StatusCompleted)
	if err != nil {
		t.Fatal(err)
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

// TestDuration_NegativeIsNotAMeasurement covers clock skew, which is a real
// possibility when one timestamp came from a database default and the other
// from this process.
func TestDuration_NegativeIsNotAMeasurement(t *testing.T) {
	tr := trip.Trip{
		Status:     trip.StatusCompleted,
		StartedAt:  time.Now(),
		FinishedAt: time.Now().Add(-time.Minute),
	}
	if _, ok := tr.Duration(); ok {
		t.Error("a trip that finished before it started reported a duration")
	}
}
