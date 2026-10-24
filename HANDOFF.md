# FleetFlow handoff

## Product direction

FleetFlow is a simulation-led fleet-operations console for Jakarta. Read
[AGENTS.md](./AGENTS.md) before making changes and use [PRD.md](./PRD.md) as the
source of truth. The product uses SUMO for reproducible scenario movement,
OSRM for external route requests, MapLibre for the map, and authenticated GPS
reports for real vehicle positions. Keep simulation and GPS data clearly
separated. Baseline-versus-dynamic comparisons and AI controls are out of scope.

## Current implementation

- The Go service has an OSRM HTTP client, coordinate-based route API, GPS
  ingestion and recent-position APIs, and OSRM/PostgreSQL readiness checks.
- The React console uses MapLibre/OpenFreeMap, submits OSRM route requests,
  polls recent GPS positions, and displays SUMO runs and generated trip history.
- `scripts/osrm/build-jakarta.ps1` prepares a regional OSM extract for OSRM;
  generated data is ignored by Git.
- `docker-compose.yml` defines the optional OSRM routing service.
- The tracked SUMO network is a legacy central-Jakarta artifact, not verified
  DKI-wide coverage. Do not describe it as full-city data.
- Automatic roster assignment, package delivery events, detailed fleet records,
  and durable Cloud Run execution are not implemented yet.

## Next steps

Use [TASKS.md](./TASKS.md) for validation and rollout items. Prioritize the
verified DKI-wide network, automatic dispatch, package event history, and
Cloud Run-compatible SUMO execution. Do not claim the full Jakarta dataset or
service workflow is verified until it has actually been generated and
exercised. Keep changes surgical and preserve unrelated user edits in the
working tree.
