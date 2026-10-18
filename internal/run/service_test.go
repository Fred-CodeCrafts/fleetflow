package run_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/run"
	"fleetflow/internal/run/memory"
)

// newService returns a Service backed by a fresh in-memory repository, which is
// how DESIGN 3.7 asks services to be unit-tested.
func newService() *run.Service { return run.NewService(memory.New()) }

func TestSaveStoresRun(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	config := []byte(`{"fleet_share":0.4}`)
	metrics := []byte(`{"baseline":{"trip_time":10}}`)

	got, err := svc.Save(ctx, 7, config, metrics)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasPrefix(got.ID, "run_") {
		t.Errorf("Save id = %q, want a run_ prefix", got.ID)
	}
	if got.Seed != 7 {
		t.Errorf("Save seed = %d, want 7", got.Seed)
	}
	if string(got.Config) != string(config) || string(got.Metrics) != string(metrics) {
		t.Errorf("Save returned config %s metrics %s, want the documents it was given", got.Config, got.Metrics)
	}
	if got.CreatedAt.IsZero() {
		t.Error("Save returned a zero created_at; the store did not stamp the record")
	}

	// The run has to be readable again, not only be in the result.
	found, err := svc.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found.ID != got.ID || found.Seed != got.Seed {
		t.Errorf("Get returned %+v, want the saved run %+v", found, got)
	}
}

// TestSaveKeepsTheSeedUnjudged pins the contract: the simulator defines seeding,
// so zero, negative and extreme values are all valid and the service must not
// invent a rule about them.
func TestSaveKeepsTheSeedUnjudged(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	for _, seed := range []int64{0, -1, -12345, math.MaxInt64, math.MinInt64} {
		got, err := svc.Save(ctx, seed, []byte(`{}`), []byte(`{}`))
		if err != nil {
			t.Fatalf("Save(seed=%d): %v", seed, err)
		}
		if got.Seed != seed {
			t.Errorf("Save(seed=%d) stored %d, want it unchanged", seed, got.Seed)
		}
	}
}

func TestSaveRefusesBlankDocuments(t *testing.T) {
	tests := []struct {
		name     string
		config   []byte
		metrics  []byte
		wantCode string
	}{
		{"nil config", nil, []byte(`{}`), "run_config_required"},
		{"empty config", []byte{}, []byte(`{}`), "run_config_required"},
		{"nil metrics", []byte(`{}`), nil, "run_metrics_required"},
		{"empty metrics", []byte(`{}`), []byte{}, "run_metrics_required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService()
			ctx := context.Background()

			got, err := svc.Save(ctx, 1, tt.config, tt.metrics)
			if err == nil {
				t.Fatalf("Save accepted a blank document as %+v", got)
			}
			if kind := domain.KindOf(err); kind != domain.KindInvalid {
				t.Errorf("Save kind = %s, want invalid", kind)
			}
			if code, _, ok := domain.Public(err); !ok || code != tt.wantCode {
				t.Errorf("Save code = %q (ok=%v), want %q", code, ok, tt.wantCode)
			}

			list, err := svc.List(ctx, 0)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("a refused save stored %d run(s)", len(list))
			}
		})
	}
}

// TestListReturnsMostRecentFirst pins the order the API promises: the newest run
// is first, because that is what a dashboard shows and what a caller asking for
// "the latest runs" expects.
func TestListReturnsMostRecentFirst(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	seeds := []int64{10, 20, 30}
	for _, seed := range seeds {
		if _, err := svc.Save(ctx, seed, []byte(`{}`), []byte(`{}`)); err != nil {
			t.Fatalf("Save(seed=%d): %v", seed, err)
		}
	}

	list, err := svc.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(seeds) {
		t.Fatalf("List returned %d runs, want %d", len(list), len(seeds))
	}
	for i := range seeds {
		want := seeds[len(seeds)-1-i]
		if list[i].Seed != want {
			t.Errorf("list[%d].Seed = %d, want %d (most recent first)", i, list[i].Seed, want)
		}
	}
}

// TestListClampsTheLimit pins the clamp from both ends: a missing or nonsensical
// limit becomes the default page, and an oversized one is capped rather than
// refused or honoured.
func TestListClampsTheLimit(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	const saved = 120
	for i := 0; i < saved; i++ {
		if _, err := svc.Save(ctx, int64(i), []byte(`{}`), []byte(`{}`)); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero means the default", 0, 25},
		{"negative means the default", -5, 25},
		{"within range is honoured", 5, 5},
		{"oversized is capped", 10_000, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := svc.List(ctx, tt.limit)
			if err != nil {
				t.Fatalf("List(%d): %v", tt.limit, err)
			}
			if len(list) != tt.want {
				t.Errorf("List(%d) returned %d runs, want %d", tt.limit, len(list), tt.want)
			}
		})
	}
}

func TestGetReportsAnUnknownRunAsNotFound(t *testing.T) {
	svc := newService()

	_, err := svc.Get(context.Background(), "run_missing")
	if err == nil {
		t.Fatal("Get returned an unknown run")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Get kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "run_not_found" {
		t.Errorf("Get code = %q (ok=%v), want run_not_found", code, ok)
	}
}

func TestGetRejectsABlankID(t *testing.T) {
	svc := newService()

	_, err := svc.Get(context.Background(), "")
	if err == nil {
		t.Fatal("Get accepted a blank id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInvalid {
		t.Errorf("Get kind = %s, want invalid", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "run_id_required" {
		t.Errorf("Get code = %q (ok=%v), want run_id_required", code, ok)
	}
}

// TestServiceIDsAreUnique checks the generated ids do not repeat, which is what
// lets a caller Get exactly the run it saved.
func TestServiceIDsAreUnique(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		got, err := svc.Save(ctx, int64(i), []byte(`{}`), []byte(`{}`))
		if err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		if seen[got.ID] {
			t.Fatalf("Save %d repeated id %q", i, got.ID)
		}
		seen[got.ID] = true
	}
}

// stubRepo is a Repository that always fails, to check how the service
// classifies a storage failure without needing a database.
type stubRepo struct{ err error }

func (s stubRepo) Create(context.Context, run.Run) (run.Run, error) {
	return run.Run{}, s.err
}

func (s stubRepo) List(context.Context, int) ([]run.Run, error) {
	return nil, s.err
}

func (s stubRepo) Get(context.Context, string) (run.Run, error) {
	return run.Run{}, s.err
}

func TestServiceHidesRawStorageError(t *testing.T) {
	raw := errors.New("pq: dial tcp 10.0.0.7:5432: connection refused")
	svc := run.NewService(stubRepo{err: raw})

	_, err := svc.Save(context.Background(), 1, []byte(`{}`), []byte(`{}`))
	if err == nil {
		t.Fatal("Save succeeded although the repository failed")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Save kind = %s, want internal", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "run_create_failed" {
		t.Errorf("Save code = %q (ok=%v), want run_create_failed", code, ok)
	}
	if strings.Contains(message, "10.0.0.7") {
		t.Errorf("Save leaked the internal cause in %q", message)
	}
	if !errors.Is(err, raw) {
		t.Error("Save dropped the cause, so logs cannot show why storage failed")
	}
}

func TestServiceReportsCancelledRequestAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := newService()
	_, err := svc.Save(ctx, 1, []byte(`{}`), []byte(`{}`))
	if err == nil {
		t.Fatal("Save succeeded with a cancelled context")
	}
	if kind := domain.KindOf(err); kind != domain.KindUnavailable {
		t.Errorf("Save kind = %s, want unavailable", kind)
	}
	if code, _, _ := domain.Public(err); code != "request_cancelled" {
		t.Errorf("Save code = %q, want request_cancelled", code)
	}
}
