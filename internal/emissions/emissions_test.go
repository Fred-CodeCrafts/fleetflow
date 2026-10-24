package emissions

import (
	"errors"
	"math"
	"testing"
)

// TestTrip_StoppedTrafficEmitsMoreThanFreeFlow is the property the whole feature
// rests on: congestion costs fuel. If this ever inverts, the environmental claim
// in the console is wrong and the tests must fail.
func TestTrip_StoppedTrafficEmitsMoreThanFreeFlow(t *testing.T) {
	const km = 10.0

	jammed, err := Trip(PassengerCar, km, 10)
	if err != nil {
		t.Fatalf("estimating at 10 km/h: %v", err)
	}
	flowing, err := Trip(PassengerCar, km, 60)
	if err != nil {
		t.Fatalf("estimating at 60 km/h: %v", err)
	}

	if jammed.CO2G <= flowing.CO2G {
		t.Errorf("10 km/h emitted %.1f g CO2 and 60 km/h emitted %.1f g; congested traffic must emit more",
			jammed.CO2G, flowing.CO2G)
	}
	if jammed.NOXG <= flowing.NOXG {
		t.Errorf("10 km/h emitted %.2f g NOx and 60 km/h emitted %.2f g; congested traffic must emit more",
			jammed.NOXG, flowing.NOXG)
	}
	if jammed.FuelLitres <= flowing.FuelLitres {
		t.Errorf("10 km/h burned %.3f L and 60 km/h burned %.3f L; congested traffic must burn more",
			jammed.FuelLitres, flowing.FuelLitres)
	}
}

// TestTrip_ScalesWithDistance pins linearity, which makes the per-trip results
// addable into a run total.
func TestTrip_ScalesWithDistance(t *testing.T) {
	one, err := Trip(PassengerCar, 5, 40)
	if err != nil {
		t.Fatal(err)
	}
	ten, err := Trip(PassengerCar, 50, 40)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(ten.CO2G-10*one.CO2G) > 1e-6 {
		t.Errorf("50 km emitted %.3f g and 5 km emitted %.3f g; want exactly ten times", ten.CO2G, one.CO2G)
	}
	if math.Abs(ten.FuelLitres-10*one.FuelLitres) > 1e-9 {
		t.Errorf("fuel is not linear in distance: %.4f vs %.4f", ten.FuelLitres, one.FuelLitres)
	}
}

// TestTrip_DieselVanEmitsMoreCO2ThanPetrolCar checks the class table is actually
// consulted rather than one shared constant.
func TestTrip_DieselVanEmitsMoreCO2ThanPetrolCar(t *testing.T) {
	car, err := Trip(PassengerCar, 20, 50)
	if err != nil {
		t.Fatal(err)
	}
	van, err := Trip(LightCommercial, 20, 50)
	if err != nil {
		t.Fatal(err)
	}

	if van.CO2G <= car.CO2G {
		t.Errorf("van emitted %.1f g and car emitted %.1f g; a diesel LCV must emit more per km", van.CO2G, car.CO2G)
	}
	if van.Fuel != Diesel || car.Fuel != Petrol {
		t.Errorf("fuels reported as %q and %q, want diesel and petrol", van.Fuel, car.Fuel)
	}
}

// TestTrip_ZeroDistanceIsZeroNotAnError keeps an empty trip out of the error
// path, because a run can legitimately contain a vehicle that never moved.
func TestTrip_ZeroDistanceIsZeroNotAnError(t *testing.T) {
	res, err := Trip(PassengerCar, 0, 0)
	if err != nil {
		t.Fatalf("zero-distance trip returned an error: %v", err)
	}
	if res.CO2G != 0 || res.FuelLitres != 0 {
		t.Errorf("zero-distance trip emitted %+v, want zeros", res)
	}
}

// TestTrip_RejectsBadInput pins the three inputs that must not be estimated,
// so a caller gets a typed error instead of a plausible-looking number.
func TestTrip_RejectsBadInput(t *testing.T) {
	if _, err := Trip(VehicleClass("hovercraft"), 1, 10); !errors.Is(err, ErrUnknownClass) {
		t.Errorf("unknown class: got %v, want ErrUnknownClass", err)
	}
	if _, err := Trip(PassengerCar, -1, 10); !errors.Is(err, ErrInvalidDistance) {
		t.Errorf("negative distance: got %v, want ErrInvalidDistance", err)
	}
	if _, err := Trip(PassengerCar, 1, -10); !errors.Is(err, ErrInvalidSpeed) {
		t.Errorf("negative speed: got %v, want ErrInvalidSpeed", err)
	}
	if _, err := Trip(PassengerCar, math.NaN(), 10); !errors.Is(err, ErrInvalidDistance) {
		t.Errorf("NaN distance: got %v, want ErrInvalidDistance", err)
	}
	if _, err := Trip(PassengerCar, 1, math.Inf(1)); !errors.Is(err, ErrInvalidSpeed) {
		t.Errorf("infinite speed: got %v, want ErrInvalidSpeed", err)
	}
}

// TestTrip_SpeedOutsideFitRangeIsClamped proves the d/v term cannot run away at
// a crawl. A stationary average is the case that would otherwise divide by a
// very small number and report an absurd emission.
func TestTrip_SpeedOutsideFitRangeIsClamped(t *testing.T) {
	atZero, err := Trip(PassengerCar, 1, 0)
	if err != nil {
		t.Fatalf("zero speed: %v", err)
	}
	atMin, err := Trip(PassengerCar, 1, 10)
	if err != nil {
		t.Fatalf("10 km/h: %v", err)
	}
	if math.Abs(atZero.CO2G-atMin.CO2G) > 1e-9 {
		t.Errorf("0 km/h emitted %.3f g and 10 km/h emitted %.3f g; below the fit range must clamp", atZero.CO2G, atMin.CO2G)
	}

	atFast, err := Trip(PassengerCar, 1, 400)
	if err != nil {
		t.Fatalf("400 km/h: %v", err)
	}
	atMax, err := Trip(PassengerCar, 1, 130)
	if err != nil {
		t.Fatalf("130 km/h: %v", err)
	}
	if math.Abs(atFast.CO2G-atMax.CO2G) > 1e-9 {
		t.Errorf("400 km/h emitted %.3f g and 130 km/h emitted %.3f g; above the fit range must clamp", atFast.CO2G, atMax.CO2G)
	}
}

// TestAdd_SumsTotals pins the aggregate used for a whole run.
func TestAdd_SumsTotals(t *testing.T) {
	a, err := Trip(PassengerCar, 10, 30)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Trip(LightCommercial, 20, 70)
	if err != nil {
		t.Fatal(err)
	}

	total := Add([]Result{a, b})
	if math.Abs(total.DistanceKM-30) > 1e-9 {
		t.Errorf("total distance = %f, want 30", total.DistanceKM)
	}
	if math.Abs(total.CO2G-(a.CO2G+b.CO2G)) > 1e-9 {
		t.Errorf("total CO2 = %f, want %f", total.CO2G, a.CO2G+b.CO2G)
	}
	if math.Abs(total.FuelLitres-(a.FuelLitres+b.FuelLitres)) > 1e-12 {
		t.Errorf("total fuel = %f, want %f", total.FuelLitres, a.FuelLitres+b.FuelLitres)
	}

	if empty := Add(nil); empty.CO2G != 0 || empty.DistanceKM != 0 {
		t.Errorf("adding nothing gave %+v, want zeros", empty)
	}
}
