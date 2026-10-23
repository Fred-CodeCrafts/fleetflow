# FleetFlow handoff

## Product direction

FleetFlow is a real-fleet operations console for Jakarta. Read
[AGENTS.md](./AGENTS.md) before making changes and use [PRD.md](./PRD.md) as the
source of truth. The product uses OSRM for road routing, MapLibre for the map,
and authenticated GPS reports for vehicle positions. Do not restore simulation,
run comparisons, fabricate vehicle movement, or display unsupported emissions
figures.

## Current implementation

- The Go service has an OSRM HTTP client, coordinate-based route API, GPS
  ingestion and recent-position APIs, and OSRM/PostgreSQL readiness checks.
- The React console uses MapLibre/OpenFreeMap, submits OSRM route requests, and
  polls recent GPS positions.
- `scripts/osrm/build-jakarta.ps1` prepares a regional OSM extract for OSRM;
  generated data is ignored by Git.
- `docker-compose.yml` defines the optional OSRM routing service.
- The previous SUMO network and simulation-related Go packages may remain in
  the repository, but they must not be wired into the current app.

## Next steps

Use [TASKS.md](./TASKS.md) for validation and rollout items. Prioritize running
the console tests/build, focused Go tests, and an end-to-end local OSRM route
smoke test. Do not claim the full Jakarta dataset or service workflow is
verified until it has actually been generated and exercised. Keep changes
surgical and preserve unrelated user edits in the working tree.
