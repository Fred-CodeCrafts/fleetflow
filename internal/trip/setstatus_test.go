package trip_test

import (
	"context"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
)

// TestStatusAdvancesForwardsOnly pins the one rule a status move has: a trip goes
// planned, then in progress, then completed, and never back. Setting the status a
// trip already has is allowed, so a retried request is not an error.
func TestStatusAdvancesForwardsOnly(t *testing.T) {
	tests := []struct {
		name     string
		from, to trip.Status
		want     bool
	}{
		{"planned to planned", trip.StatusPlanned, trip.StatusPlanned, true},
		{"planned to in progress", trip.StatusPlanned, trip.StatusInProgress, true},
		{"planned to completed", trip.StatusPlanned, trip.StatusCompleted, true},
		{"in progress to in progress", trip.StatusInProgress, trip.StatusInProgress, true},
		{"in progress to completed", trip.StatusInProgress, trip.StatusCompleted, true},
		{"completed to completed", trip.StatusCompleted, trip.StatusCompleted, true},
		{"in progress to planned", trip.StatusInProgress, trip.StatusPlanned, false},
		{"completed to planned", trip.StatusCompleted, trip.StatusPlanned, false},
		{"completed to in progress", trip.StatusCompleted, trip.StatusInProgress, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.Advances(tt.to); got != tt.want {
				t.Errorf("%s.Advances(%s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestServiceSetStatusMovesTheTripForward(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	tr, err := svc.Create(ctx, trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.SetStatus(ctx, tr.ID, "in_progress")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != trip.StatusInProgress {
		t.Errorf("SetStatus returned %s, want in_progress", got.Status)
	}

	// The move has to be visible through List, not only in the result.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != trip.StatusInProgress {
		t.Errorf("List returned %+v, want the trip in progress", list)
	}
}

// TestServiceSetStatusRejectsBadInput covers what the service refuses before the
// store is reached: a blank id and a status that is not a real one are both the
// client's to fix.
func TestServiceSetStatusRejectsBadInput(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	tr, err := svc.Create(ctx, trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tests := []struct {
		name     string
		tripID   string
		status   string
		wantCode string
	}{
		{"blank id", "   ", "in_progress", "trip_id_required"},
		{"unknown status", tr.ID, "flying", "trip_status_unknown"},
		{"blank status", tr.ID, "", "trip_status_unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.SetStatus(ctx, tt.tripID, tt.status); err == nil {
				t.Fatal("SetStatus accepted input the domain refuses")
			} else {
				if kind := domain.KindOf(err); kind != domain.KindInvalid {
					t.Errorf("kind = %s, want invalid", kind)
				}
				if code, _, _ := domain.Public(err); code != tt.wantCode {
					t.Errorf("code = %q, want %q", code, tt.wantCode)
				}
			}
		})
	}
}

// TestServiceSetStatusKeepsTheRepositoryOutcome checks that a rule the store
// decided — here, a trip that may not move backwards — reaches the transport with
// its own code instead of being rewritten as a storage failure.
func TestServiceSetStatusKeepsTheRepositoryOutcome(t *testing.T) {
	conflict := domain.Conflict("trip_status_regressed", "trip t1 is completed and cannot go back to planned")
	svc := trip.NewService(stubRepo{err: conflict})

	_, err := svc.SetStatus(context.Background(), "t1", "planned")
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_status_regressed" {
		t.Errorf("code = %q, want trip_status_regressed", code)
	}
}
