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
- `internal/api`: HTTP endpoints (`/healthz`, `/api/reports`, `/api/route`, `/api/sim/start`, `/api/stream`, `/api/metrics`, `/api/runs`).
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
- `GET /api/vehicles`, `POST /api/vehicles` - Fleet vehicle records
- `GET /api/drivers`, `POST /api/drivers` - Driver records
- `GET /api/trips`, `POST /api/trips` - Trip records, with `POST /api/trips/{id}/status` to advance one
- `GET /api/runs`, `POST /api/runs` - Saved comparison runs

### API keys

The two ingest endpoints require a key, because they are the ones an outside system
calls. Issue one with the service's own command; it stores the key, prints it once,
and exits without serving:

```sh
go run ./cmd/fleetflow -issue-key "dispatch integration"
# flf_9Qx...   <- the only time this is shown
```

Only the SHA-256 digest is stored, so the key cannot be read back afterwards, and it
is never written to a log. If you lose it, issue another one: there is nothing to
recover, and nothing to look up. Present it as a bearer token:

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
record endpoints (`/api/vehicles`, `/api/drivers`, `/api/trips`, `/api/runs`) and
the simulation controls stay open, because they belong to the dashboard.

#### The report format

A report says how fast one vehicle is going on one edge. Every field is checked
before anything is written, and every refusal names what it refused:

| field | type | required | accepted |
|---|---|---|---|
| `vehicle_id` | string | yes | non-empty; it is also the vehicle the dashboard shows as live |
| `edge_id` | string | yes | an edge from `GET /api/graph` |
| `speed_mps` | number | yes | greater than 0 and at most 90 (324 km/h, inclusive) |
| `ts` | integer | no | Unix milliseconds, positive, at most 5 minutes ahead of this server's clock. Omit it to mean "now" |

A field the payload does not define is refused rather than ignored, so a
misspelled key is an error instead of a report that looks accepted and changes
nothing — and the same goes for a second JSON object after the first.

| status | code | meaning |
|---|---|---|
| `401` | `unauthenticated` | missing, malformed or unknown key; nothing is read and nothing is written |
| `400` | `vehicle_id_required`, `edge_id_required` | a required field was empty |
| `400` | `speed_out_of_range` | zero, negative, not a number, or above 90 m/s |
| `400` | `ts_invalid`, `ts_in_future` | not a positive Unix-millisecond time, or dated in the future |
| `400` | `empty_body`, `malformed_json`, `unknown_field`, `bad_field_type`, `trailing_data` | the body is not one JSON object of the shape above |
| `404` | `edge_not_found` | `edge_id` is not in the graph |
| `413` | `body_too_large` | over 64 KiB |
| `503` | `engine_unavailable` | this process is running without an engine |

The future bound is not pedantry. A report's timestamp is kept on the edge, and
decay only resumes once the hold time has passed since it, so a single report
dated next year would hold that road at its last congestion until next year.

### Simulation runs

`POST /api/sim/start` drives one seeded demand through both engines at once — the
static baseline and the dynamic one — and answers `202` with the run that is now
going; `POST /api/sim/reset` stops it and clears it.

```sh
# start the demo scenario: a 30 second morning peak of 2,000 vehicles, half of
# them on FleetFlow
curl -sS -X POST http://localhost:8080/api/sim/start \
  -H "Content-Type: application/json" \
  -d '{"seed":1}'
# {"seed":1,"vehicles":2000,"fleet_share":0.5,"step_s":1,"window_s":30,"running":true}

curl -sS -X POST http://localhost:8080/api/sim/reset
```

Every field is optional and defaults to the demo configuration
(`seed`, `vehicles`, `fleet_share`, `step_s`, `window_s`, `reroute_every_s`,
`reroute_threshold`); a value out of range comes back as `400` with a named code
and changes nothing. Starting a run replaces the one that was going.

The departure window is the field worth knowing about: 2,000 vehicles spread over
ten minutes barely meet each other, and a network nobody fills is one where static
and dynamic routing make identical choices. The 30 s default is a peak, and it is
what gives the comparison something to measure. `docs/results.md` has the numbers,
including the seeds where FleetFlow does not win.

A run's fleet vehicles reach the routing engine over the public API, so simulated
traffic is validated, authenticated and measured like an external client. That
needs a credential the process can present as its own, and it mints one at startup:
a random key, kept hashed in memory, never written to the database, the environment
or a log. There is nothing to issue and nothing to configure.

```sh
go run ./cmd/fleetflow -demo
```

`-demo` is what serves `/api/sim/start` and `/api/sim/reset`; without it those routes
are 404. A run takes the engine's weights — it clears them at start and then feeds
them — so the flag is an assertion that nothing live is reporting to this process.
The deployed demo passes it; a real deployment does not.

### Saving a run

`POST /api/runs` keeps the run that is going, and `GET /api/runs` lists what has
been kept. The endpoint takes no body: the server captures the seed, the demand
and the metrics from the run itself, so what is stored is what ran. A save with
no run going is `409` with `{"code":"no_run"}`, and a save that carries a body is
refused rather than silently ignored.

```sh
curl -sS -X POST http://localhost:8080/api/runs
# {"id":"run_d9a4...","seed":1,"config":{...},"metrics":{...},"created_at":"..."}

curl -sS "http://localhost:8080/api/runs?limit=10"
# {"runs":[...]}
```

Saved runs live in Postgres when a database is configured, so they are still
there after a restart; without one they are in memory and lost with the process.

### Watching a run

`GET /api/stream` is one Server-Sent Events connection carrying three kinds of
event, each at the cadence its contents change at: `vehicles` five times a second,
`edges` and `metrics` once a second. The body of `metrics` is exactly what
`GET /api/metrics` answers, so the same shape is parsed whether you ask once or
watch.

```sh
# watch a run: -N keeps curl from buffering the stream
curl -sS -N http://localhost:8080/api/stream
# event: metrics
# data: {"engine":{"metric_version":3,...},"run":{...}}
# event: edges
# data: {"metric_version":3,"edges":[{"edge_id":"e_AB","cost_s":10.2,"free_flow_s":10}]}
# event: vehicles
# data: {"now_s":42,"running":true,"vehicles":[{"id":"v0007","kind":"simulated",...}]}
```

Every vehicle is labelled with where its position came from. A simulated vehicle
names the panel it belongs to (`arm`: `baseline` or `fleetflow`); a vehicle that has
reported through `POST /api/reports` in the last minute appears as `live`, drawn on
the edge it reported on. The report carries a speed rather than a coordinate, so
that is the edge rather than a point — drawing a position the API never received
would be worse than drawing the edge it did.

A run advances only while somebody is watching the stream, so `start` followed by
`stream` is the whole sequence; a run nobody is watching waits where it is rather
than spending CPU on a demo nobody can see. That is what lets the deployed service
keep Cloud Run's default CPU allocation, which is throttled outside a request.

## Deployment

FleetFlow is containerized with a multi-stage Dockerfile and deployed to **Google Cloud Run** with `max-instances=1` and `min-instances=1`.

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
