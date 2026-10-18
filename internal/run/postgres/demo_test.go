package postgres_test

import (
	"context"
	"testing"
)

// TestCreateKeepsTheDemoFlag checks that the column really is stored and read back.
// It is the one field that says whether the run owned the engine's weights, so a
// record that lost it could not be read honestly after the fact.
func TestCreateKeepsTheDemoFlag(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	tests := []struct {
		id   string
		demo bool
	}{
		{"run_demo", true},
		{"run_live", false},
	}

	for _, tt := range tests {
		rec := record(tt.id, 3)
		rec.Demo = tt.demo

		stored := mustCreate(t, repo, rec)
		if stored.Demo != tt.demo {
			t.Errorf("Create(%s) returned demo = %v, want %v", tt.id, stored.Demo, tt.demo)
		}

		found, err := repo.Get(ctx, tt.id)
		if err != nil {
			t.Fatalf("Get(%s): %v", tt.id, err)
		}
		if found.Demo != tt.demo {
			t.Errorf("Get(%s) returned demo = %v, want %v", tt.id, found.Demo, tt.demo)
		}
	}
}

// TestListKeepsTheDemoFlag covers the read the dashboard actually makes: the flag has
// to survive the list query too, not just a single-row read.
func TestListKeepsTheDemoFlag(t *testing.T) {
	repo, _ := newRepository(t)

	rec := record("run_listed", 9)
	rec.Demo = true
	mustCreate(t, repo, rec)

	found, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("List returned %d runs, want 1", len(found))
	}
	if !found[0].Demo {
		t.Error("List lost the demo flag")
	}
}
