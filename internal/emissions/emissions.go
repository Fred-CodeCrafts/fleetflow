// Package emissions estimates the exhaust emissions of a trip from its distance
// and average speed.
//
// Why this exists: a fleet platform shows an "emission standard" field and an
// objective called "minimise fuel / emissions", but neither is a number. FleetFlow
// already knows every vehicle's distance and speed, so it can report grams, and
// that turns congestion-aware routing into a measurable environmental claim
// rather than a slogan: the same trip on a less congested route burns less fuel,
// and this package is what proves it.
//
// The model is COPERT's speed-dependent hot-emission curve, the same family the
// EMEP/EEA guidebook publishes and the one European inventories use:
//
//	EF(v) = (a + b·v + c·v² + d/v) / (e·v + f)        [g/km], v in km/h
//
// Each pollutant and vehicle class has its own coefficients. This package ships
// a small, documented subset for a Euro 5 petrol passenger car and a Euro 5
// diesel light commercial vehicle, which are the two classes the simulator's
// background traffic and fleet vehicles represent.
//
// What this is not: a homologation model. It ignores cold starts, road gradient,
// load, ambient temperature and the difference between urban and motorway
// drive cycles beyond what average speed captures. Those omissions are stated
// here, in the API response and in the console, because an emissions number
// that looks authoritative and is not is worse than no number.
package emissions

import (
	"errors"
	"math"
)

// Fuel is the fuel a vehicle burns, which selects the CO2 factor and the
// pollutant coefficients.
type Fuel string

const (
	Petrol Fuel = "petrol"
	Diesel Fuel = "diesel"
)

// VehicleClass is the emission class the coefficients describe.
type VehicleClass string

const (
	// PassengerCar is a Euro 5 petrol passenger car.
	PassengerCar VehicleClass = "passenger_car"
	// LightCommercial is a Euro 5 diesel light commercial vehicle under 3.5 t.
	LightCommercial VehicleClass = "light_commercial"
)

// Errors returned when a caller asks for a class this package has no
// coefficients for. They are typed so the API can answer 400 rather than
// silently reporting zero grams.
var (
	ErrUnknownClass    = errors.New("no emission coefficients for that vehicle class")
	ErrInvalidSpeed    = errors.New("speed must be finite and non-negative")
	ErrInvalidDistance = errors.New("distance must be finite and non-negative")
)

// coef holds one pollutant's COPERT coefficients.
type coef struct {
	a, b, c, d, e, f float64
	// minKmh and maxKmh bound the range the curve was fitted over. Outside it
	// the curve is extrapolation, so the result is clamped to the boundary
	// speed rather than allowed to run away: the d/v term explodes as v → 0.
	minKmh, maxKmh float64
}

// curve evaluates the COPERT function at a speed in km/h, clamped to the fitted
// range.
func (c coef) curve(kmh float64) float64 {
	v := kmh
	if v < c.minKmh {
		v = c.minKmh
	}
	if v > c.maxKmh {
		v = c.maxKmh
	}
	num := c.a + c.b*v + c.c*v*v
	if v > 0 {
		num += c.d / v
	}
	den := c.e*v + c.f
	if den == 0 {
		return 0
	}
	return num / den
}

// class describes one vehicle class: its fuel, its CO2 factor and its
// pollutant curves.
type class struct {
	fuel Fuel
	// co2PerKm is the tank-to-wheel CO2 in grams per kilometre at the reference
	// cycle, used as the base for the speed correction below.
	co2PerKm float64
	// nox, co, hc and pm are COPERT curves in g/km.
	nox, co, hc, pm coef
	// fuelPerKm is litres per 100 km at the reference cycle, for the fuel
	// figure the console shows.
	fuelPer100Km float64
}

// The coefficient tables.
//
// Values are the published COPERT 5 / EMEP-EEA hot-emission coefficients for
// Euro 5 vehicles over the 10–130 km/h fit range, rounded to the precision the
// guidebook prints. They are a starting point for a comparison, not a
// certified inventory: the point of the feature is that the *difference*
// between two routes is measured with one consistent model, so both arms of a
// comparison are computed the same way.
var classes = map[VehicleClass]class{
	PassengerCar: {
		fuel:         Petrol,
		co2PerKm:     154.7,
		fuelPer100Km: 6.6,
		nox:          coef{a: 0.2166, b: -0.0021, c: 0.0000093, d: 0.62, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 130},
		co:           coef{a: 1.042, b: -0.0113, c: 0.0000637, d: 2.31, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 130},
		hc:           coef{a: 0.625, b: -0.0075, c: 0.0000375, d: 1.02, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 130},
		pm:           coef{a: 0.0015, b: 0.0, c: 0.0, d: 0.0, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 130},
	},
	LightCommercial: {
		fuel:         Diesel,
		co2PerKm:     203.0,
		fuelPer100Km: 7.7,
		nox:          coef{a: 0.4906, b: -0.0048, c: 0.0000207, d: 1.94, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 110},
		co:           coef{a: 0.6606, b: -0.0069, c: 0.0000336, d: 2.02, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 110},
		hc:           coef{a: 0.1730, b: -0.0018, c: 0.0000087, d: 0.47, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 110},
		pm:           coef{a: 0.0230, b: -0.0002, c: 0.0000011, d: 0.05, e: 0.0, f: 1.0, minKmh: 10, maxKmh: 110},
	},
}

// co2SpeedCorrection shapes CO2 around the reference cycle.
//
// COPERT's CO2 curve is much flatter than the pollutant curves, and the useful
// property for this project is the U-shape: consumption is high in stop-start
// traffic, lowest around 60–80 km/h, and rises again with aerodynamic drag at
// motorway speed. A parabola through the reference point reproduces that shape
// with one coefficient and is honest about being an approximation.
const (
	co2BestKmh   = 70.0
	co2Curvature = 0.00022
)

// Result is one trip's estimate.
type Result struct {
	// DistanceKM and MeanSpeedKmh are the inputs, echoed so a stored result can
	// be checked without recomputing.
	DistanceKM   float64 `json:"distance_km"`
	MeanSpeedKmh float64 `json:"mean_speed_kmh"`

	// Grams of each pollutant over the trip.
	CO2G float64 `json:"co2_g"`
	NOXG float64 `json:"nox_g"`
	COG  float64 `json:"co_g"`
	HCG  float64 `json:"hc_g"`
	PMG  float64 `json:"pm_g"`

	// FuelLitres is the implied fuel burn over the trip.
	FuelLitres float64 `json:"fuel_litres"`

	// Class and Fuel say which coefficient set produced these numbers, so a
	// reader can tell a diesel van from a petrol car.
	Class VehicleClass `json:"class"`
	Fuel  Fuel         `json:"fuel"`
}

// Trip estimates one trip.
//
// Distance and speed are the only inputs because they are the only things a
// road report gives. A zero-distance trip emits nothing and is not an error;
// a negative one is.
func Trip(class VehicleClass, distanceKM, meanSpeedKmh float64) (Result, error) {
	c, ok := classes[class]
	if !ok {
		return Result{}, ErrUnknownClass
	}
	if math.IsNaN(distanceKM) || math.IsInf(distanceKM, 0) || distanceKM < 0 {
		return Result{}, ErrInvalidDistance
	}
	if math.IsNaN(meanSpeedKmh) || math.IsInf(meanSpeedKmh, 0) || meanSpeedKmh < 0 {
		return Result{}, ErrInvalidSpeed
	}

	res := Result{
		DistanceKM:   distanceKM,
		MeanSpeedKmh: meanSpeedKmh,
		Class:        class,
		Fuel:         c.fuel,
	}
	if distanceKM == 0 {
		return res, nil
	}

	// A stationary average (a vehicle that never moved) is not a division by
	// zero here: the curve clamps to its minimum speed, which is the correct
	// reading — crawling traffic emits at the worst rate the model knows.
	res.NOXG = c.nox.curve(meanSpeedKmh) * distanceKM
	res.COG = c.co.curve(meanSpeedKmh) * distanceKM
	res.HCG = c.hc.curve(meanSpeedKmh) * distanceKM
	res.PMG = c.pm.curve(meanSpeedKmh) * distanceKM
	res.CO2G = c.co2PerKm * co2Factor(meanSpeedKmh) * distanceKM
	res.FuelLitres = c.fuelPer100Km * co2Factor(meanSpeedKmh) * distanceKM / 100

	return res, nil
}

// co2Factor is the multiplier CO2 gets at a given speed relative to the
// reference cycle.
//
// It clamps to the same 10–130 km/h fit range as the pollutant curves, so every
// figure in a Result shares one lower bound. Clamping CO2 differently would mean
// a crawling vehicle's CO2 and NOx were estimated over different speed ranges,
// and the comparison between them would be meaningless.
func co2Factor(kmh float64) float64 {
	v := kmh
	if v < 10 {
		v = 10
	}
	if v > 130 {
		v = 130
	}
	d := v - co2BestKmh
	return 1 + co2Curvature*d*d
}

// Add sums a set of results, which is how a run's total is built from its trips.
//
// The mean speed of a total is not the mean of the means: it is total distance
// over total time, and time is not carried here, so the caller sets it. Summing
// everything else is exact.
func Add(results []Result) Result {
	var total Result
	for _, r := range results {
		total.DistanceKM += r.DistanceKM
		total.CO2G += r.CO2G
		total.NOXG += r.NOXG
		total.COG += r.COG
		total.HCG += r.HCG
		total.PMG += r.PMG
		total.FuelLitres += r.FuelLitres
	}
	return total
}

// Method is the disclaimer the API attaches to every emissions response, so a
// client cannot present the number as a certified inventory.
const Method = "COPERT 5 speed-dependent hot-emission curves for Euro 5 vehicles. " +
	"Excludes cold starts, gradient, payload and ambient temperature; " +
	"compare routes within this model rather than treating the value as a certified inventory."
