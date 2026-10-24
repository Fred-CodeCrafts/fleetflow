package sumo

import (
	"strings"
	"testing"
)

func TestParseTracks(t *testing.T) {
	input := `<fcd-export>
	<timestep time="0"><vehicle id="vehicle-1" x="106.81" y="-6.24"/></timestep>
	<timestep time="5"><vehicle id="vehicle-1" x="106.82" y="-6.25"/><vehicle id="vehicle-2" x="106.9" y="-6.1"/></timestep>
</fcd-export>`

	tracks, err := parseTracks(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].VehicleID != "vehicle-1" {
		t.Fatalf("tracks = %#v", tracks)
	}
	if len(tracks[0].Positions) != 2 || tracks[0].Positions[1].Longitude != 106.82 {
		t.Fatalf("vehicle-1 positions = %#v", tracks[0].Positions)
	}
	if tracks[1].Positions[0].TimeS != 5 {
		t.Fatalf("vehicle-2 position time = %v, want 5", tracks[1].Positions[0].TimeS)
	}
}

func TestParseTracksRejectsInvalidCoordinates(t *testing.T) {
	input := `<fcd-export><timestep time="0"><vehicle id="v1" x="200" y="-6"/></timestep></fcd-export>`
	if _, err := parseTracks(strings.NewReader(input)); err == nil {
		t.Fatal("parseTracks() accepted a longitude outside the geographic range")
	}
}

func TestParseTrips(t *testing.T) {
	input := `<tripinfos>
	<tripinfo id="done" depart="0" arrival="90" duration="90" routeLength="2360.85">
		<emissions CO2_abs="517547.08" fuel_abs="167779.96"/>
	</tripinfo>
	<tripinfo id="active" depart="60" arrival="-1" duration="30" routeLength="450">
		<emissions CO2_abs="1000" fuel_abs="200"/>
	</tripinfo>
</tripinfos>`

	trips, err := parseTrips(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) != 2 || trips[0].Status != TripCompleted || trips[1].Status != TripRunning {
		t.Fatalf("trips = %#v", trips)
	}
	if trips[0].ArrivalS == nil || *trips[0].ArrivalS != 90 {
		t.Fatalf("completed arrival = %v, want 90", trips[0].ArrivalS)
	}
	if trips[0].CO2Kg < 0.517547 || trips[0].CO2Kg > 0.517548 {
		t.Fatalf("CO2Kg = %v, want SUMO mg converted to kg", trips[0].CO2Kg)
	}
	if trips[0].FuelKg < 0.167779 || trips[0].FuelKg > 0.167781 {
		t.Fatalf("FuelKg = %v, want SUMO mg converted to kg", trips[0].FuelKg)
	}
	if trips[1].ArrivalS != nil {
		t.Fatalf("running arrival = %v, want unknown", trips[1].ArrivalS)
	}
}
