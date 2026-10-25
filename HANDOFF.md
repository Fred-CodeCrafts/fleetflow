# FleetFlow handoff

## Product direction

FleetFlow is a fleet-operations console for Jakarta. Read
[AGENTS.md](./AGENTS.md) before making changes and use [PRD.md](./PRD.md) as the
source of truth for scope. Scenarios run through Eclipse SUMO when a network is
configured, or through the built-in Go generator; every run records which one
produced it (`runner`). OSRM answers route requests, MapLibre draws the map, and
authenticated GPS reports are stored separately from simulation data. There are
no AI controls, and the console routes nothing of its own: OSRM answers or the
request fails with 503.

## Current implementation

- Go API: fleet, drivers, trips, GPS ingestion and recent positions, scenario
  runs (memory and PostgreSQL repositories), dispatch, emissions summary, the hub
  catalogue at `GET /api/hubs`, and Flyway-managed schema in `db/migration`.
- Dispatch turns a completed run into fleet records: vehicles get a unit name and
  a separate plate, trips are deduplicated by vehicle, origin and destination, and
  each trip links back to the run that produced it. Dispatch assigns no drivers;
  `scripts/dev/seed-fleet.ps1` pairs the demo drivers with vehicles.
- React console: MapLibre/OpenFreeMap map with hubs above routes and below
  vehicles, route planner, vehicle and driver detail pages with trip history, run
  ledger, and one sustainability page holding fleet totals, per-vehicle
  attribution and per-run history.
- Emissions have two honest paths. A SUMO run uses SUMO's HBEFA3 class
  (`HBEFA3/PC_G_EU4`, a generic petrol car). A synthetic run is charged by
  `internal/fleet` per vehicle class: 0.30 kg/km at 3.16 kg CO2/kg for a 14 t
  diesel truck down to zero tailpipe CO2 for the electric cargo motorcycle. The
  class travels with each trip and the console prints it. Neither path is a
  measured inventory.
- Routing has no fallback: `/api/route` returns 503 when OSRM fails and
  `/readyz` reports `routing: "unavailable"`. A failed route is never drawn.
- `scripts/osrm/build-jakarta.ps1` prepares the regional extract;
  `docker-compose.yml` runs the optional OSRM service on port 5001 by default
  because 5000 is commonly taken.
- The tracked SUMO network (`data/networks/jakarta.net.xml`) is a central-Jakarta
  artifact, not verified DKI-wide coverage. Do not describe it as city-wide.

## Next steps

1. **Deployment is blocked on billing, not on code.** The Cloud Run service
   `fleetflow` in project `fleetflow-4448` exists but answers 404/503: its billing
   account closed on 24 October, and Cloud Run refuses deploys without an open
   account. Restore billing, then `.\deploy\deploy.ps1 -Project fleetflow-4448 -Tag
   v0.1.0` puts the tagged image on the service and prints the URL.
2. Record the three-minute demo using the shot list and the expected questions in
   the README's Submission pack section.
3. Produce one real SUMO scenario run so the demo can show `runner: "sumo"`
   beside a synthetic run, and validate the Jakarta network if that is claimed.
4. Replace the emission planning factors with an operator's own fuel figures
   when one is available; until then do not convert the numbers to rupiah.
5. `main` carries the released tag; `dev` runs ahead of it between releases, and
   a release is a merge of `dev` into `main` plus a tag. Keep the small-commit
   habit and the branch model in [AGENTS.md](./AGENTS.md).
