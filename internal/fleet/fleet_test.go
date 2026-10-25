package fleet_test

import (
	"testing"

	"fleetflow/internal/fleet"
)

// The point of the class table is that the same kilometre costs different
// vehicles different amounts. A single grams-per-kilometre constant would report
// the same figure for a loaded box truck and an electric cargo motorcycle, which
// is what the emissions dashboard used to do and what a judge can debunk in one
// question.
func TestADieselTruckAndAnElectricMotorcycleDifferOnOneKilometre(t *testing.T) {
	truck, ok := fleet.Emission("diesel-heavy")
	if !ok {
		t.Fatal("the class table has no heavy diesel truck")
	}
	if truck.FuelKgPerKm <= 0 {
		t.Fatalf("the diesel truck class burns %v kg/km", truck.FuelKgPerKm)
	}
	motorcycle, ok := fleet.Emission("electric-light")
	if !ok {
		t.Fatal("the class table has no electric motorcycle")
	}

	truckFuel, truckCO2, ok := fleet.Burn("diesel-heavy", 1000)
	if !ok {
		t.Fatal("burning one kilometre in the truck failed")
	}
	bikeFuel, bikeCO2, ok := fleet.Burn("electric-light", 1000)
	if !ok {
		t.Fatal("burning one kilometre on the motorcycle failed")
	}

	if truckFuel <= 0 || truckCO2 <= 0 {
		t.Fatalf("truck burned %v kg and emitted %v kg, want both positive", truckFuel, truckCO2)
	}
	if bikeFuel != 0 || bikeCO2 != 0 {
		t.Fatalf("electric motorcycle burned %v kg and emitted %v kg, want tailpipe zero", bikeFuel, bikeCO2)
	}
	if truckCO2 <= bikeCO2 {
		t.Fatalf("truck emitted %v kg, motorcycle %v kg", truckCO2, bikeCO2)
	}
	// The arithmetic has to be the model, not a lookup: one kilometre at
	// 0.30 kg/km of diesel at 3.16 kg CO2/kg.
	if got, want := truckCO2, 0.948; got != want {
		t.Fatalf("truck CO2 = %v kg, want %v", got, want)
	}
	if motorcycle.CO2PerKgFuel != 0 {
		t.Fatalf("electric class carries a fuel carbon factor of %v", motorcycle.CO2PerKgFuel)
	}
}

// Every plate has to describe the same vehicle every time it is asked about, or
// a vehicle's specification would change between dispatches.
func TestAPlateAlwaysDescribesTheSameVehicle(t *testing.T) {
	for _, plate := range []string{"B-9001-JKT", "B-9042-JKT", "B-9100-JKT"} {
		first := fleet.For(plate)
		second := fleet.For(plate)
		if first != second {
			t.Fatalf("%s returned %#v then %#v", plate, first, second)
		}
		if first.Make == "" || first.Model == "" || first.Type == "" || first.Class == "" {
			t.Fatalf("%s has an incomplete profile: %#v", plate, first)
		}
		if _, ok := fleet.Emission(first.Class); !ok {
			t.Fatalf("%s names emission class %q, which the table does not have", plate, first.Class)
		}
	}
	if fleet.Name("B-9001-JKT") != "Unit 1" {
		t.Fatalf("B-9001-JKT is named %q, want Unit 1", fleet.Name("B-9001-JKT"))
	}
}

// An unknown class must be reported, not treated as zero: charging nothing for a
// vehicle nobody described would quietly understate the fleet's emissions.
func TestAnUnknownClassIsNotSilentlyFree(t *testing.T) {
	if _, _, ok := fleet.Burn("diesel-rocket", 1000); ok {
		t.Fatal("an unknown class reported a successful burn")
	}
}
