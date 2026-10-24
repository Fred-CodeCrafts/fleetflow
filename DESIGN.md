# FleetFlow Design

FleetFlow is a simulation-led fleet-operations console. The product contract is
defined by [PRD.md](./PRD.md). SUMO executes seeded traffic scenarios; OSRM
provides separate driving-route estimates; real GPS remains its own data source.
This document describes the intended boundaries, not a claim that every planned
component is already implemented. Progress is tracked in [TASKS.md](./TASKS.md).

## System shape

```mermaid
flowchart LR
    Operator["Fleet operator"] --> Console["React + TypeScript console"]
    Console --> API["FleetFlow Go API"]
    Console --> Map["MapLibre GL + OpenFreeMap"]
    API --> Runs["Scenario/run coordinator"]
    Runs --> Demand["Seeded demand + SUMO route tools"]
    Demand --> SUMO["SUMO runtime"]
    Runs --> Events["Run, trip, position, and delivery events"]
    Events --> DB["PostgreSQL (optional in local development)"]
    API --> OSRM["OSRM route service"]
    Vehicle["GPS integration"] -->|Authenticated real reports| API
```

## Runtime components

### Console

- React and TypeScript application under `console/`; MapLibre GL renders
  OpenFreeMap tiles with visible attribution.
- Scenario controls select a named demand profile and seed. A run screen shows
  SUMO state and simulated positions; trip details show the route and ordered
  delivery history.
- Fleet and driver detail pages display operational attributes and history.
  Scenario dispatch has no manual trip-creation or assignment action.
- Map layers and labels keep SUMO movement distinct from reported GPS. There is
  no static demo-marker mode.

### Go API and scenario coordinator

- `cmd/fleetflow` configures the API, OSRM client, persistence, and simulation
  runtime.
- `internal/api` validates requests and exposes fleet records, scenarios, runs,
  generated trips, delivery events, routing, and real GPS endpoints.
- A scenario coordinator owns run lifecycle, deterministic demand inputs,
  automatic assignment, SUMO process execution, output collection, and errors.
  Long-running work must not be reported as complete before SUMO exits and
  outputs are validated.
- Run metadata records the seed, preset, network identity, SUMO version, and
  timestamps. PostgreSQL migrations persist runs, generated trips, stops,
  assignments, and ordered events; memory storage is for explicitly temporary
  local development only.
- Dispatch is deterministic and explainable: reject unavailable or
  incompatible vehicle/driver pairs, honor capacity and license/shift rules,
  and surface demand that cannot be assigned.

### SUMO and network data

- Generate repeatable trip demand with SUMO's supported demand and route tools,
  including a recorded seed; execute the resulting scenario with the SUMO
  binary, not the legacy graph simulator.
- Collect SUMO floating-car output for simulated map positions, trip information
  for completed movement, and emission output for modeled fuel/emissions.
  Convert output into API events while preserving simulated time and units.
- `SUMO_HOME`, the SUMO executable, and network path are validated explicitly.
  Missing tools, incompatible inputs, non-zero exits, and malformed output are
  visible errors; no mock run replaces them.
- `data/networks/jakarta.net.xml` is currently documented as a legacy
  central-Jakarta artifact, not a verified DKI-wide network. Do not enable it as
  the production scenario network until source, extent, licensing, and a
  representative route are verified. Keep downloaded source data and generated
  runtime outputs out of Git.
- Preserve OpenStreetMap attribution and ODbL obligations for derived network
  data. Document the exact geographic extent rather than claiming full Jakarta
  or metropolitan coverage without validation.

### Routing, map, and emissions

- SUMO routes simulated demand using its road network and route tools. OSRM is
  independently used by the existing route-estimate endpoint; no
  FleetFlow-owned shortest-path fallback is allowed.
- GPS endpoints accept authenticated reports. They never read SUMO output, and
  simulation endpoints never reinterpret GPS samples as simulation events.
- SUMO fuel and emissions are estimates under the configured vehicle/emission
  model. Persist model/version, units, and run assumptions; never label them as
  measured activity or verified carbon savings.
- Basemap rendering depends on OpenFreeMap; routing and simulation runtime
  failures are surfaced rather than converted to fallback data.

## Deployment and failure behavior

- `GET /healthz` reports process liveness. `GET /readyz` checks configured
  PostgreSQL, OSRM, and required simulation runtime according to deployment
  configuration.
- The production runtime must contain a supported SUMO installation or delegate
  execution to a durable worker. A local developer's `sumo.exe` is not a
  production dependency strategy.
- Run execution must be recoverable/observable across API restarts in
  production. In-process goroutines alone are not a durable run queue.
- API/database/SUMO/OSRM errors remain explicit. No service returns mock
  positions, trips, routes, or emissions on failure.
