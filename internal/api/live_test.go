package api

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestLiveVehicles_RemembersWhereAVehicleLastReported(t *testing.T) {
	live := newLiveVehicles(time.Minute, 10)
	now := time.Now()

	live.observe("truck-1", "e_AB", 12.5, now)
	live.observe("truck-2", "e_CD", 8.0, now)

	seen := live.snapshot(time.Now())
	if len(seen) != 2 {
		t.Fatalf("snapshot() returned %d vehicles, want 2", len(seen))
	}
	// Ordered by ID, so two frames built from the same state match.
	if seen[0].ID != "truck-1" || seen[0].EdgeID != "e_AB" || seen[0].SpeedMPS != 12.5 {
		t.Errorf("first vehicle = %+v, want truck-1 on e_AB at 12.5", seen[0])
	}
	if seen[1].ID != "truck-2" {
		t.Errorf("second vehicle = %+v, want truck-2", seen[1])
	}
}

func TestLiveVehicles_FollowsAVehicleToItsNewEdge(t *testing.T) {
	live := newLiveVehicles(time.Minute, 10)
	now := time.Now()

	live.observe("truck-1", "e_AB", 12.5, now)
	live.observe("truck-1", "e_BD", 9.0, now)

	seen := live.snapshot(time.Now())
	if len(seen) != 1 {
		t.Fatalf("snapshot() returned %d vehicles, want 1: a vehicle is one entry, not one per report", len(seen))
	}
	if seen[0].EdgeID != "e_BD" {
		t.Errorf("edge = %s, want the edge of the latest report", seen[0].EdgeID)
	}
}

// A replayed report belongs in the weights, but a vehicle that was on the road
// minutes ago is not on it now, and drawing it as live would be a lie.
func TestLiveVehicles_IgnoresAReplayOfOldHistory(t *testing.T) {
	live := newLiveVehicles(time.Minute, 10)

	live.observe("truck-1", "e_AB", 12.5, time.Now().Add(-time.Hour))

	if seen := live.snapshot(time.Now()); len(seen) != 0 {
		t.Errorf("snapshot() returned %+v, want nothing for a report an hour old", seen)
	}
}

func TestLiveVehicles_ForgetsAVehicleThatWentQuiet(t *testing.T) {
	live := newLiveVehicles(50*time.Millisecond, 10)

	live.observe("truck-1", "e_AB", 12.5, time.Now())
	time.Sleep(80 * time.Millisecond)

	if seen := live.snapshot(time.Now()); len(seen) != 0 {
		t.Errorf("snapshot() returned %+v after the ttl, want nothing", seen)
	}
}

func TestLiveVehicles_DropsTheQuietestWhenFull(t *testing.T) {
	live := newLiveVehicles(time.Minute, 2)

	live.observe("first", "e_AB", 1, time.Now())
	time.Sleep(2 * time.Millisecond)
	live.observe("second", "e_AB", 1, time.Now())
	time.Sleep(2 * time.Millisecond)
	live.observe("third", "e_AB", 1, time.Now())

	seen := live.snapshot(time.Now())
	if len(seen) != 2 {
		t.Fatalf("snapshot() returned %d vehicles, want the capacity of 2", len(seen))
	}
	if seen[0].ID != "second" || seen[1].ID != "third" {
		t.Errorf("kept %s and %s, want the two most recent", seen[0].ID, seen[1].ID)
	}
}

// TestLiveVehicles_FollowsReportsOnTheWire is the hook that matters: a vehicle
// that reports through the public API becomes visible as live traffic.
func TestLiveVehicles_FollowsReportsOnTheWire(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	if rec := postJSON(t, s, "/api/reports",
		`{"vehicle_id":"truck-9","edge_id":"e_AB","speed_mps":8.5}`); rec.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want 202: %s", rec.Code, rec.Body)
	}

	seen := s.live.snapshot(time.Now())
	if len(seen) != 1 {
		t.Fatalf("snapshot() returned %d vehicles after one report, want 1", len(seen))
	}
	if seen[0].ID != "truck-9" || seen[0].EdgeID != "e_AB" || seen[0].SpeedMPS != 8.5 {
		t.Errorf("vehicle = %+v, want truck-9 on e_AB at 8.5", seen[0])
	}
}

// A report the API refuses must not put a vehicle on the live view: nothing else
// about a refused request is allowed to happen either.
func TestLiveVehicles_IgnoresARejectedReport(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	if rec := postJSON(t, s, "/api/reports",
		`{"vehicle_id":"truck-9","edge_id":"nowhere","speed_mps":8.5}`); rec.Code == http.StatusAccepted {
		t.Fatalf("report for an unknown edge was accepted: %s", rec.Body)
	}

	if seen := s.live.snapshot(time.Now()); len(seen) != 0 {
		t.Errorf("snapshot() returned %+v after a refused report, want nothing", seen)
	}
}

func TestLiveVehicles_SurvivesConcurrentReportAndRead(t *testing.T) {
	live := newLiveVehicles(time.Minute, 32)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				live.observe("truck-"+string(rune('a'+n)), "e_AB", 10, time.Now())
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			live.snapshot(time.Now())
		}
	}()
	wg.Wait()

	if seen := live.snapshot(time.Now()); len(seen) != 8 {
		t.Errorf("snapshot() returned %d vehicles, want 8", len(seen))
	}
}
