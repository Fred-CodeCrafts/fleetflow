package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
)

// insertVehicleWithStatus adds a vehicle row with a chosen status, so a test can
// set up the case DESIGN 3.5 names: a vehicle that is offline, or already on a
// trip, must not be given another one.
func insertVehicleWithStatus(t *testing.T, pool *pgxpool.Pool, id, status string) {
	t.Helper()

	const q = `insert into vehicles (id, name, status) values ($1, $2, $3)`
	if _, err := pool.Exec(context.Background(), q, id, "Vehicle "+id, status); err != nil {
		t.Fatalf("inserting vehicle %s: %v", id, err)
	}
}

func TestCreateWithAnAvailableVehicleSucceeds(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")

	stored := mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})
	if stored.VehicleID != "v1" {
		t.Errorf("Create returned VehicleID %q, want v1", stored.VehicleID)
	}
}

// TestCreateRefusesAnUnavailableVehicle covers the half of availability the
// partial unique index cannot express: a vehicle that is offline may not be given
// a trip at all, however few trips it is on.
func TestCreateRefusesAnUnavailableVehicle(t *testing.T) {
	for _, status := range []string{"offline", "on_trip"} {
		t.Run(status, func(t *testing.T) {
			repo, pool := newRepository(t)
			insertVehicleWithStatus(t, pool, "v1", status)

			_, err := repo.Create(context.Background(), trip.Trip{
				ID: "t1", Origin: "A", Destination: "B",
				VehicleID: "v1", Status: trip.StatusPlanned,
			})
			if err == nil {
				t.Fatalf("Create gave a trip to a vehicle whose status is %s", status)
			}
			if kind := domain.KindOf(err); kind != domain.KindConflict {
				t.Errorf("Create kind = %s, want conflict", kind)
			}
			if code, _, _ := domain.Public(err); code != "trip_vehicle_unavailable" {
				t.Errorf("Create code = %q, want trip_vehicle_unavailable", code)
			}

			// A refused create must not have written anything.
			list, err := repo.List(context.Background())
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("a refused create stored %d trip(s)", len(list))
			}
		})
	}
}

func TestCreateWithAnUnknownVehicleIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)

	_, err := repo.Create(context.Background(), trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "nobody", Status: trip.StatusPlanned,
	})
	if err == nil {
		t.Fatal("Create accepted a trip on a vehicle that does not exist")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Create kind = %s, want not found", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_vehicle_unknown" {
		t.Errorf("Create code = %q, want trip_vehicle_unknown", code)
	}
}

// TestCreateWithoutAVehicleIsUnaffected keeps the availability check out of the
// case it does not apply to.
func TestCreateWithoutAVehicleIsUnaffected(t *testing.T) {
	repo, _ := newRepository(t)

	stored := mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned,
	})
	if stored.VehicleID != "" {
		t.Errorf("Create returned VehicleID %q, want empty", stored.VehicleID)
	}
}

// TestConcurrentCreateForOneVehicleHasOneWinner is the point of doing the check
// and the insert in one transaction: two requests race for the same available
// vehicle, and a check followed by a separate write would let both through.
func TestConcurrentCreateForOneVehicleHasOneWinner(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")

	const attempts = 8
	var wg sync.WaitGroup
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = repo.Create(context.Background(), trip.Trip{
				ID: fmt.Sprintf("t%d", i), Origin: "A", Destination: "B",
				VehicleID: "v1", Status: trip.StatusPlanned,
			})
		}(i)
	}
	wg.Wait()

	winners := 0
	for i, err := range errs {
		if err == nil {
			winners++
			continue
		}
		if kind := domain.KindOf(err); kind != domain.KindConflict {
			t.Errorf("attempt %d failed with kind %s (%v), want a conflict", i, kind, err)
		}
	}
	if winners != 1 {
		t.Errorf("%d of %d attempts succeeded, want exactly 1", winners, attempts)
	}
}
