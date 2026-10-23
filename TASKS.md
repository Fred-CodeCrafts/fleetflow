# FleetFlow delivery checklist

[PRD.md](./PRD.md) is the product source of truth; [DESIGN.md](./DESIGN.md)
describes the current architecture. This checklist tracks the real-fleet
direction: OSRM routing, reported GPS positions, and no simulator/comparison
workflow.

## Implementation

- [x] Add an OSRM client and route API using geographic coordinates.
- [x] Configure OSRM as a separate service and add a Jakarta-region OSM-to-OSRM
  dataset preparation workflow.
- [x] Replace the console's simulation/comparison workflow with a MapLibre map,
  route planner, and GPS position view.
- [x] Add authenticated vehicle-position ingestion and recent-position reads.
- [x] Remove simulator, comparison, graph-import, reports, and emissions
  endpoints and console pages from the active application.
- [x] Update the README and product requirements for the chosen direction.

## Validation and rollout

- [x] Re-run console tests and the production build after the UI changes.
- [x] Run Go tests for routing, locations, config, and API readiness.
- [ ] Validate the dataset-preparation script and OSRM Compose service end to
  end; document verified runtime results.
- [ ] Smoke-test a route request against the local Jakarta OSRM instance and
  verify its returned geometry on the map.
- [ ] Confirm API failure states, GPS expiry, and attribution in the running
  console.
- [ ] Review any remaining legacy simulation code and remove it only after
  confirming it is unused and its removal is safe.
