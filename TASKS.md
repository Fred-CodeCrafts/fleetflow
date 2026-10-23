# FleetFlow delivery checklist

[PRD.md](./PRD.md) defines the simulation-led product direction;
[DESIGN.md](./DESIGN.md) records the intended architecture and its current
implementation caveats.

## Existing foundation

- [x] React/TypeScript operator console and Go API.
- [x] MapLibre map, OpenFreeMap tiles, and visible map attribution.
- [x] OSRM route client/API and Jakarta-region OSM data preparation workflow.
- [x] Authenticated GPS ingestion with a separate live-GPS view.
- [ ] Remove the legacy static map preview; replace it with SUMO run playback.
- [x] PostgreSQL persistence for existing fleet, driver, and trip records.

## SUMO simulation and delivery operations

- [x] Verify local SUMO 1.27.1 can generate seeded demand with `randomTrips.py`,
  route it with `duarouter`, and produce tripinfo, geographic FCD, and emission
  XML using the retained Jakarta network. This smoke test only establishes the
  local toolchain; it does not validate network coverage or production setup.
- [ ] Obtain and document a current OSM-derived road network covering all DKI
  Jakarta; verify its extent against the administrative boundary, provenance,
  license, connectivity, and representative routes. Keep source/generated data
  out of Git.
- [ ] Add validated SUMO runtime/network configuration and clear readiness and
  process failure reporting.
- [ ] Add seeded scenario presets and a reproducible SUMO run lifecycle;
  capture network/version/seed and persist run state.
- [ ] Persist SUMO positions, completed trips, ordered stops, package events,
  and history with a new database migration and memory-backed tests.
- [ ] Generate scenario delivery demand and automatically assign compatible
  available vehicles and drivers; explain unassigned deliveries.
- [ ] Add scenario/run monitoring and read-only trip detail screens with route
  playback and package-event timelines. Remove manual trip creation and
  assignment from the scenario workflow.
- [ ] Expand vehicle details with specifications and maintenance history; add
  driver contact, license, shift, and availability without unnecessary
  sensitive identity fields.
- [ ] Report SUMO fuel/emissions as model estimates with model, units, and
  assumptions; separate simulated estimates from measured activity.
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
