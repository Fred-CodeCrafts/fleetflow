package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/store/storetest"
	"fleetflow/internal/trip"
	"fleetflow/internal/trip/postgres"
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

// insertVehicle adds a vehicle row directly with SQL. A trip's vehicle must
// exist because of the foreign key, and the tests choose their own ids so runs
// cannot collide.
func insertVehicle(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	const q = `insert into vehicles (id, name, status) values ($1, $2, 'available')`
	if _, err := pool.Exec(context.Background(), q, id, "Vehicle "+id); err != nil {
		t.Fatalf("inserting vehicle %s: %v", id, err)
	}
}

// mustCreate stores tr or fails the test.
func mustCreate(t *testing.T, repo *postgres.Repository, tr trip.Trip) trip.Trip {
	t.Helper()
	stored, err := repo.Create(context.Background(), tr)
	if err != nil {
		t.Fatalf("Create(%s): %v", tr.ID, err)
	}
	return stored
}

func TestCreateThenListRoundTrips(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	want := trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusPlanned}
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored != want {
		t.Errorf("Create returned %+v, want %+v", stored, want)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != want {
		t.Errorf("List returned %+v, want just %+v", list, want)
	}
}

// TestListOrdersOldestFirst pins the contract: List is oldest first, with the id
// as the tiebreaker for rows that share a timestamp.
func TestListOrdersOldestFirst(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	for _, id := range []string{"t1", "t2", "t3"} {
		mustCreate(t, repo, trip.Trip{ID: id, Origin: "Depot", Destination: "Port", Status: trip.StatusPlanned})
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d trips, want 3", len(list))
	}
	if list[0].ID != "t1" || list[1].ID != "t2" || list[2].ID != "t3" {
		t.Errorf("List order = %s, %s, %s, want t1, t2, t3", list[0].ID, list[1].ID, list[2].ID)
	}
}

// TestSecondRepositoryReadsWhatTheFirstWrote is the "survives a restart"
// requirement: the record lives in the database, not in a repository instance,
// so a fresh Repository over the same pool reads it back.
func TestSecondRepositoryReadsWhatTheFirstWrote(t *testing.T) {
	_, pool := newRepository(t)
	ctx := context.Background()

	want := trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusInProgress}
	mustCreate(t, postgres.New(pool), want)

	second := postgres.New(pool)
	list, err := second.List(ctx)
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

	tr := trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusPlanned}
	mustCreate(t, repo, tr)

	_, err := repo.Create(ctx, tr)
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_exists" {
		t.Errorf("Create error code = %q (ok = %v), want trip_exists", code, ok)
	}
}

func TestAssignThenListShowsVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusPlanned})

	updated, err := repo.Assign(ctx, "t1", "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if updated.VehicleID != "v1" {
		t.Errorf("Assign returned vehicle %q, want v1", updated.VehicleID)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want t1 on v1", list)
	}
}

func TestUnassigningClearsTheVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusPlanned})
	if _, err := repo.Assign(ctx, "t1", "v1"); err != nil {
		t.Fatalf("Assign(v1): %v", err)
	}

	updated, err := repo.Assign(ctx, "t1", "")
	if err != nil {
		t.Fatalf("Assign(empty): %v", err)
	}
	if updated.VehicleID != "" {
		t.Errorf("Assign(empty) left vehicle %q, want none", updated.VehicleID)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "" {
		t.Errorf("List returned %+v, want t1 with no vehicle", list)
	}
}

func TestCompletedTripRefusesAssignment(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "Depot", Destination: "Port", Status: trip.StatusCompleted})

	_, err := repo.Assign(ctx, "t1", "v1")
	if err == nil {
		t.Fatal("Assign accepted a completed trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_completed" {
		t.Errorf("Assign error code = %q (ok = %v), want trip_completed", code, ok)
	}
}

func TestOneVehicleCannotBeOnTwoUnfinishedTrips(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})
	mustCreate(t, repo, trip.Trip{ID: "t2", Origin: "C", Destination: "D", Status: trip.StatusPlanned})

	if _, err := repo.Assign(ctx, "t1", "v1"); err != nil {
		t.Fatalf("Assign(t1): %v", err)
	}

	_, err := repo.Assign(ctx, "t2", "v1")
	if err == nil {
		t.Fatal("Assign put one vehicle on two unfinished trips")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_vehicle_busy" {
		t.Errorf("Assign error code = %q (ok = %v), want trip_vehicle_busy", code, ok)
	}
}

// TestVehicleFreedByCompletedTripCanBeReused checks that finishing a trip
// releases its vehicle: the partial unique index excludes completed trips, so a
// new unfinished trip may take the vehicle the finished one held.
func TestVehicleFreedByCompletedTripCanBeReused(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})
	mustCreate(t, repo, trip.Trip{ID: "t2", Origin: "C", Destination: "D", Status: trip.StatusPlanned})

	if _, err := repo.Assign(ctx, "t1", "v1"); err != nil {
		t.Fatalf("Assign(t1): %v", err)
	}

	// Finish the first trip without going through Assign, which refuses a
	// completed trip on purpose.
	if _, err := pool.Exec(ctx, `update trips set status = 'completed' where id = 't1'`); err != nil {
		t.Fatalf("completing t1: %v", err)
	}

	updated, err := repo.Assign(ctx, "t2", "v1")
	if err != nil {
		t.Fatalf("Assign(t2) after t1 completed: %v", err)
	}
	if updated.VehicleID != "v1" {
		t.Errorf("Assign(t2) returned vehicle %q, want v1", updated.VehicleID)
	}
}

func TestAssignUnknownTripIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	_, err := repo.Assign(ctx, "nope", "v1")
	if err == nil {
		t.Fatal("Assign accepted an unknown trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_not_found" {
		t.Errorf("Assign error code = %q (ok = %v), want trip_not_found", code, ok)
	}
}

func TestAssignUnknownVehicleIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	_, err := repo.Assign(ctx, "t1", "ghost")
	if err == nil {
		t.Fatal("Assign accepted a vehicle that does not exist")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_vehicle_unknown" {
		t.Errorf("Assign error code = %q (ok = %v), want trip_vehicle_unknown", code, ok)
	}
}

// TestConcurrentAssignOneVehicleHasOneWinner races N goroutines to put N
// different trips on one vehicle. Exactly one may win; the rest must be
// conflicts, which is the partial unique index deciding the race rather than a
// read-then-write in Go.
func TestConcurrentAssignOneVehicleHasOneWinner(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	insertVehicle(t, pool, "v1")

	const n = 8
	ids := make([]string, n)
	for i := range ids {
		id := fmt.Sprintf("t%d", i)
		ids[i] = id
		mustCreate(t, repo, trip.Trip{ID: id, Origin: "A", Destination: "B", Status: trip.StatusPlanned})
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes, conflicts := 0, 0
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := repo.Assign(ctx, id, "v1")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case domain.KindOf(err) == domain.KindConflict:
				conflicts++
			default:
				t.Errorf("Assign(%s) unexpected error: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("concurrent assign winners = %d, want 1", successes)
	}
	if conflicts != n-1 {
		t.Errorf("concurrent assign conflicts = %d, want %d", conflicts, n-1)
	}
}
