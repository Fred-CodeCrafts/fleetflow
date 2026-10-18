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
- `POST /api/reports` - Ingest vehicle speed reports (requires an API key)
- `POST /api/route` - Query route between origin and destination (requires an API key)
- `POST /api/sim/start` - Start simulation run with specified seed & parameters
- `POST /api/sim/reset` - Stop and reset simulation
- `GET /api/stream` - Server-Sent Events (SSE) telemetry stream for the dashboard
- `GET /api/metrics` - Real-time metrics and comparative statistics

### API keys

The two ingest endpoints require a key, because they are the ones an outside system
calls. Issue one with the service's own command; it stores the key, prints it once,
and exits without serving:

```sh
go run ./cmd/fleetflow -issue-key "dispatch integration"
# flf_9Qx...   <- the only time this is shown
```

Only the SHA-256 digest is stored, so the key cannot be read back afterwards, and it
is never written to a log. Present it as a bearer token:

```sh
# submit a vehicle report
curl -sS -X POST http://localhost:8080/api/reports \
  -H "Authorization: Bearer $FLEETFLOW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":13.9}'

# ask for a route
curl -sS -X POST http://localhost:8080/api/route \
  -H "Authorization: Bearer $FLEETFLOW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"from_node":"A","to_node":"D"}'

# ask for a route and price the path already being driven, so the two costs
# come from one snapshot and can be compared
curl -sS -X POST http://localhost:8080/api/route \
  -H "Authorization: Bearer $FLEETFLOW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"from_node":"A","to_node":"D","current_edges":["e_AB","e_BD"]}'
# {"edges":["e_AC","e_CD"],"cost_s":46.1,"metric_version":3,"current_cost_s":72.0}
```

`current_edges` must be a chain that starts at `from_node`; anything else returns
`400` and computes nothing.

A missing or wrong key returns `401` with `{"code":"unauthenticated"}` and does
nothing else: no report reaches the weights, and no route is computed. The fleet
record endpoints (`/api/vehicles`, `/api/drivers`, `/api/trips`) stay open, because
they belong to the dashboard.

## Deployment

FleetFlow is containerized with a multi-stage Dockerfile and deployed to **Google Cloud Run** with `max-instances=1` and `min-instances=1`.

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
