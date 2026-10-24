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
- [ ] Validate network connectivity and representative SUMO routes on the DKI
  network. The optional full-network SUMO load test is memory-intensive and has
  not passed on the current 16 GB development machine.
- [x] Add validated SUMO runtime/network configuration and explicit process
  failure reporting.
- [x] Add seeded scenarios and a reproducible SUMO run lifecycle; capture
  SUMO version/seed and persist run state.
- [x] Persist scenario results, generated trips, and SUMO position traces.
- [ ] Persist ordered delivery stops, package events, and event history with a
  database migration and memory-backed tests.
- [ ] Generate scenario delivery demand and automatically assign compatible
  available vehicles and drivers; explain unassigned deliveries.
- [x] Add scenario/run monitoring and read-only generated trip detail screens;
  remove manual trip creation and driver assignment controls.
- [ ] Add multi-stop route playback and package-event timelines.
- [x] Capture optional vehicle specifications and maintenance notes, and driver
  contact, license, shift, and availability without collecting sensitive
  identity fields; keep missing values explicitly unknown.
- [x] Add individual vehicle and driver profile pages.
- [ ] Link automatically dispatched scenario trips to fleet records and show
  those records' associated trip history.
- [ ] Record SUMO fuel/emissions model, units, and assumptions; separate
  simulated estimates from measured activity.
- [ ] Package or delegate SUMO execution for durable Cloud Run deployment; test
  recovery and observability across API restarts.

## Validation and rollout

- [ ] Add focused Go tests for scenario validation, deterministic inputs,
  assignment eligibility, run failure handling, output parsing, and event order.
- [ ] Add focused API and console tests for read-only trip flow, generated
  history, clear source labels, and unavailable-data states.
- [ ] Run Go formatting, vet, build, and tests, preserving any user-owned local
  changes; run the console tests, typecheck, and production build.
- [ ] Exercise a full scenario end to end on the verified DKI-wide network.
- [ ] Verify real GPS and SUMO map layers cannot be confused.
- [ ] Validate deployed SUMO runtime, database persistence, OSRM, attribution,
  and operational errors before production rollout.
