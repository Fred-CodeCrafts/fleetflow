package simulation

import (
	"fmt"
	"hash/fnv"
	"sort"

	"fleetflow/internal/simulation/sumo"
)

func generateDeliveries(result *sumo.Result) error {
	if result == nil {
		return fmt.Errorf("SUMO result is required to generate deliveries")
	}
	tracks := make(map[string]sumo.Track, len(result.Tracks))
	for _, track := range result.Tracks {
		tracks[track.VehicleID] = track
	}

	deliveries := make([]sumo.Delivery, 0, len(result.Trips))
	for _, trip := range result.Trips {
		if trip.Status == sumo.TripCompleted && trip.ArrivalS == nil {
			return fmt.Errorf("completed SUMO trip %q has no arrival time", trip.ID)
		}
		track, ok := tracks[trip.ID]
		if !ok || len(track.Positions) < 2 {
			return fmt.Errorf("SUMO trip %q has no complete position trace for delivery stops", trip.ID)
		}
		if trip.Status == sumo.TripCompleted && trip.ArrivalS != nil {
			end := len(track.Positions)
			for end > 0 && track.Positions[end-1].TimeS > *trip.ArrivalS {
				end--
			}
			if end < 2 {
				return fmt.Errorf("completed SUMO trip %q has too few positions before arrival", trip.ID)
			}
			track.Positions = track.Positions[:end]
		}
		delivery := generateDelivery(result.Seed, trip, track)
		deliveries = append(deliveries, delivery)
	}
	result.Deliveries = deliveries
	return nil
}

func generateDelivery(seed int64, trip sumo.Trip, track sumo.Track) sumo.Delivery {
	deliveryID := fmt.Sprintf("delivery_%d_%s", seed, trip.ID)
	stopCount := 1 + int(deliveryHash(seed, trip.ID)%3)
	maxStops := len(track.Positions) - 2
	if maxStops < 1 {
		maxStops = 1
	}
	if stopCount > maxStops {
		stopCount = maxStops
	}

	delivery := sumo.Delivery{
		ID:        deliveryID,
		TripID:    trip.ID,
		VehicleID: track.VehicleID,
		Status:    sumo.DeliveryInProgress,
		Packages:  make([]sumo.Package, 0, stopCount),
		Stops:     make([]sumo.DeliveryStop, 0, stopCount),
		Events: []sumo.DeliveryEvent{{
			TimeS: trip.DepartS,
			Type:  "delivery_created",
		}},
	}
	if trip.Status == sumo.TripCompleted {
		delivery.Status = sumo.DeliveryCompleted
	}

	for index := 1; index <= stopCount; index++ {
		positionIndex := index * (len(track.Positions) - 1) / (stopCount + 1)
		if positionIndex < 1 {
			positionIndex = 1
		}
		position := track.Positions[positionIndex]
		stopID := fmt.Sprintf("%s_stop_%d", deliveryID, index)
		packageID := fmt.Sprintf("%s_package_%d", deliveryID, index)
		stopStatus := sumo.StopReached
		packageStatus := sumo.PackageInTransit
		if delivery.Status == sumo.DeliveryCompleted {
			stopStatus = sumo.StopDelivered
			packageStatus = sumo.PackageDelivered
		}
		delivery.Stops = append(delivery.Stops, sumo.DeliveryStop{
			ID: stopID, Order: index, PackageID: packageID,
			Latitude: position.Latitude, Longitude: position.Longitude,
			TimeS: position.TimeS, Status: stopStatus,
		})
		delivery.Packages = append(delivery.Packages, sumo.Package{
			ID: packageID, StopID: stopID, Status: packageStatus,
		})
		delivery.Events = append(delivery.Events, sumo.DeliveryEvent{
			TimeS: position.TimeS, Type: "stop_reached", PackageID: packageID, StopID: stopID,
		})
		if packageStatus == sumo.PackageDelivered {
			delivery.Events = append(delivery.Events, sumo.DeliveryEvent{
				TimeS: position.TimeS, Type: "package_delivered", PackageID: packageID, StopID: stopID,
			})
		}
	}
	if trip.Status == sumo.TripCompleted && trip.ArrivalS != nil {
		delivery.Events = append(delivery.Events, sumo.DeliveryEvent{
			TimeS: *trip.ArrivalS,
			Type:  "delivery_completed",
		})
	}
	sort.SliceStable(delivery.Events, func(i, j int) bool {
		return delivery.Events[i].TimeS < delivery.Events[j].TimeS
	})
	for index := range delivery.Events {
		delivery.Events[index].Sequence = index + 1
	}
	return delivery
}

func deliveryHash(seed int64, tripID string) uint64 {
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "%d:%s", seed, tripID)
	return hash.Sum64()
}
