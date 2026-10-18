package sim

import "testing"

func TestNewVehicle_StartsPending(t *testing.T) {
	d := Demand{VehicleID: "v0001", From: "A", To: "D", DepartS: 12.5, Fleet: true}
	v := newVehicle(d)

	if v.ID() != "v0001" || v.Origin() != "A" || v.Destination() != "D" {
		t.Errorf("unexpected accessors: %s %s -> %s", v.ID(), v.Origin(), v.Destination())
	}
	if !v.IsFleet() {
		t.Error("expected a fleet vehicle")
	}
	if v.State() != VehiclePending {
		t.Errorf("State() = %s, want pending", v.State())
	}
	if v.DepartureS() != 12.5 {
		t.Errorf("DepartureS() = %f, want 12.5", v.DepartureS())
	}
	if v.Progress(100) != 0 || v.RouteChanges() != 0 {
		t.Errorf("expected zeroed progress and route changes, got %f and %d", v.Progress(100), v.RouteChanges())
	}
	if got := v.CurrentEdge(); got != -1 {
		t.Errorf("currentEdge() = %d, want -1 while pending", got)
	}
	if _, ok := v.TripDuration(); ok {
		t.Error("expected no trip duration before arrival")
	}
}

func TestVehicle_CurrentEdgeFollowsTheLeg(t *testing.T) {
	v := newVehicle(Demand{VehicleID: "v0002", From: "A", To: "D"})
	v.route = []int{4, 9}
	v.state = VehicleRunning

	if got := v.CurrentEdge(); got != 4 {
		t.Errorf("currentEdge() = %d, want 4", got)
	}

	v.leg = 1
	if got := v.CurrentEdge(); got != 9 {
		t.Errorf("currentEdge() = %d, want 9", got)
	}

	v.leg = 2 // past the end of the route
	if got := v.CurrentEdge(); got != -1 {
		t.Errorf("currentEdge() = %d, want -1 past the route", got)
	}
}

func TestVehicle_TripDurationAfterArrival(t *testing.T) {
	v := newVehicle(Demand{VehicleID: "v0003", DepartS: 10})
	v.state = VehicleArrived
	v.arriveS = 42.5

	got, ok := v.TripDuration()
	if !ok {
		t.Fatal("expected a trip duration after arrival")
	}
	if got != 32.5 {
		t.Errorf("TripDuration() = %f, want 32.5", got)
	}
}

func TestVehicleState_String(t *testing.T) {
	cases := map[VehicleState]string{
		VehiclePending:  "pending",
		VehicleRunning:  "running",
		VehicleArrived:  "arrived",
		VehicleState(9): "unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("VehicleState(%d).String() = %q, want %q", state, got, want)
		}
	}
}
