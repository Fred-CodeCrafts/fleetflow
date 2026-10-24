package sumo

import (
	"errors"
	"fmt"
	"time"
)

const (
	maxVehicles     = 500
	maxSeed         = 2147483647
	maxDepartureWin = 24 * 60 * 60

	defaultVehicleTypeID = "fleetflow_passenger"
	emissionModelName    = "HBEFA3"
	emissionClassName    = "HBEFA3/PC_G_EU4"
)

var errScenarioInvalid = errors.New("invalid SUMO scenario")

type Scenario struct {
	Seed             int64 `json:"seed"`
	Vehicles         int   `json:"vehicles"`
	DepartureWindowS int   `json:"departure_window_s"`
}

func (s Scenario) Validate() error {
	if s.Seed < 0 || s.Seed > maxSeed {
		return fmt.Errorf("%w: seed must be between 0 and %d", errScenarioInvalid, maxSeed)
	}
	if s.Vehicles < 1 || s.Vehicles > maxVehicles {
		return fmt.Errorf("%w: vehicles must be between 1 and %d", errScenarioInvalid, maxVehicles)
	}
	if s.DepartureWindowS < 60 || s.DepartureWindowS > maxDepartureWin {
		return fmt.Errorf("%w: departure window must be between 60 and %d seconds", errScenarioInvalid, maxDepartureWin)
	}
	return nil
}

type TripStatus string

const (
	TripRunning   TripStatus = "running"
	TripCompleted TripStatus = "completed"
)

type Trip struct {
	ID        string     `json:"id"`
	Status    TripStatus `json:"status"`
	DepartS   float64    `json:"depart_s"`
	ArrivalS  *float64   `json:"arrival_s,omitempty"`
	DurationS float64    `json:"duration_s"`
	DistanceM float64    `json:"distance_m"`
	CO2Kg     float64    `json:"co2_kg"`
	FuelKg    float64    `json:"fuel_kg"`
}

type Position struct {
	TimeS     float64 `json:"time_s"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type Track struct {
	VehicleID string     `json:"vehicle_id"`
	Positions []Position `json:"positions"`
}

type DeliveryStatus string

const (
	DeliveryInProgress DeliveryStatus = "in_progress"
	DeliveryCompleted  DeliveryStatus = "completed"
)

type StopStatus string

const (
	StopPending   StopStatus = "pending"
	StopReached   StopStatus = "reached"
	StopDelivered StopStatus = "delivered"
)

type PackageStatus string

const (
	PackageInTransit PackageStatus = "in_transit"
	PackageDelivered PackageStatus = "delivered"
)

type DeliveryStop struct {
	ID        string     `json:"id"`
	Order     int        `json:"order"`
	PackageID string     `json:"package_id"`
	Latitude  float64    `json:"latitude"`
	Longitude float64    `json:"longitude"`
	TimeS     float64    `json:"time_s"`
	Status    StopStatus `json:"status"`
}

type Package struct {
	ID     string        `json:"id"`
	StopID string        `json:"stop_id"`
	Status PackageStatus `json:"status"`
}

type DeliveryEvent struct {
	Sequence  int     `json:"sequence"`
	TimeS     float64 `json:"time_s"`
	Type      string  `json:"type"`
	PackageID string  `json:"package_id,omitempty"`
	StopID    string  `json:"stop_id,omitempty"`
}

type Delivery struct {
	ID        string          `json:"id"`
	TripID    string          `json:"trip_id"`
	VehicleID string          `json:"vehicle_id"`
	Status    DeliveryStatus  `json:"status"`
	Packages  []Package       `json:"packages"`
	Stops     []DeliveryStop  `json:"stops"`
	Events    []DeliveryEvent `json:"events"`
}

type EmissionProvenance struct {
	Model       string `json:"model"`
	Class       string `json:"class"`
	Source      string `json:"source"`
	InputUnit   string `json:"input_unit"`
	OutputUnit  string `json:"output_unit"`
	Assumptions string `json:"assumptions"`
}

type Result struct {
	SUMOVersion string             `json:"sumo_version"`
	Seed        int64              `json:"seed"`
	StartedAt   time.Time          `json:"started_at"`
	FinishedAt  time.Time          `json:"finished_at"`
	Emissions   EmissionProvenance `json:"emissions"`
	Trips       []Trip             `json:"trips"`
	Tracks      []Track            `json:"tracks"`
	Deliveries  []Delivery         `json:"deliveries"`
}
