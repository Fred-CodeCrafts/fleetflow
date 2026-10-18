package memory_test

import (
	"context"
	"errors"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/run"
	"fleetflow/internal/run/memory"
)

// newRun returns a storable run with the two documents filled in, since a blank
// one is refused.
func newRun(id string, seed int64) run.Run {
	return run.Run{ID: id, Seed: seed, Config: []byte(`{"a":1}`), Metrics: []byte(`{"b":2}`)}
}

func TestCreateThenList(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	stored, err := repo.Create(ctx, newRun("r1", 1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.CreatedAt.IsZero() {
		t.Error("Create returned a zero created_at; the store did not stamp the record")
	}

	list, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "r1" || list[0].Seed != 1 {
		t.Errorf("List returned %+v, want just r1", list)
	}
}

// TestListIsMostRecentFirst pins the order against the store directly: the newest
// save leads, regardless of timestamps sharing an instant.
func TestListIsMostRecentFirst(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	for _, id := range []string{"r1", "r2", "r3"} {
		if _, err := repo.Create(ctx, newRun(id, 0)); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}

	list, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 || list[0].ID != "r3" || list[1].ID != "r2" || list[2].ID != "r1" {
		t.Errorf("List order = %v, want r3, r2, r1", ids(list))
	}
}

func TestListHonoursTheLimit(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	for _, id := range []string{"r1", "r2", "r3"} {
		if _, err := repo.Create(ctx, newRun(id, 0)); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}

	list, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != "r3" || list[1].ID != "r2" {
		t.Errorf("List(2) = %v, want the two most recent, r3 then r2", ids(list))
	}
}

func TestCreateRejectsDuplicateID(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, newRun("r1", 0)); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, newRun("r1", 0))
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}

	list, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d runs after a rejected create, want 1", len(list))
	}
}

func TestCreateRejectsARecordWithNoID(t *testing.T) {
	repo := memory.New()

	blank := run.Run{Seed: 1, Config: []byte(`{}`), Metrics: []byte(`{}`)}
	_, err := repo.Create(context.Background(), blank)
	if err == nil {
		t.Fatal("Create accepted a run with no id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
}

// TestGetAndListReturnFreshCopies guards the seam: a caller that mutates the bytes
// or the slice it was handed must not be able to reach the stored run.
func TestGetAndListReturnFreshCopies(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, newRun("r1", 1)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Config[0] = 'X'
	list, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Metrics[0] = 'Y'

	again, err := repo.Get(ctx, "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(again.Config) != `{"a":1}` || string(again.Metrics) != `{"b":2}` {
		t.Errorf("stored run = config %s metrics %s, want the originals", again.Config, again.Metrics)
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	repo := memory.New()

	_, err := repo.Get(context.Background(), "r_missing")
	if err == nil {
		t.Fatal("Get returned an unknown run")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Get kind = %s, want not_found", kind)
	}
}

func TestMethodsHonourACancelledContext(t *testing.T) {
	repo := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := newRun("r1", 0)
	if _, err := repo.Create(ctx, rec); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx, 10); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
	if _, err := repo.Get(ctx, "r1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get error = %v, want context.Canceled", err)
	}
}

// ids returns the ids of list, for a readable failure message.
func ids(list []run.Run) []string {
	out := make([]string, len(list))
	for i, r := range list {
		out[i] = r.ID
	}
	return out
}
