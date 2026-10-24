package memory

import (
	"context"
	"testing"
	"time"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
)

func TestRepositoryStoresIsolatedRunResults(t *testing.T) {
	repo := New()
	run := simulation.Run{
		ID:        "sim_test",
		Status:    simulation.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		Result: &sumo.Result{
			Tracks: []sumo.Track{{VehicleID: "v1", Positions: []sumo.Position{{Longitude: 106.8, Latitude: -6.2}}}},
			Deliveries: []sumo.Delivery{{
				ID:       "delivery-1",
				Packages: []sumo.Package{{ID: "package-1", Status: sumo.PackageDelivered}},
				Stops:    []sumo.DeliveryStop{{ID: "stop-1", Status: sumo.StopDelivered}},
				Events:   []sumo.DeliveryEvent{{Sequence: 1, Type: "package_delivered"}},
			}},
		},
	}
	if err := repo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	found, err := repo.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found.Result.Tracks[0].Positions[0].Longitude = 0
	found.Result.Deliveries[0].Packages[0].Status = sumo.PackageInTransit
	found.Result.Deliveries[0].Stops[0].Status = sumo.StopPending
	found.Result.Deliveries[0].Events[0].Type = "tampered"
	again, err := repo.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Result.Tracks[0].Positions[0].Longitude; got != 106.8 {
		t.Fatalf("stored position changed through returned value: longitude = %v", got)
	}
	if got := again.Result.Deliveries[0].Packages[0].Status; got != sumo.PackageDelivered {
		t.Errorf("stored package changed through returned value: status = %q", got)
	}
	if got := again.Result.Deliveries[0].Stops[0].Status; got != sumo.StopDelivered {
		t.Errorf("stored stop changed through returned value: status = %q", got)
	}
	if got := again.Result.Deliveries[0].Events[0].Type; got != "package_delivered" {
		t.Errorf("stored event changed through returned value: type = %q", got)
	}
}

func TestRepositoryListOrdersNewestFirst(t *testing.T) {
	repo := New()
	for _, run := range []simulation.Run{
		{ID: "older", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "newer", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	} {
		if err := repo.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	found, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].ID != "newer" || found[1].ID != "older" {
		t.Fatalf("run list order = %#v", found)
	}
	if found[0].Result != nil || found[1].Result != nil {
		t.Fatal("run list included large result payloads")
	}
}
