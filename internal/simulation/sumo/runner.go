package sumo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	fcdPeriodSeconds = 5
	completionGraceS = 3600
	minTripDistanceM = 250
	maxTripDistanceM = 15000
)

type Toolchain struct {
	Python      string
	RandomTrips string
	DuaRouter   string
	SUMO        string
	Network     string
	WorkDir     string
}

type Runner struct {
	tools   Toolchain
	version string
}

func NewRunner(ctx context.Context, tools Toolchain) (*Runner, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	for name, path := range map[string]string{
		"Python":       tools.Python,
		"duarouter":    tools.DuaRouter,
		"SUMO":         tools.SUMO,
		"randomTrips":  tools.RandomTrips,
		"network file": tools.Network,
	} {
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("%s path is required", name)
		}
	}

	for _, file := range []struct {
		name string
		path string
	}{
		{name: "randomTrips", path: tools.RandomTrips},
		{name: "network", path: tools.Network},
	} {
		info, err := os.Stat(file.path)
		if err != nil {
			return nil, fmt.Errorf("%s %q is unavailable: %w", file.name, file.path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s %q is not a regular file", file.name, file.path)
		}
	}

	for _, executable := range []*string{&tools.Python, &tools.DuaRouter, &tools.SUMO} {
		resolved, err := exec.LookPath(*executable)
		if err != nil {
			return nil, fmt.Errorf("required SUMO tool %q is unavailable: %w", *executable, err)
		}
		*executable = resolved
	}

	versionOutput, err := exec.CommandContext(ctx, tools.SUMO, "--version").CombinedOutput()
	if err != nil {
		return nil, commandError("read SUMO version", tools.SUMO, versionOutput, err)
	}
	version := strings.TrimSpace(string(versionOutput))
	if version == "" {
		return nil, fmt.Errorf("SUMO %q returned an empty version", tools.SUMO)
	}

	return &Runner{tools: tools, version: version}, nil
}

func (r *Runner) Run(ctx context.Context, scenario Scenario) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("SUMO runner is required")
	}
	if ctx == nil {
		return Result{}, fmt.Errorf("context is required")
	}
	if err := scenario.Validate(); err != nil {
		return Result{}, err
	}

	workDir, err := os.MkdirTemp(r.tools.WorkDir, "fleetflow-sumo-")
	if err != nil {
		return Result{}, fmt.Errorf("create SUMO run directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	demandPath := filepath.Join(workDir, "demand.trips.xml")
	routesPath := filepath.Join(workDir, "routes.rou.xml")
	tripInfoPath := filepath.Join(workDir, "tripinfo.xml")
	fcdPath := filepath.Join(workDir, "fcd.xml")
	emissionsPath := filepath.Join(workDir, "emissions.xml")

	period := float64(scenario.DepartureWindowS) / float64(scenario.Vehicles)
	args := []string{
		r.tools.RandomTrips,
		"-n", r.tools.Network,
		"-o", demandPath,
		"-e", strconv.Itoa(scenario.DepartureWindowS),
		"--period", strconv.FormatFloat(period, 'f', 6, 64),
		"--seed", strconv.FormatInt(scenario.Seed, 10),
		"--prefix", "fleetflow",
		"--min-distance", strconv.Itoa(minTripDistanceM),
		"--max-distance", strconv.Itoa(maxTripDistanceM),
		"--no-validate",
		"--vehicle-class", "passenger",
	}
	if _, err := r.runCommand(ctx, workDir, "generate seeded demand", r.tools.Python, args...); err != nil {
		return Result{}, err
	}
	if err := pinEmissionClass(demandPath); err != nil {
		return Result{}, fmt.Errorf("pin SUMO emission class: %w", err)
	}

	args = []string{
		"-n", r.tools.Network,
		"--route-files", demandPath,
		"--output-file", routesPath,
		"--seed", strconv.FormatInt(scenario.Seed, 10),
		"--no-warnings", "true",
	}
	if _, err := r.runCommand(ctx, workDir, "route seeded demand", r.tools.DuaRouter, args...); err != nil {
		return Result{}, err
	}

	endTime := scenario.DepartureWindowS + completionGraceS
	args = []string{
		"-n", r.tools.Network,
		"--route-files", routesPath,
		"--end", strconv.Itoa(endTime),
		"--tripinfo-output", tripInfoPath,
		"--tripinfo-output.write-unfinished", "true",
		"--fcd-output", fcdPath,
		"--fcd-output.geo",
		"--device.fcd.period", strconv.Itoa(fcdPeriodSeconds),
		"--emission-output", emissionsPath,
		"--no-step-log", "true",
		"--duration-log.disable", "true",
	}

	startedAt := time.Now().UTC()
	if _, err := r.runCommand(ctx, workDir, "run SUMO scenario", r.tools.SUMO, args...); err != nil {
		return Result{}, err
	}
	if info, err := os.Stat(emissionsPath); err != nil {
		return Result{}, fmt.Errorf("SUMO did not create emissions output: %w", err)
	} else if !info.Mode().IsRegular() || info.Size() == 0 {
		return Result{}, fmt.Errorf("SUMO emissions output %q is empty or not a regular file", emissionsPath)
	}

	tripsFile, err := os.Open(tripInfoPath)
	if err != nil {
		return Result{}, fmt.Errorf("open SUMO tripinfo output: %w", err)
	}
	trips, parseErr := parseTrips(tripsFile)
	closeErr := tripsFile.Close()
	if parseErr != nil {
		return Result{}, parseErr
	}
	if closeErr != nil {
		return Result{}, fmt.Errorf("close SUMO tripinfo output: %w", closeErr)
	}
	if len(trips) != scenario.Vehicles {
		return Result{}, fmt.Errorf("SUMO produced %d trip records for %d generated vehicles", len(trips), scenario.Vehicles)
	}

	fcdFile, err := os.Open(fcdPath)
	if err != nil {
		return Result{}, fmt.Errorf("open SUMO FCD output: %w", err)
	}
	tracks, parseErr := parseTracks(fcdFile)
	closeErr = fcdFile.Close()
	if parseErr != nil {
		return Result{}, parseErr
	}
	if closeErr != nil {
		return Result{}, fmt.Errorf("close SUMO FCD output: %w", closeErr)
	}
	if len(tracks) == 0 {
		return Result{}, fmt.Errorf("SUMO produced no geographic vehicle positions")
	}

	return Result{
		SUMOVersion: r.version,
		Seed:        scenario.Seed,
		StartedAt:   startedAt,
		FinishedAt:  time.Now().UTC(),
		Emissions: EmissionProvenance{
			Model:       emissionModelName,
			Class:       emissionClassName,
			Source:      "SUMO tripinfo CO2_abs and fuel_abs",
			InputUnit:   "mg",
			OutputUnit:  "kg",
			Assumptions: "All generated vehicles use the HBEFA3 Euro 4 gasoline passenger-car class; these are model estimates, not fleet-specific or measured activity.",
		},
		Trips:  trips,
		Tracks: tracks,
	}, nil
}

func (r *Runner) runCommand(ctx context.Context, dir, action, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = dir
	home := filepath.Dir(filepath.Dir(r.tools.RandomTrips))
	command.Env = append(os.Environ(), "SUMO_HOME="+home)

	output, err := command.CombinedOutput()
	if err != nil {
		return output, commandError(action, executable, output, err)
	}
	return output, nil
}

func commandError(action, executable string, output []byte, err error) error {
	details := strings.TrimSpace(string(output))
	if len(details) > 2048 {
		details = details[len(details)-2048:]
		details = "[last 2048 bytes] " + details
	}
	if details == "" {
		return fmt.Errorf("%s with %q: %w", action, executable, err)
	}
	return fmt.Errorf("%s with %q: %w: %s", action, executable, err, details)
}
