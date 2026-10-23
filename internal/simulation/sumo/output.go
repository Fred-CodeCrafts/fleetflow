package sumo

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
)

type fcdTimestep struct {
	Time     float64      `xml:"time,attr"`
	Vehicles []fcdVehicle `xml:"vehicle"`
}

type fcdVehicle struct {
	ID string  `xml:"id,attr"`
	X  float64 `xml:"x,attr"`
	Y  float64 `xml:"y,attr"`
}

type tripInfo struct {
	ID          string       `xml:"id,attr"`
	Depart      string       `xml:"depart,attr"`
	Arrival     string       `xml:"arrival,attr"`
	Duration    string       `xml:"duration,attr"`
	RouteLength string       `xml:"routeLength,attr"`
	Emissions   tripEmission `xml:"emissions"`
}

type tripEmission struct {
	CO2  string `xml:"CO2_abs,attr"`
	Fuel string `xml:"fuel_abs,attr"`
}

func parseTracks(reader io.Reader) ([]Track, error) {
	decoder := xml.NewDecoder(reader)
	tracks := make(map[string][]Position)
	order := make([]string, 0)
	lastTime := -1.0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read SUMO FCD output: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "timestep" {
			continue
		}

		var frame fcdTimestep
		if err := decoder.DecodeElement(&frame, &start); err != nil {
			return nil, fmt.Errorf("decode SUMO FCD timestep: %w", err)
		}
		if !finite(frame.Time) || frame.Time < lastTime {
			return nil, fmt.Errorf("invalid SUMO FCD timestep %v", frame.Time)
		}
		lastTime = frame.Time
		for _, vehicle := range frame.Vehicles {
			if vehicle.ID == "" || !finite(vehicle.X) || !finite(vehicle.Y) ||
				vehicle.X < -180 || vehicle.X > 180 || vehicle.Y < -90 || vehicle.Y > 90 {
				return nil, fmt.Errorf("invalid SUMO FCD vehicle position for %q", vehicle.ID)
			}
			if _, exists := tracks[vehicle.ID]; !exists {
				order = append(order, vehicle.ID)
			}
			tracks[vehicle.ID] = append(tracks[vehicle.ID], Position{
				TimeS:     frame.Time,
				Longitude: vehicle.X,
				Latitude:  vehicle.Y,
			})
		}
	}

	result := make([]Track, 0, len(order))
	for _, id := range order {
		result = append(result, Track{VehicleID: id, Positions: tracks[id]})
	}
	return result, nil
}

func parseTrips(reader io.Reader) ([]Trip, error) {
	decoder := xml.NewDecoder(reader)
	trips := make([]Trip, 0)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read SUMO tripinfo output: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "tripinfo" {
			continue
		}

		var info tripInfo
		if err := decoder.DecodeElement(&info, &start); err != nil {
			return nil, fmt.Errorf("decode SUMO tripinfo: %w", err)
		}
		depart, err := parseNumber("depart", info.Depart)
		if err != nil {
			return nil, err
		}
		arrival, err := parseNumber("arrival", info.Arrival)
		if err != nil {
			return nil, err
		}
		duration, err := parseNumber("duration", info.Duration)
		if err != nil {
			return nil, err
		}
		distance, err := parseNumber("routeLength", info.RouteLength)
		if err != nil {
			return nil, err
		}
		co2, err := parseNumber("CO2_abs", info.Emissions.CO2)
		if err != nil {
			return nil, err
		}
		fuel, err := parseNumber("fuel_abs", info.Emissions.Fuel)
		if err != nil {
			return nil, err
		}
		if info.ID == "" || depart < 0 || duration < 0 || distance < 0 || co2 < 0 || fuel < 0 {
			return nil, fmt.Errorf("invalid SUMO tripinfo for %q", info.ID)
		}

		trip := Trip{
			ID:        info.ID,
			Status:    TripRunning,
			DepartS:   depart,
			DurationS: duration,
			DistanceM: distance,
			CO2Kg:     co2 / 1_000_000,
			FuelKg:    fuel / 1_000_000,
		}
		if arrival >= 0 {
			if arrival < depart {
				return nil, fmt.Errorf("SUMO trip %q arrives before departure", info.ID)
			}
			trip.Status = TripCompleted
			trip.ArrivalS = &arrival
		} else if arrival != -1 {
			return nil, fmt.Errorf("invalid SUMO arrival time %v for trip %q", arrival, info.ID)
		}
		trips = append(trips, trip)
	}
	return trips, nil
}

func parseNumber(field, value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || !finite(number) {
		return 0, fmt.Errorf("invalid SUMO %s value %q", field, value)
	}
	return number, nil
}

func finite(number float64) bool {
	return !math.IsNaN(number) && !math.IsInf(number, 0)
}
