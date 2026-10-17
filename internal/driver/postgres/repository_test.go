package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
	"fleetflow/internal/driver/postgres"
	"fleetflow/internal/store/storetest"
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

// mustCreate stores d or fails the test.
func mustCreate(t *testing.T, repo *postgres.Repository, d driver.Driver) {
	t.Helper()
	if _, err := repo.Create(context.Background(), d); err != nil {
		t.Fatalf("Create(%s): %v", d.ID, err)
	}
}

// mustVehicle inserts a vehicle row directly. A driver's vehicle must exist
// because the column is a foreign key, and no vehicle repository is needed to
// exercise the driver store on its own.
func mustVehicle(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	const q = `insert into vehicles (id, name, status) values ($1, $2, 'available')`
	if _, err := pool.Exec(context.Background(), q, id, "Vehicle "+id); err != nil {
		t.Fatalf("inserting vehicle %s: %v", id, err)
	}
}

func TestCreateThenListRoundTrips(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	want := driver.Driver{ID: "d1", Name: "Ada Lovelace", Contact: "ada@example.com"}
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

	// Creation order is the order List returns.
	second := driver.Driver{ID: "d2", Name: "Grace Hopper"}
	mustCreate(t, repo, second)
	list, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != "d1" || list[1].ID != "d2" {
		t.Errorf("List returned %+v, want d1 then d2", list)
	}
}

// TestSecondRepositoryReadsWhatTheFirstWrote is the "survives a restart"
// requirement: the record lives in the database, not in a repository instance,
// so a fresh Repository over the same pool reads it back.
func TestSecondRepositoryReadsWhatTheFirstWrote(t *testing.T) {
	_, pool := newRepository(t)
	ctx := context.Background()

	want := driver.Driver{ID: "d1", Name: "Ada Lovelace", Contact: "ada@example.com"}
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

	d := driver.Driver{ID: "d1", Name: "Ada Lovelace"}
	mustCreate(t, repo, d)

	_, err := repo.Create(ctx, d)
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "driver_exists" {
		t.Errorf("Create error code = %q (ok = %v), want driver_exists", code, ok)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d drivers after a rejected create, want 1", len(list))
	}
}

func TestAssignThenListShowsTheVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")
	mustCreate(t, repo, driver.Driver{ID: "d1", Name: "Ada Lovelace"})

	got, err := repo.Assign(ctx, "d1", "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("Assign returned VehicleID %q, want v1", got.VehicleID)
	}

	// The change must live in the store, not only in the returned copy.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want the vehicle to be assigned", list)
	}
}

func TestUnassigningClearsTheVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")
	mustCreate(t, repo, driver.Driver{ID: "d1", Name: "Ada Lovelace"})

	if _, err := repo.Assign(ctx, "d1", "v1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	got, err := repo.Assign(ctx, "d1", "")
	if err != nil {
		t.Fatalf("Assign unassign: %v", err)
	}
	if got.VehicleID != "" {
		t.Errorf("Assign returned VehicleID %q after unassigning, want empty", got.VehicleID)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "" {
		t.Errorf("List returned %+v, want no vehicle", list)
	}
}

// TestAssignToTheSameDriverIsNotAConflict covers keeping a vehicle a driver
// already holds, which the unique index must not read as a second claimant.
func TestAssignToTheSameDriverIsNotAConflict(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")
	mustCreate(t, repo, driver.Driver{ID: "d1", Name: "Ada"})

	for i := 0; i < 2; i++ {
		got, err := repo.Assign(ctx, "d1", "v1")
		if err != nil {
			t.Fatalf("Assign %d: %v", i, err)
		}
		if got.VehicleID != "v1" {
			t.Errorf("Assign %d returned VehicleID %q, want v1", i, got.VehicleID)
		}
	}
}

func TestAssignUnknownDriverIsNotFound(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")

	_, err := repo.Assign(ctx, "nobody", "v1")
	if err == nil {
		t.Fatal("Assign accepted an unknown driver")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "driver_not_found" {
		t.Errorf("Assign error code = %q (ok = %v), want driver_not_found", code, ok)
	}
}

func TestAssignAVehicleAnotherDriverHoldsIsAConflict(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")
	mustCreate(t, repo, driver.Driver{ID: "d1", Name: "Ada"})
	mustCreate(t, repo, driver.Driver{ID: "d2", Name: "Grace"})
	if _, err := repo.Assign(ctx, "d1", "v1"); err != nil {
		t.Fatalf("Assign d1: %v", err)
	}

	_, err := repo.Assign(ctx, "d2", "v1")
	if err == nil {
		t.Fatal("Assign let two drivers hold the same vehicle")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "driver_vehicle_taken" {
		t.Errorf("Assign error code = %q (ok = %v), want driver_vehicle_taken", code, ok)
	}

	// A rejected assignment must leave both drivers as they were.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].VehicleID != "v1" || list[1].VehicleID != "" {
		t.Errorf("List = %+v, want the rejected assignment to change nothing", list)
	}
}

func TestAssignUnknownVehicleIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()
	mustCreate(t, repo, driver.Driver{ID: "d1", Name: "Ada"})

	_, err := repo.Assign(ctx, "d1", "v-missing")
	if err == nil {
		t.Fatal("Assign accepted a vehicle that does not exist")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "driver_vehicle_unknown" {
		t.Errorf("Assign error code = %q (ok = %v), want driver_vehicle_unknown", code, ok)
	}

	// The driver must be unchanged after the rejected assignment.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "" {
		t.Errorf("List returned %+v, want the driver to still be unassigned", list)
	}
}

// TestAssignRaceLeavesOneWinner is the concurrency requirement: the unique index
// on vehicle_id, not the service, decides who wins. N goroutines claim one
// vehicle; exactly one succeeds and N-1 see a conflict.
func TestAssignRaceLeavesOneWinner(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()
	mustVehicle(t, pool, "v1")

	const n = 8
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "d" + string(rune('a'+i))
		mustCreate(t, repo, driver.Driver{ID: ids[i], Name: "Driver " + ids[i]})
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		conflicts int
	)
	wg.Add(n)
	for _, id := range ids {
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
				t.Errorf("Assign(%s) error = %v, want nil or a conflict", id, err)
			}
		}(id)
	}
	wg.Wait()

	if successes != 1 || conflicts != n-1 {
		t.Errorf("race produced %d winners and %d conflicts, want 1 and %d", successes, conflicts, n-1)
	}

	// The database must agree: exactly one driver holds the vehicle.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	held := 0
	for _, d := range list {
		if d.VehicleID == "v1" {
			held++
		}
	}
	if held != 1 {
		t.Errorf("%d drivers hold v1 after the race, want 1", held)
	}
}
