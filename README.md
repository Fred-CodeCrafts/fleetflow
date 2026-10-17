# FleetFlow

> An open-source fleet navigation platform with dynamic weights.

FleetFlow coordinates fleet vehicle routing in real time by dynamically re-weighting road networks based on live observed speeds, preventing fleets from congesting their own routes.

## Overview

Traditional navigation systems and static shortest-path routers direct vehicles along the exact same optimal corridors, often causing self-induced congestion when multiple fleet vehicles share the road network. FleetFlow solves this by:

1. **Ingesting live speed reports** from fleet vehicles.
2. **Smoothing congestion states** using exponential moving averages per edge.
3. **Decaying stale road weights** toward free-flow conditions when roads lack recent reports.
4. **Publishing immutable metric snapshots** via atomic pointer swaps so route queries (Dijkstra) never block on weight updates.
5. **Evaluating side by side** against a static baseline router using a deterministic mesoscopic simulator and an embedded live dashboard.

## Status

**MVP in active development** for the **Road To DevFest Jakarta 2026** competition.

- **Initial Commit:** 17 October 2026
- **Target Deployment:** Google Cloud Run (single instance, in-memory state)
- **License:** Apache License 2.0

## Background & Originality Notice

FleetFlow builds conceptually on academic research and an earlier thesis prototype (`sumo-routing`). However:
- **All code in this repository is written fresh from scratch starting 17 October 2026.**
- No code, files, or text were copied from earlier prototypes or third-party repositories (`sumo-routing`, `fleetbase`).
- The earlier thesis prototype serves solely as background conceptual motivation.

## Project Architecture

FleetFlow is designed as a single, self-contained Go binary:

- `cmd/fleetflow`: Server entrypoint, flag parsing, and wiring.
- `internal/graph`: Road network topology, parsing, and derived capacity/free-flow metrics.
- `internal/weights`: Per-edge state, moving-average smoothing, hold-and-decay logic, and BPR-inspired cost calculations.
- `internal/engine`: Customizer goroutine, atomic metric snapshots, and Dijkstra shortest-path router behind an interface.
- `internal/sim`: Deterministic mesoscopic simulator with background traffic, fleet vehicles, and hysteresis rerouting.
- `internal/api`: HTTP endpoints (`/healthz`, `/api/reports`, `/api/route`, `/api/sim/start`, `/api/stream`, `/api/metrics`).
- `web`: Embedded real-time dual-canvas dashboard served via `go:embed`.

## Quick Start (Local Development)

### Prerequisites

- [Go](https://golang.org/) 1.25+
- [Git](https://git-scm.com/)

### Running Locally

```bash
# Clone the repository
git clone https://github.com/Fred-CodeCrafts/fleetflow.git
cd fleetflow

# Run the service
go run ./cmd/fleetflow
```

Once running, open `http://localhost:8080` in your browser to view the operator dashboard.

## API Endpoints

- `GET /healthz` - Health check and liveness probe
- `POST /api/reports` - Ingest vehicle speed reports
- `POST /api/route` - Query route between origin and destination
- `POST /api/sim/start` - Start simulation run with specified seed & parameters
- `POST /api/sim/reset` - Stop and reset simulation
- `GET /api/stream` - Server-Sent Events (SSE) telemetry stream for the dashboard
- `GET /api/metrics` - Real-time metrics and comparative statistics

## Deployment

FleetFlow is containerized with a multi-stage Dockerfile and deployed to **Google Cloud Run** with `max-instances=1` and `min-instances=1`.

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
