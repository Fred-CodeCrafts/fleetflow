package simulation

import (
	"reflect"
	"testing"

	"fleetflow/internal/simulation/sumo"
)

func TestGenerateDeliveriesIsDeterministicAndOrdersCompletedEvents(t *testing.T) {
	arrival := 30.0
	result := sumo.Result{
		Seed: 42,
		Trips: []sumo.Trip{{
			ID: "fleetflow0", Status: sumo.TripCompleted, DepartS: 0, ArrivalS: &arrival,
		}},
		Tracks: []sumo.Track{{
			VehicleID: "fleetflow0",
			Positions: []sumo.Position{
				{TimeS: 0, Longitude: 106.8, Latitude: -6.2},
				{TimeS: 5, Longitude: 106.81, Latitude: -6.21},
				{TimeS: 10, Longitude: 106.82, Latitude: -6.22},
				{TimeS: 15, Longitude: 106.83, Latitude: -6.23},
				{TimeS: 20, Longitude: 106.84, Latitude: -6.24},
			},
		}},
	}
	if err := generateDeliveries(&result); err != nil {
		t.Fatal(err)
	}
	first := result.Deliveries[0]
	if len(first.Stops) == 0 || len(first.Packages) != len(first.Stops) {
		t.Fatalf("delivery stops/packages = %d/%d", len(first.Stops), len(first.Packages))
	}
	if first.Status != sumo.DeliveryCompleted {
		t.Fatalf("delivery status = %q, want completed", first.Status)
	}
	if first.Events[len(first.Events)-1].Type != "delivery_completed" {
		t.Fatalf("last event = %#v, want delivery_completed", first.Events[len(first.Events)-1])
	}
	for index, event := range first.Events {
		if event.Sequence != index+1 {
			t.Errorf("event %d sequence = %d", index, event.Sequence)
		}
		if index > 0 && event.TimeS < first.Events[index-1].TimeS {
			t.Errorf("event %d at %v precedes event %d at %v",
				index, event.TimeS, index-1, first.Events[index-1].TimeS)
		}
	}

	again := result
	again.Deliveries = nil
	if err := generateDeliveries(&again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again.Deliveries[0]) {
		t.Fatal("same seed and SUMO output generated different delivery history")
	}
}

func TestGenerateDeliveriesDoesNotCompleteRunningTrip(t *testing.T) {
	result := sumo.Result{
		Seed: 3,
		Trips: []sumo.Trip{{
			ID: "fleetflow1", Status: sumo.TripRunning, DepartS: 2,
		}},
		Tracks: []sumo.Track{{
			VehicleID: "fleetflow1",
			Positions: []sumo.Position{
				{TimeS: 5, Longitude: 106.8, Latitude: -6.2},
				{TimeS: 10, Longitude: 106.81, Latitude: -6.21},
			},
		}},
	}
	if err := generateDeliveries(&result); err != nil {
		t.Fatal(err)
	}
	delivery := result.Deliveries[0]
	if delivery.Status != sumo.DeliveryInProgress || len(delivery.Stops) != 1 {
		t.Fatalf("running delivery = %#v", delivery)
	}
	for _, event := range delivery.Events {
		if event.Type == "package_delivered" || event.Type == "delivery_completed" {
			t.Errorf("running trip has premature terminal event %#v", event)
		}
	}
	if delivery.Packages[0].Status != sumo.PackageInTransit {
		t.Errorf("running package status = %q, want in_transit", delivery.Packages[0].Status)
	}
}

// A scenario the button starts is finished through the same path as SUMO: the
// generator's trips are handed to the delivery history builder. That only works
// if a completed trip carries its arrival, which is the bug this test pins: the
// generator used to mark a trip complete without saying when it arrived, so every
// started run failed with "has no arrival time".
func TestSyntheticScenarioTripsAreCompleteEnoughForTheDeliveryHistory(t *testing.T) {
	result := GenerateSyntheticScenario(42, 20, 300)
	for _, trip := range result.Trips {
		if trip.Status != sumo.TripCompleted {
			continue
		}
		if trip.ArrivalS == nil {
			t.Fatalf("trip %q is completed without an arrival time", trip.ID)
		}
		if *trip.ArrivalS < trip.DepartS+trip.DurationS {
			t.Fatalf("trip %q arrives at %.0fs, before its own duration ends at %.0fs",
				trip.ID, *trip.ArrivalS, trip.DepartS+trip.DurationS)
		}
	}
	if err := generateDeliveries(&result); err != nil {
		t.Fatalf("delivery history rejected the generated scenario: %v", err)
	}
	if len(result.Deliveries) != len(result.Trips) {
		t.Fatalf("built %d deliveries for %d trips", len(result.Deliveries), len(result.Trips))
	}
}

func TestGenerateDeliveriesRejectsMissingTrack(t *testing.T) {
	result := sumo.Result{Trips: []sumo.Trip{{ID: "fleetflow0"}}}
	if err := generateDeliveries(&result); err == nil {
		t.Fatal("generated a delivery without a SUMO position trace")
	}
}

func TestGenerateDeliveriesRejectsCompletedTripWithoutArrival(t *testing.T) {
	result := sumo.Result{Trips: []sumo.Trip{{ID: "fleetflow0", Status: sumo.TripCompleted}}}
	if err := generateDeliveries(&result); err == nil {
		t.Fatal("generated completed delivery history without a SUMO arrival time")
	}
}
