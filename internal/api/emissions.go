package api

import (
	"errors"
	"net/http"
	"strconv"

	"fleetflow/internal/domain"
	"fleetflow/internal/emissions"
)

// emissionsResponse is the body of GET /api/emissions.
//
// It answers two questions a fleet operator actually asks: what did the trips I
// ran emit, and what would a given trip emit. The run figures come from the live
// comparison when one is going, and the per-trip figure is computed on demand.
//
// Every response carries Method, because these are estimates under a documented
// model, not measurements. A client that shows the grams without it is
// overstating what this service knows.
type emissionsResponse struct {
	// Run is the live comparison's emissions, one entry per arm, or null when no
	// run is going.
	Run *emissionsRunView `json:"run"`
	// Trip is the estimate for the query parameters, when the request asked for
	// one.
	Trip *emissionsTripView `json:"trip"`
	// Classes lists the vehicle classes this service can estimate, so a client
	// can offer the choice instead of guessing at the names.
	Classes []string `json:"classes"`
	Method  string   `json:"method"`
}

// emissionsRunView is both arms' emissions side by side.
type emissionsRunView struct {
	Seed       int64            `json:"seed"`
	Vehicles   int              `json:"vehicles"`
	FleetShare float64          `json:"fleet_share"`
	Baseline   emissionsArmView `json:"baseline"`
	FleetFlow  emissionsArmView `json:"fleet_flow"`
	// Savings is baseline minus fleet_flow, so a positive number means the
	// dynamic arm emitted less. It is reported explicitly rather than left to
	// the client because the sign is the whole point and getting it backwards
	// would invert the claim.
	Savings emissionsArmView `json:"savings"`
}

// emissionsArmView is one arm's figures plus the distance they cover.
type emissionsArmView struct {
	DistanceKM float64 `json:"distance_km"`
	CO2G       float64 `json:"co2_g"`
	NOXG       float64 `json:"nox_g"`
	COG        float64 `json:"co_g"`
	HCG        float64 `json:"hc_g"`
	PMG        float64 `json:"pm_g"`
	FuelLitres float64 `json:"fuel_litres"`
	// CO2PerKM is the intensity, which is the fair way to compare arms whose
	// trips may have covered different distances.
	CO2PerKM float64 `json:"co2_per_km"`
}

// emissionsTripView is one trip's estimate, echoing the inputs it used.
type emissionsTripView struct {
	Class        string  `json:"class"`
	Fuel         string  `json:"fuel"`
	DistanceKM   float64 `json:"distance_km"`
	MeanSpeedKMH float64 `json:"mean_speed_kmh"`
	CO2G         float64 `json:"co2_g"`
	NOXG         float64 `json:"nox_g"`
	COG          float64 `json:"co_g"`
	HCG          float64 `json:"hc_g"`
	PMG          float64 `json:"pm_g"`
	FuelLitres   float64 `json:"fuel_litres"`
	CO2PerKM     float64 `json:"co2_per_km"`
}

// emissionsClassNames is the wire form of every class the model supports. It is
// written out rather than derived from the map so the order is stable, which
// makes the response diffable in a test.
var emissionsClassNames = []string{
	string(emissions.PassengerCar),
	string(emissions.LightCommercial),
}

// handleEmissions answers GET /api/emissions.
//
// Query parameters, all optional:
//
//	class     vehicle class (passenger_car, light_commercial)
//	distance  trip distance in kilometres
//	speed     mean trip speed in km/h
//
// With distance and speed it returns one trip's estimate; without them it
// returns only the live run's figures. Both may be present.
func (s *Server) handleEmissions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	body := emissionsResponse{
		Classes: emissionsClassNames,
		Method:  emissions.Method,
	}

	// A trip estimate is computed only when the caller asked for one. A distance
	// of zero is a legitimate answer, so presence is decided by the parameter
	// being there at all rather than by its value.
	_, hasDistance := q["distance"]
	_, hasSpeed := q["speed"]
	if hasDistance || hasSpeed {
		class := emissions.PassengerCar
		if name := q.Get("class"); name != "" {
			class = emissions.VehicleClass(name)
		}
		distance, err := queryFloat(q.Get("distance"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		speed, err := queryFloat(q.Get("speed"))
		if err != nil {
			writeError(w, r, err)
			return
		}

		est, err := emissions.Trip(class, distance, speed)
		if err != nil {
			writeError(w, r, emissionsInputError(err))
			return
		}

		body.Trip = &emissionsTripView{
			Class:        string(est.Class),
			Fuel:         string(est.Fuel),
			DistanceKM:   est.DistanceKM,
			MeanSpeedKMH: est.MeanSpeedKmh,
			CO2G:         est.CO2G,
			NOXG:         est.NOXG,
			COG:          est.COG,
			HCG:          est.HCG,
			PMG:          est.PMG,
			FuelLitres:   est.FuelLitres,
			CO2PerKM:     perKM(est.CO2G, est.DistanceKM),
		}
	}

	if live := s.currentRun(); live != nil {
		m := live.metrics()
		baseline := armEmissionsOf(m.Baseline)
		fleet := armEmissionsOf(m.FleetFlow)
		body.Run = &emissionsRunView{
			Seed:       m.Seed,
			Vehicles:   m.Vehicles,
			FleetShare: m.FleetShare,
			Baseline:   baseline,
			FleetFlow:  fleet,
			Savings: emissionsArmView{
				DistanceKM: baseline.DistanceKM - fleet.DistanceKM,
				CO2G:       baseline.CO2G - fleet.CO2G,
				NOXG:       baseline.NOXG - fleet.NOXG,
				COG:        baseline.COG - fleet.COG,
				HCG:        baseline.HCG - fleet.HCG,
				PMG:        baseline.PMG - fleet.PMG,
				FuelLitres: baseline.FuelLitres - fleet.FuelLitres,
				CO2PerKM:   baseline.CO2PerKM - fleet.CO2PerKM,
			},
		}
	}

	writeJSON(w, http.StatusOK, body)
}

func emissionsInputError(err error) error {
	switch {
	case errors.Is(err, emissions.ErrUnknownClass):
		return domain.Invalid("emissions_class_unknown", "class must be passenger_car or light_commercial")
	case errors.Is(err, emissions.ErrInvalidDistance):
		return domain.Invalid("emissions_distance_invalid", "distance must be finite and non-negative")
	case errors.Is(err, emissions.ErrInvalidSpeed):
		return domain.Invalid("emissions_speed_invalid", "speed must be finite and non-negative")
	default:
		return err
	}
}

// armEmissionsOf converts one arm's metrics into the emissions view.
func armEmissionsOf(m armMetrics) emissionsArmView {
	return emissionsArmView{
		DistanceKM: m.DistanceKM,
		CO2G:       m.Emissions.CO2G,
		NOXG:       m.Emissions.NOXG,
		COG:        m.Emissions.COG,
		HCG:        m.Emissions.HCG,
		PMG:        m.Emissions.PMG,
		FuelLitres: m.Emissions.FuelLitres,
		CO2PerKM:   perKM(m.Emissions.CO2G, m.DistanceKM),
	}
}

// perKM returns grams per kilometre, or zero when there is no distance to divide
// by. Zero is the honest answer for "no trips finished yet": a ratio with no
// denominator is not a number, and reporting infinity or NaN in JSON would break
// a client that parses it.
func perKM(grams, km float64) float64 {
	if km <= 0 {
		return 0
	}
	return grams / km
}

// codeQueryNumberInvalid is reported when a numeric query parameter is present
// but is not a number.
const codeQueryNumberInvalid = "query_number_invalid"

// queryFloat parses an optional numeric query parameter.
//
// A missing parameter is zero, because the caller decides whether it needed one.
// A present but unparseable value is an error rather than a silent zero: a
// request asking for "distance=abc" has made a mistake, and answering as though
// it asked for no distance would hide it.
func queryFloat(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, domain.Invalid(codeQueryNumberInvalid, "query parameter %q is not a number", raw)
	}
	return v, nil
}
