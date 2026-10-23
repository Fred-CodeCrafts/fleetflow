package api

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/sim"
)

// batchingFleet is a FleetService that also takes batches, so the wrappers'
// forwarding can be checked. It records what reached it.
type batchingFleet struct {
	countingFleet
	batches      int
	observations int
	batchErr     error
}

func (f *batchingFleet) ReportBatch(obs []sim.SpeedObservation, _ time.Time) error {
	f.batches++
	f.observations += len(obs)
	return f.batchErr
}

// customBatchingFleet is a batch reporter that is also a customizer, which is what
// the real HTTP client is under a demo run: it batches reports and it refreshes
// weights on the simulation clock.
type customBatchingFleet struct{ batchingFleet }

func (customBatchingFleet) CustomizeEvery() time.Duration { return 7 * time.Second }
func (customBatchingFleet) DecayStale(time.Time)          {}
func (customBatchingFleet) Customize() *engine.Metric     { return nil }

// TestObservedFleet_ForwardsBatches is the regression guard for a real slowdown.
//
// The counting wrapper is chosen by type assertion, and an early version of it
// embedded the FleetService interface without forwarding ReportBatch. That made
// the wrapped service look like it could not batch, the simulator fell back to one
// HTTP request per vehicle, and a 60,000-vehicle run went from one request a step
// to 30,000: measured at 120,455 POSTs to /api/reports against zero to the batch
// endpoint, and the run advanced 15 simulated seconds in 40 seconds of wall clock.
func TestObservedFleet_ForwardsBatches(t *testing.T) {
	var buf bytes.Buffer

	for name, inner := range map[string]sim.FleetService{
		"plain":  &batchingFleet{},
		"custom": &customBatchingFleet{},
	} {
		t.Run(name, func(t *testing.T) {
			wrapped := observedFleetOf(inner, logBuffer(&buf))

			batch, ok := wrapped.(sim.BatchReporter)
			if !ok {
				t.Fatal("the wrapper does not forward ReportBatch: the simulator would report one vehicle at a time")
			}

			obs := []sim.SpeedObservation{
				{VehicleID: "v1", EdgeID: "e_AB", SpeedMPS: 10},
				{VehicleID: "v2", EdgeID: "e_BD", SpeedMPS: 11},
			}
			if err := batch.ReportBatch(obs, time.Now()); err != nil {
				t.Fatalf("ReportBatch: %v", err)
			}

			var reached *batchingFleet
			switch v := inner.(type) {
			case *batchingFleet:
				reached = v
			case *customBatchingFleet:
				reached = &v.batchingFleet
			}
			if reached.batches != 1 || reached.observations != len(obs) {
				t.Errorf("inner saw %d batches / %d observations, want 1 / %d",
					reached.batches, reached.observations, len(obs))
			}
		})
	}
}

// TestObservedFleet_CountsABatchPerObservation keeps the finished-run line honest:
// a batched run must report the same number of reports as an unbatched one, or the
// count would silently mean "requests" instead of "observations".
func TestObservedFleet_CountsABatchPerObservation(t *testing.T) {
	var buf bytes.Buffer
	inner := &batchingFleet{}
	wrapped := observedFleetOf(inner, logBuffer(&buf))

	batch := wrapped.(sim.BatchReporter)
	obs := make([]sim.SpeedObservation, 5)
	for i := range obs {
		obs[i] = sim.SpeedObservation{VehicleID: "v", EdgeID: "e", SpeedMPS: 10}
	}
	if err := batch.ReportBatch(obs, time.Now()); err != nil {
		t.Fatalf("ReportBatch: %v", err)
	}

	// The counted wrapper is reachable through the batch wrapper; its counter is
	// what the run reports.
	bf, ok := wrapped.(*observedBatchFleet)
	if !ok {
		t.Fatalf("wrapped is %T, want *observedBatchFleet", wrapped)
	}
	if bf.inner.reports != len(obs) {
		t.Errorf("reports = %d, want %d: a batch of n counts as n observations", bf.inner.reports, len(obs))
	}
}

// TestObservedFleet_DoesNotClaimBatchingItCannotDo is the other half: a service
// that cannot batch must not be wrapped as though it can, or the simulator would
// take the batch path and count an error instead of falling back.
func TestObservedFleet_DoesNotClaimBatchingItCannotDo(t *testing.T) {
	var buf bytes.Buffer
	wrapped := observedFleetOf(countingFleet{}, logBuffer(&buf))

	if _, ok := wrapped.(sim.BatchReporter); ok {
		t.Error("a service that cannot batch was wrapped as one")
	}
}

// TestObservedFleet_CountsAFailedBatch keeps a broken batch visible: it is counted
// once per observation, because that is how many observations the weights lost.
func TestObservedFleet_CountsAFailedBatch(t *testing.T) {
	var buf bytes.Buffer
	inner := &batchingFleet{batchErr: errors.New("server refused the batch")}
	wrapped := observedFleetOf(inner, logBuffer(&buf))

	obs := []sim.SpeedObservation{{VehicleID: "v1", EdgeID: "e", SpeedMPS: 10}}
	if err := wrapped.(sim.BatchReporter).ReportBatch(obs, time.Now()); err == nil {
		t.Fatal("expected the batch error to be returned")
	}

	bf := wrapped.(*observedBatchFleet)
	if bf.inner.reportFailures != len(obs) {
		t.Errorf("reportFailures = %d, want %d", bf.inner.reportFailures, len(obs))
	}
}
