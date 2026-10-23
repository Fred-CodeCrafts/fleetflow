package sumo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewRunnerRequiresToolPaths(t *testing.T) {
	_, err := NewRunner(context.Background(), Toolchain{})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("NewRunner() error = %v, want a missing tool path error", err)
	}
}

func TestNewRunnerRejectsMissingNetwork(t *testing.T) {
	randomTrips := filepath.Join(t.TempDir(), "randomTrips.py")
	if err := os.WriteFile(randomTrips, []byte("# test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewRunner(context.Background(), Toolchain{
		Python:      "python",
		RandomTrips: randomTrips,
		DuaRouter:   "duarouter",
		SUMO:        "sumo",
		Network:     filepath.Join(t.TempDir(), "missing.net.xml"),
	})
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("NewRunner() error = %v, want a missing network error", err)
	}
}

func TestRunRejectsInvalidScenarioBeforeStartingTools(t *testing.T) {
	runner := &Runner{}
	_, err := runner.Run(context.Background(), Scenario{})
	if !errors.Is(err, errScenarioInvalid) {
		t.Fatalf("Run() error = %v, want invalid scenario", err)
	}
}

func TestRunnerExecutesSUMOWhenConfigured(t *testing.T) {
	network := os.Getenv("FLEETFLOW_SUMO_NETWORK")
	if network == "" {
		t.Skip("set FLEETFLOW_SUMO_NETWORK and SUMO_HOME to run the local SUMO integration test")
	}
	home := os.Getenv("SUMO_HOME")
	if home == "" {
		t.Fatal("SUMO_HOME is required when FLEETFLOW_SUMO_NETWORK is set")
	}

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}
	runner, err := NewRunner(context.Background(), Toolchain{
		Python:      "python",
		RandomTrips: filepath.Join(home, "tools", "randomTrips.py"),
		DuaRouter:   filepath.Join(home, "bin", "duarouter"+extension),
		SUMO:        filepath.Join(home, "bin", "sumo"+extension),
		Network:     network,
		WorkDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx, Scenario{Seed: 42, Vehicles: 3, DepartureWindowS: 120})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trips) != 3 || len(result.Tracks) == 0 || result.SUMOVersion == "" {
		t.Fatalf("incomplete SUMO result: trips=%d tracks=%d version=%q", len(result.Trips), len(result.Tracks), result.SUMOVersion)
	}
}
