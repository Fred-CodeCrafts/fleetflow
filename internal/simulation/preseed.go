package simulation

import (
	"context"
	"time"

	"fleetflow/internal/simulation/sumo"
)

// PreseededRunID is the stable identifier for the default pre-seeded Jakarta logistics run.
const PreseededRunID = "sim_jakarta_logistics_seed"

// SyntheticRunner generates realistic multi-hub logistics simulation results
// without requiring local SUMO binaries to be installed.
type SyntheticRunner struct{}

// NewSyntheticRunner returns a new SyntheticRunner.
func NewSyntheticRunner() *SyntheticRunner {
	return &SyntheticRunner{}
}

// Run returns the precomputed multi-hub scenario result.
func (r *SyntheticRunner) Run(ctx context.Context, scenario sumo.Scenario) (sumo.Result, error) {
	if err := ctx.Err(); err != nil {
		return sumo.Result{}, err
	}
	run := DefaultPreseededRun()
	result := *run.Result
	result.Seed = scenario.Seed
	return result, nil
}

// LogisticsHub represents an origin/destination warehouse or dispatch facility in Jakarta.
type LogisticsHub struct {
	Name      string  `json:"name"`
	Code      string  `json:"code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Area      string  `json:"area"`
}

// JakartaLogisticsHubs provides the five primary operational hubs across Jakarta.
var JakartaLogisticsHubs = []LogisticsHub{
	{Name: "Tanjung Priok Port Terminal", Code: "HUB-TPK", Latitude: -6.1030, Longitude: 106.8833, Area: "North Jakarta"},
	{Name: "Pluit Urban Logistics Depot", Code: "HUB-PLT", Latitude: -6.1245, Longitude: 106.7905, Area: "Northwest Jakarta"},
	{Name: "Halim Air Cargo Terminal", Code: "HUB-HLM", Latitude: -6.2625, Longitude: 106.8911, Area: "East Jakarta"},
	{Name: "Cilandak Urban Fulfillment Hub", Code: "HUB-CLK", Latitude: -6.2921, Longitude: 106.8042, Area: "South Jakarta"},
	{Name: "Tomang Regional Logistics Spine", Code: "HUB-TMG", Latitude: -6.1750, Longitude: 106.7915, Area: "West Jakarta"},
}

// DefaultPreseededRun returns a pre-computed Jakarta simulation run featuring
// multi-hub logistics routes, realistic tracks, delivery stops, and emissions.
func DefaultPreseededRun() Run {
	baseTime := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	finishTime := baseTime.Add(25 * time.Minute)

	trips := []sumo.Trip{
		{
			ID:        "trip_tpk_kpg",
			Status:    sumo.TripCompleted,
			DepartS:   10,
			DurationS: 850,
			DistanceM: 8420,
			CO2Kg:     1.82,
			FuelKg:    0.58,
		},
		{
			ID:        "trip_plt_gld",
			Status:    sumo.TripCompleted,
			DepartS:   25,
			DurationS: 620,
			DistanceM: 5200,
			CO2Kg:     1.15,
			FuelKg:    0.37,
		},
		{
			ID:        "trip_hlm_sdr",
			Status:    sumo.TripCompleted,
			DepartS:   40,
			DurationS: 1100,
			DistanceM: 11350,
			CO2Kg:     2.45,
			FuelKg:    0.78,
		},
		{
			ID:        "trip_clk_sny",
			Status:    sumo.TripCompleted,
			DepartS:   60,
			DurationS: 980,
			DistanceM: 9140,
			CO2Kg:     1.98,
			FuelKg:    0.63,
		},
		{
			ID:        "trip_tmg_gni",
			Status:    sumo.TripCompleted,
			DepartS:   80,
			DurationS: 540,
			DistanceM: 4680,
			CO2Kg:     0.94,
			FuelKg:    0.30,
		},
	}

	tracks := []sumo.Track{
		// Vehicle 1: Isuzu Giga (Tanjung Priok Port -> Kelapa Gading Commercial)
		{
			VehicleID: "B-9012-JKT",
			Positions: []sumo.Position{
				{TimeS: 0, Latitude: -6.1030, Longitude: 106.8833},
				{TimeS: 60, Latitude: -6.1115, Longitude: 106.8872},
				{TimeS: 120, Latitude: -6.1202, Longitude: 106.8910},
				{TimeS: 180, Latitude: -6.1288, Longitude: 106.8955},
				{TimeS: 240, Latitude: -6.1362, Longitude: 106.8998},
				{TimeS: 300, Latitude: -6.1435, Longitude: 106.9030},
				{TimeS: 360, Latitude: -6.1500, Longitude: 106.9055},
				{TimeS: 420, Latitude: -6.1575, Longitude: 106.9080},
			},
		},
		// Vehicle 2: Mitsubishi Fuso (Pluit Depot -> Glodok Commercial)
		{
			VehicleID: "B-9431-JKT",
			Positions: []sumo.Position{
				{TimeS: 0, Latitude: -6.1245, Longitude: 106.7905},
				{TimeS: 60, Latitude: -6.1280, Longitude: 106.7960},
				{TimeS: 120, Latitude: -6.1315, Longitude: 106.8020},
				{TimeS: 180, Latitude: -6.1350, Longitude: 106.8080},
				{TimeS: 240, Latitude: -6.1390, Longitude: 106.8120},
				{TimeS: 300, Latitude: -6.1432, Longitude: 106.8155},
			},
		},
		// Vehicle 3: Toyota HiAce (Halim Logistics -> Sudirman Corridor)
		{
			VehicleID: "B-9204-JKT",
			Positions: []sumo.Position{
				{TimeS: 0, Latitude: -6.2625, Longitude: 106.8911},
				{TimeS: 60, Latitude: -6.2550, Longitude: 106.8800},
				{TimeS: 120, Latitude: -6.2480, Longitude: 106.8680},
				{TimeS: 180, Latitude: -6.2410, Longitude: 106.8550},
				{TimeS: 240, Latitude: -6.2330, Longitude: 106.8430},
				{TimeS: 300, Latitude: -6.2250, Longitude: 106.8320},
				{TimeS: 360, Latitude: -6.2190, Longitude: 106.8260},
				{TimeS: 420, Latitude: -6.2150, Longitude: 106.8220},
			},
		},
		// Vehicle 4: Daihatsu Gran Max (Cilandak Hub -> Senayan City)
		{
			VehicleID: "B-9550-JKT",
			Positions: []sumo.Position{
				{TimeS: 0, Latitude: -6.2921, Longitude: 106.8042},
				{TimeS: 60, Latitude: -6.2810, Longitude: 106.8025},
				{TimeS: 120, Latitude: -6.2700, Longitude: 106.8000},
				{TimeS: 180, Latitude: -6.2580, Longitude: 106.7980},
				{TimeS: 240, Latitude: -6.2470, Longitude: 106.7975},
				{TimeS: 300, Latitude: -6.2360, Longitude: 106.7972},
				{TimeS: 360, Latitude: -6.2270, Longitude: 106.7970},
			},
		},
		// Vehicle 5: Honda PCX Electric (Tomang Spine -> Grand Indonesia Thamrin)
		{
			VehicleID: "B-9118-JKT",
			Positions: []sumo.Position{
				{TimeS: 0, Latitude: -6.1750, Longitude: 106.7915},
				{TimeS: 60, Latitude: -6.1775, Longitude: 106.7980},
				{TimeS: 120, Latitude: -6.1810, Longitude: 106.8050},
				{TimeS: 180, Latitude: -6.1850, Longitude: 106.8120},
				{TimeS: 240, Latitude: -6.1900, Longitude: 106.8170},
				{TimeS: 300, Latitude: -6.1950, Longitude: 106.8215},
			},
		},
	}

	deliveries := []sumo.Delivery{
		{
			ID:        "del_01",
			TripID:    "trip_tpk_kpg",
			VehicleID: "B-9012-JKT",
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{ID: "stop_01", Order: 1, PackageID: "pkg_kpg_retail", Latitude: -6.1575, Longitude: 106.9080, TimeS: 420, Status: sumo.StopDelivered},
			},
		},
		{
			ID:        "del_02",
			TripID:    "trip_plt_gld",
			VehicleID: "B-9431-JKT",
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{ID: "stop_02", Order: 1, PackageID: "pkg_gld_electronics", Latitude: -6.1432, Longitude: 106.8155, TimeS: 300, Status: sumo.StopDelivered},
			},
		},
		{
			ID:        "del_03",
			TripID:    "trip_hlm_sdr",
			VehicleID: "B-9204-JKT",
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{ID: "stop_03", Order: 1, PackageID: "pkg_sdr_express", Latitude: -6.2150, Longitude: 106.8220, TimeS: 420, Status: sumo.StopDelivered},
			},
		},
		{
			ID:        "del_04",
			TripID:    "trip_clk_sny",
			VehicleID: "B-9550-JKT",
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{ID: "stop_04", Order: 1, PackageID: "pkg_sny_parcels", Latitude: -6.2270, Longitude: 106.7970, TimeS: 360, Status: sumo.StopDelivered},
			},
		},
		{
			ID:        "del_05",
			TripID:    "trip_tmg_gni",
			VehicleID: "B-9118-JKT",
			Status:    sumo.DeliveryCompleted,
			Stops: []sumo.DeliveryStop{
				{ID: "stop_05", Order: 1, PackageID: "pkg_gni_grocery", Latitude: -6.1950, Longitude: 106.8215, TimeS: 300, Status: sumo.StopDelivered},
			},
		},
	}

	return Run{
		ID:        PreseededRunID,
		Status:    StatusCompleted,
		Scenario:  sumo.Scenario{Seed: 101, Vehicles: 5, DepartureWindowS: 120},
		CreatedAt: baseTime,
		StartedAt: &baseTime,
		FinishedAt: &finishTime,
		Result: &sumo.Result{
			SUMOVersion: "Eclipse SUMO 1.15.0 / Pre-seeded Multi-Hub Scenario",
			Seed:        101,
			StartedAt:   baseTime,
			FinishedAt:  finishTime,
			Emissions: sumo.EmissionProvenance{
				Model:       "HBEFA3",
				Class:       "HBEFA3/PC_G_EU4",
				Source:      "Pre-seeded multi-hub logistics telemetry",
				InputUnit:   "km/h, gradient",
				OutputUnit:  "kg CO2, kg fuel",
				Assumptions: "Urban logistics fleet with Euro 4 diesel and electric commercial vehicles",
			},
			Trips:      trips,
			Tracks:     tracks,
			Deliveries: deliveries,
		},
	}
}
