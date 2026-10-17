package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/store/storetest"
	"fleetflow/internal/vehicle"
	"fleetflow/internal/vehicle/postgres"
)

// newRepository returns a Postgres repository over the test database and empties
// the fleet tables first, so each test chooses the state it starts from. It
// skips, through storetest.Pool, unless TEST_DATABASE_URL is set.
func newRepository(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := storetest.Pool(t)
	storetest.Reset(t, pool)
	return postgres.New(pool), pool
}

// mustCreate stores v or fails the test.
func mustCreate(t *testing.T, repo *postgres.Repository, v vehicle.Vehicle) {
	t.Helper()
	if _, err := repo.Create(context.Background(), v); err != nil {
		t.Fatalf("Create(%s): %v", v.ID, err)
	}
}

func TestCreateThenListRoundTrips(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	want := vehicle.Vehicle{ID: "v1", Name: "B 1", Type: "van", Status: vehicle.StatusOnTrip}
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored != want {
		t.Errorf("Create returned %+v, want %+v", stored, want)
	}

	list, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != want {
		t.Errorf("List returned %+v, want just %+v", list, want)
	}
}

// TestSecondRepositoryReadsWhatTheFirstWrote is the "survives a restart"
// requirement: the record lives in the database, not in a repository instance,
// so a fresh Repository over the same pool reads it back.
func TestSecondRepositoryReadsWhatTheFirstWrote(t *testing.T) {
	_, pool := newRepository(t)
	ctx := context.Background()

	want := vehicle.Vehicle{ID: "v1", Name: "B 1", Type: "van", Status: vehicle.StatusOffline}
	mustCreate(t, postgres.New(pool), want)

	second := postgres.New(pool)
	list, err := second.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List from the second repository: %v", err)
	}
	if len(list) != 1 || list[0] != want {
		t.Errorf("second repository read %+v, want just %+v", list, want)
	}
}

func TestCreateDuplicateIDIsAConflict(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	v := vehicle.Vehicle{ID: "v1", Name: "B 1", Status: vehicle.StatusAvailable}
	mustCreate(t, repo, v)

	_, err := repo.Create(ctx, v)
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "vehicle_exists" {
		t.Errorf("Create error code = %q (ok = %v), want vehicle_exists", code, ok)
	}

	list, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d vehicles after a rejected create, want 1", len(list))
	}
}

func TestListFiltersByStatus(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, vehicle.Vehicle{ID: "v1", Status: vehicle.StatusAvailable})
	mustCreate(t, repo, vehicle.Vehicle{ID: "v2", Status: vehicle.StatusOnTrip})
	mustCreate(t, repo, vehicle.Vehicle{ID: "v3", Status: vehicle.StatusAvailable})

	all, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List(all) returned %d vehicles, want 3", len(all))
	}
	// Oldest first: the ids double as the deterministic order.
	if all[0].ID != "v1" || all[1].ID != "v2" || all[2].ID != "v3" {
		t.Errorf("List(all) order = %s, %s, %s, want v1, v2, v3", all[0].ID, all[1].ID, all[2].ID)
	}

	available, err := repo.List(ctx, vehicle.ByStatus(vehicle.StatusAvailable))
	if err != nil {
		t.Fatalf("List(available): %v", err)
	}
	if len(available) != 2 {
		t.Fatalf("List(available) returned %d vehicles, want 2", len(available))
	}
	for _, v := range available {
		if v.Status != vehicle.StatusAvailable {
			t.Errorf("List(available) returned a %s vehicle", v.Status)
		}
	}
}
