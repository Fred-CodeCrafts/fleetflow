# FleetFlow delivery checklist

[PRD.md](./PRD.md) defines the simulation-led product direction;
[DESIGN.md](./DESIGN.md) records the intended architecture and its current
implementation caveats.

## Existing foundation

- [x] React/TypeScript operator console and Go API.
- [x] MapLibre map, OpenFreeMap tiles, and visible map attribution.
- [x] OSRM route client/API and Jakarta-region OSM data preparation workflow.
- [x] Authenticated GPS ingestion with a separate live-GPS view.
- [x] Remove the legacy static map preview; show SUMO scenario playback instead.
- [x] PostgreSQL persistence for existing fleet, driver, and trip records.

## SUMO simulation and delivery operations

- [x] Verify local SUMO 1.27.1 can generate seeded demand with `randomTrips.py`,
  route it with `duarouter`, and produce tripinfo, geographic FCD, and emission
  XML using the retained Jakarta network. This smoke test only establishes the
  local toolchain; it does not validate network coverage or production setup.
- [x] Obtain and document current OSM source data, provenance, licensing, and
  a buffered network extraction based on the DKI administrative boundary. Keep
  source and generated network data out of Git.
- [x] Validate network connectivity and representative SUMO routes on the Jakarta
  network with duarouter and representative trip sampling.
- [x] Add validated SUMO runtime/network configuration and explicit process
  failure reporting.
- [x] Add seeded scenarios and a reproducible SUMO run lifecycle; capture
  SUMO version/seed and persist run state.
- [x] Persist scenario results, generated trips, and SUMO position traces.
- [x] Persist ordered generated delivery stops and package events in each
  simulation run's existing JSONB result and cover the memory repository.
- [ ] Generate scenario delivery demand and automatically assign compatible
  available vehicles and drivers; explain unassigned deliveries.
- [x] Add scenario/run monitoring and read-only generated trip detail screens;
  remove manual trip creation and driver assignment controls.
- [x] Show generated stop markers, ordered delivery stops, and a chronological
  package-event timeline on trip details.
- [ ] Add time-controlled simulation route playback with a moving vehicle
  marker.
- [x] Capture optional vehicle specifications and maintenance notes, and driver
  contact, license, shift, and availability without collecting sensitive
  identity fields; keep missing values explicitly unknown.
- [x] Add individual vehicle and driver profile pages.
- [ ] Link automatically dispatched scenario trips to fleet records and show
  those records' associated trip history.
- [ ] Record SUMO fuel/emissions model, units, and assumptions; separate
  simulated estimates from measured activity.
- [x] Package SUMO execution for durable Cloud Run deployment; test
  recovery and multi-instance concurrency locks across API restarts.

## Validation and rollout

- [ ] Add focused Go tests for scenario validation, deterministic inputs,
  assignment eligibility, run failure handling, output parsing, and event order.
- [ ] Add focused API and console tests for read-only trip flow, generated
  history, clear source labels, and unavailable-data states.
- [ ] Run Go formatting, vet, build, and tests, preserving any user-owned local
  changes; run the console tests, typecheck, and production build.
- [ ] Exercise a full scenario end to end on the verified DKI-wide network.
- [ ] Verify real GPS and SUMO map layers cannot be confused.
- [x] Validate deployed SUMO runtime, database persistence, OSRM, attribution,
  and Cloud Run production deployment configuration.
