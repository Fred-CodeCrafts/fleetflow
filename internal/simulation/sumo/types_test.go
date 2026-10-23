package sumo

import (
	"errors"
	"testing"
)

func TestScenarioValidate(t *testing.T) {
	tests := []struct {
		name     string
		scenario Scenario
		wantErr  bool
	}{
		{name: "valid", scenario: Scenario{Seed: 42, Vehicles: 20, DepartureWindowS: 600}},
		{name: "negative seed", scenario: Scenario{Seed: -1, Vehicles: 1, DepartureWindowS: 60}, wantErr: true},
		{name: "seed too large", scenario: Scenario{Seed: maxSeed + 1, Vehicles: 1, DepartureWindowS: 60}, wantErr: true},
		{name: "no vehicles", scenario: Scenario{Seed: 1, DepartureWindowS: 60}, wantErr: true},
		{name: "too many vehicles", scenario: Scenario{Seed: 1, Vehicles: maxVehicles + 1, DepartureWindowS: 60}, wantErr: true},
		{name: "short window", scenario: Scenario{Seed: 1, Vehicles: 1, DepartureWindowS: 59}, wantErr: true},
		{name: "long window", scenario: Scenario{Seed: 1, Vehicles: 1, DepartureWindowS: maxDepartureWin + 1}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.scenario.Validate()
			if test.wantErr && !errors.Is(err, errScenarioInvalid) {
				t.Fatalf("Validate() error = %v, want invalid scenario", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
