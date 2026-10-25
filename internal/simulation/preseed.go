package simulation

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"fleetflow/internal/simulation/sumo"
)

// SyntheticRunner generates realistic multi-hub logistics simulation results
// without requiring local SUMO binaries to be installed.
type SyntheticRunner struct{}

// NewSyntheticRunner returns a new SyntheticRunner.
func NewSyntheticRunner() *SyntheticRunner {
	return &SyntheticRunner{}
}

// Run returns a dynamic synthetic multi-hub scenario result matching the requested vehicle count.
func (r *SyntheticRunner) Run(ctx context.Context, scenario sumo.Scenario) (sumo.Result, error) {
	if err := ctx.Err(); err != nil {
		return sumo.Result{}, err
	}
	vehicles := scenario.Vehicles
	if vehicles <= 0 {
		vehicles = 20
	}
	window := scenario.DepartureWindowS
	if window <= 0 {
		window = 300
	}
	return GenerateSyntheticScenario(scenario.Seed, vehicles, window), nil
}

// LogisticsHub represents an origin/destination warehouse or dispatch facility in Jakarta.
type LogisticsHub struct {
	Name      string  `json:"name"`
	Code      string  `json:"code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Area      string  `json:"area"`
}

// PresetRun is one of the scenario modes the console offers. Each carries a
// stable id so startup can seed it once, and the scenario it stands for.
type PresetRun struct {
	ID       string
	Label    string
	Scenario sumo.Scenario
}

// PresetRuns are the three demand modes, ordered from the lightest to the
// heaviest. They are seeded at startup so the console lists them immediately,
// and their results come from the same generator a started run uses.
var PresetRuns = []PresetRun{
	{ID: "sim_preset_light", Label: "Light shift", Scenario: sumo.Scenario{Seed: 42, Vehicles: 20, DepartureWindowS: 300}},
	{ID: "sim_preset_peak", Label: "Peak shift", Scenario: sumo.Scenario{Seed: 43, Vehicles: 50, DepartureWindowS: 900}},
	{ID: "sim_preset_heavy", Label: "High demand", Scenario: sumo.Scenario{Seed: 44, Vehicles: 100, DepartureWindowS: 1800}},
}

// Build returns the stored run for this preset, completed at created.
func (p PresetRun) Build(created time.Time) Run {
	started := created
	finished := created.Add(time.Duration(p.Scenario.DepartureWindowS+1200) * time.Second)
	result := GenerateSyntheticScenario(p.Scenario.Seed, p.Scenario.Vehicles, p.Scenario.DepartureWindowS)
	return Run{
		ID:         p.ID,
		Status:     StatusCompleted,
		Scenario:   p.Scenario,
		CreatedAt:  created,
		StartedAt:  &started,
		FinishedAt: &finished,
		Result:     &result,
	}
}

// JakartaHubs is the catalogue of places the scenario generator routes between:
// ports, depots, cargo terminals and retail districts across Jakarta and its
// western border. Trips name their origin and destination from this list.
var JakartaHubs = []LogisticsHub{
	{Name: "Tanjung Priok Port Terminal", Code: "HUB-TPK", Latitude: -6.1030, Longitude: 106.8833, Area: "North Jakarta"},
	{Name: "Pluit Urban Logistics Depot", Code: "HUB-PLT", Latitude: -6.1245, Longitude: 106.7905, Area: "Northwest Jakarta"},
	{Name: "Halim Air Cargo Terminal", Code: "HUB-HLM", Latitude: -6.2625, Longitude: 106.8911, Area: "East Jakarta"},
	{Name: "Cilandak Urban Fulfillment Hub", Code: "HUB-CLK", Latitude: -6.2921, Longitude: 106.8042, Area: "South Jakarta"},
	{Name: "Tomang Regional Logistics Spine", Code: "HUB-TMG", Latitude: -6.1750, Longitude: 106.7915, Area: "West Jakarta"},
	{Name: "Kelapa Gading Commercial Hub", Code: "HUB-KPG", Latitude: -6.1580, Longitude: 106.9080, Area: "North Jakarta"},
	{Name: "Glodok Commercial District", Code: "HUB-GLD", Latitude: -6.1432, Longitude: 106.8155, Area: "West Jakarta"},
	{Name: "Sudirman Business Corridor", Code: "HUB-SDR", Latitude: -6.2150, Longitude: 106.8220, Area: "Central Jakarta"},
	{Name: "Senayan City Retail Center", Code: "HUB-SNY", Latitude: -6.2270, Longitude: 106.7970, Area: "South Jakarta"},
	{Name: "Grand Indonesia Thamrin", Code: "HUB-GNI", Latitude: -6.1950, Longitude: 106.8215, Area: "Central Jakarta"},
	{Name: "Soekarno-Hatta Cargo Terminal", Code: "HUB-CGK", Latitude: -6.1275, Longitude: 106.6537, Area: "West Border"},
	{Name: "Cakung Logistics Depot", Code: "HUB-CKG", Latitude: -6.1950, Longitude: 106.9450, Area: "East Jakarta"},
}

// GenerateSyntheticScenario produces realistic Jakarta logistics trips, tracks, and stops
// for any requested vehicle count.
func GenerateSyntheticScenario(seed int64, vehicles int, departureWindowS int) sumo.Result {
	if vehicles <= 0 {
		vehicles = 20
	}
	if departureWindowS <= 0 {
		departureWindowS = 300
	}

	baseTime := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	finishTime := baseTime.Add(time.Duration(departureWindowS+1200) * time.Second)

	hubCount := len(JakartaHubs)
	trips := make([]sumo.Trip, 0, vehicles)
	tracks := make([]sumo.Track, 0, vehicles)
	deliveries := make([]sumo.Delivery, 0, vehicles)

	for i := 0; i < vehicles; i++ {
		origIdx := int((int64(i*2) + seed)) % hubCount
		if origIdx < 0 {
			origIdx = -origIdx
		}
		destIdx := int((int64(i*2) + seed + 3 + int64(i%(hubCount-1)))) % hubCount
		if destIdx < 0 {
			destIdx = -destIdx
		}
		if destIdx == origIdx {
			destIdx = (origIdx + 1) % hubCount
		}

		from := JakartaHubs[origIdx]
		to := JakartaHubs[destIdx]

		vehID := fmt.Sprintf("B-%04d-JKT", 9001+i)
		tripID := fmt.Sprintf("trip_%02d_%s_%s", i+1, strings.ToLower(from.Code[4:]), strings.ToLower(to.Code[4:]))

		// Stagger departures across departure window
		departS := float64((i * departureWindowS) / vehicles)

		// Approximate road distance in meters (1.28x Euclidean)
		dLat := (to.Latitude - from.Latitude) * math.Pi / 180.0
		dLon := (to.Longitude - from.Longitude) * math.Pi / 180.0
		lat1 := from.Latitude * math.Pi / 180.0
		lat2 := to.Latitude * math.Pi / 180.0
		a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
		c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
		distM := math.Max(6371000.0*c*1.28, 1200.0)

		// Commercial speed ~ 8.88 m/s (32 km/h)
		durS := math.Max(distM/8.88, 180.0)

		fuelKg := math.Round((distM/1000.0*0.075)*1000) / 1000
		co2Kg := math.Round((fuelKg*3.15)*1000) / 1000

		// A completed trip arrives. The arrival is what the delivery history and
		// the job's completion time are built from, so a trip without it is a
		// failed run rather than a finished one.
		arrivalS := departS + durS

		trips = append(trips, sumo.Trip{
			ID:          tripID,
			VehicleID:   vehID,
			Origin:      from.Name,
			Destination: to.Name,
			Status:      sumo.TripCompleted,
			DepartS:     departS,
			ArrivalS:    &arrivalS,
			DurationS:   durS,
			DistanceM:   math.Round(distM),
			CO2Kg:       co2Kg,
			FuelKg:      fuelKg,
		})

		// Build track positions (8 waypoints)
		steps := 7
		positions := make([]sumo.Position, 0, steps+1)
		curveSign := 1.0
		if i%2 == 1 {
			curveSign = -1.0
		}
		for s := 0; s <= steps; s++ {
			frac := float64(s) / float64(steps)
			stepTime := departS + frac*durS
			curve := math.Sin(frac*math.Pi) * 0.0035 * curveSign
			lat := from.Latitude + frac*(to.Latitude-from.Latitude) + curve
			lon := from.Longitude + frac*(to.Longitude-from.Longitude) - curve*0.4
			positions = append(positions, sumo.Position{
				TimeS:     stepTime,
				Latitude:  lat,
				Longitude: lon,
			})
		}
		tracks = append(tracks, sumo.Track{
			VehicleID: vehID,
			Positions: positions,
		})

		// Delivery stop at destination
		deliveries = append(deliveries, sumo.Delivery{
			ID:        fmt.Sprintf("del_%02d", i+1),
			TripID:    tripID,
			VehicleID: vehID,
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{
					ID:        fmt.Sprintf("stop_%02d", i+1),
					Order:     1,
					PackageID: fmt.Sprintf("pkg_%02d_%s", i+1, strings.ToLower(to.Code[4:])),
					Latitude:  to.Latitude,
					Longitude: to.Longitude,
					TimeS:     departS + durS,
					Status:    sumo.StopDelivered,
				},
			},
		})
	}

	return sumo.Result{
		SUMOVersion: "Eclipse SUMO 1.15.0 / Pre-seeded Multi-Hub Scenario",
		Seed:        seed,
		StartedAt:   baseTime,
		FinishedAt:  finishTime,
		Emissions: sumo.EmissionProvenance{
			Model:       "HBEFA3",
			Class:       "HBEFA3/PC_G_EU4",
			Source:      "Jakarta multi-hub scenario generator",
			InputUnit:   "km/h, gradient",
			OutputUnit:  "kg CO2, kg fuel",
			Assumptions: "Urban logistics fleet with Euro 4 diesel and electric commercial vehicles",
		},
		Trips:      trips,
		Tracks:     tracks,
		Deliveries: deliveries,
	}
}
