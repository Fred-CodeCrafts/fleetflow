# FleetFlow PRD

Road To DevFest Jakarta 2026 / Roro Jonggrang Sprint
Author: Frederick Garner Wibowo
Status: simulation-led product direction, 23 October 2026

## 1. Product scope

FleetFlow is a Jakarta fleet-operations console backed by reproducible SUMO
traffic simulations. Operators launch a seeded scenario, inspect automatically
dispatched deliveries and vehicle movement, and review the route and package
event history. The console distinguishes simulated output from GPS telemetry;
neither is fabricated to look like the other.

SUMO is the traffic simulator and route engine for generated scenarios.
OSRM remains the service for standalone road-route estimates. FleetFlow must
not substitute a home-grown shortest-path router, invent GPS reports, or present
simulated results as measurements from a real fleet. Scenario comparisons and
AI-assisted decisions are not in the current scope.

## 2. Users

| User | Need |
|---|---|
| Fleet operator | Start a repeatable scenario, monitor generated deliveries, and inspect completed trip history |
| Fleet manager | Review vehicle specifications, maintenance, driver qualifications, utilization, and estimated impact |
| Vehicle integration | Submit authenticated real GPS positions, kept separate from simulator output |
| Developer | Run FleetFlow and SUMO locally with documented network provenance and setup |

## 3. Functional requirements

### Scenarios and simulation

- Provide named scenario presets with defined demand, simulation duration, and
  reproducible random seeds.
- Run scenarios using the SUMO executable, a documented Jakarta road network,
  and SUMO-generated routes/vehicle movement. Record the SUMO version,
  network identity, preset, seed, and run timestamps.
- Show run lifecycle and actionable startup/runtime errors. An unavailable
  SUMO executable or invalid network is an error, never a successful mock run.
- A run produces inspectable generated trips, vehicle locations, completed
  trips, route geometry, and ordered delivery events.
- Label all run-derived movement and metrics as simulation output, distinct
  from real GPS.
- Do not add baseline-versus-dynamic experiment comparisons or AI controls.

### Deliveries, dispatch, and trip history

- Create delivery demand from the selected scenario; operators view generated
  trips and cannot manually create or assign a trip in the dispatch workflow.
- Automatically assign an available compatible vehicle and driver using
  deterministic, documented eligibility rules. Respect capacity, vehicle
  availability, driver shift, and license class. Explain unassigned demand.
- Support one or more delivery stops per trip and package-level status events,
  including creation, assignment, arrival, delivery, and completion.
- Provide a trip detail view with an interactive map, highlighted SUMO route,
  origin, ordered destinations, current/completed state, and a chronological
  event timeline. Retain completed trip history for the run.

### Fleet and driver records

- Vehicle profiles include registration, make, model, year, color, body/fuel
  type, capacity, status, and maintenance history.
- Driver profiles include name, contact phone, license class and expiry, shift,
  and availability. Do not collect full KTP/NIK, gender, or age absent a
  necessary and approved product requirement.
- Vehicle and driver detail pages show their own profile and associated
  scenario/trip history; assignments remain automatic in the operating flow.
- Seed/demo identities must be clearly fictitious and must not resemble real
  personal data.

### Emissions and operational insights

- Report SUMO-generated fuel and emissions as estimates from the selected
  vehicle/emission model; retain the model, units, SUMO version, and scenario
  assumptions with the result.
- Do not label a simulation estimate as measured fuel, actual fleet emissions,
  or verified carbon savings. Missing SUMO output is shown as unavailable.
- Provide useful operational summaries such as demand served, unassigned
  deliveries, completed trips, distance, duration, vehicle utilization, and
  fuel/emissions estimates, each with clear simulated or measured provenance.

### Map, routing, and real GPS

- Render an interactive Jakarta map with MapLibre GL and visible basemap
  attribution.
- SUMO trip routes and vehicle positions come from SUMO outputs. Real GPS mode
  displays only recent authenticated position reports received by the server.
- Keep simulation playback and real GPS visually separate and explicitly
  labeled; never silently substitute one for another.
- OSRM route requests return route geometry, distance, and estimated duration.
  Clearly surface unavailable/no-route failures; do not use a home-grown
  fallback.
- Use a Jakarta road network whose source, license, geographic coverage, and
  build procedure are documented. Do not claim metropolitan or DKI-wide
  coverage until the network extent is verified against that boundary.

## 4. Data and truthful display

- Map tiles use OpenFreeMap and map rendering uses MapLibre GL. Preserve
  OpenStreetMap attribution and ODbL obligations for derived map/network data.
- SUMO outputs are simulated observations, not real vehicle telemetry.
- OSRM distance/duration are route estimates, not live-traffic predictions.
- GPS positions are actual reports and expire according to the configured
  freshness window; no route interpolation may be presented as GPS history.
- Emissions are model estimates unless backed by measured fuel/activity data.
  Record the model/factor and units; do not claim verified reductions.
- PostgreSQL-backed data survives restarts. Memory-backed development data is
  temporary and must be disclosed.

## 5. Explicit non-goals

- AI, autonomous operational decisions, or opaque assignment/routing policies.
- Baseline-versus-dynamic routing comparisons, experiment scoreboards, or
  thesis-result reporting.
- Manual trip creation or manual vehicle/driver dispatch in the scenario
  workflow.
- Fabricated live GPS positions, synthetic points presented as real telemetry,
  static demo map markers, or unsupported emissions/savings claims.
- FleetFlow-owned shortest-path routing.

## 6. Operational requirements

- `GET /healthz` reports process liveness; `GET /readyz` checks configured
  PostgreSQL and required runtime dependencies.
- SUMO executable, network path, and OSRM URL are explicit configuration, with
  useful validation and errors. No external-service failure becomes mock data.
- Persist scenario/run metadata, generated trips, package events, and completed
  history when PostgreSQL is configured. Clearly disclose memory-only behavior.
- Keep the console usable at desktop and narrow viewport sizes, with
  keyboard-accessible controls, visible validation, and readable map
  attribution.
- The production container must include or explicitly connect to a supported
  SUMO runtime; local-only availability does not satisfy deployment readiness.

## 7. Acceptance checks

1. Run a deterministic scenario twice with the same network, preset, SUMO
   version, and seed; verify its recorded inputs and generated demand.
2. Confirm scenario execution invokes SUMO and fails clearly if its executable
   or network is unavailable.
3. Start the console and inspect generated trips, automatic eligible
   assignments, vehicle positions, and completed route/event history.
4. Verify trip details show the route on the map and ordered package events;
   operators cannot create or manually assign scenario trips.
5. Inspect vehicle and driver profiles, including maintenance, capacity,
   license, shift, and availability.
6. Verify fuel/emissions are traceable SUMO estimates, clearly distinguished
   from real measured data and absent when output is unavailable.
7. Confirm real GPS mode only shows authenticated fresh reports and remains
   distinct from SUMO simulation.
8. Confirm OSRM failures, invalid scenarios, database errors, and missing
   simulation inputs are visible and never return success-shaped fallbacks.
