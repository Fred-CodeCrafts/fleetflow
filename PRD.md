# FleetFlow PRD

Road To DevFest Jakarta 2026 / Roro Jonggrang Sprint
Author: Frederick Garner Wibowo
Status: product direction updated 23 October 2026

## 1. Product scope

FleetFlow is a fleet-operations application for dispatchers working in Jakarta.
The MVP manages fleet records and trips, displays reported vehicle GPS positions
on a real street map, and requests driving routes from OSRM using an
OpenStreetMap road extract covering Jakarta and its surroundings.

The application is an operator tool, not a simulator or research-results viewer.
Routing must be performed by OSRM; FleetFlow must not substitute its own
shortest-path implementation or make up vehicle movement, traffic conditions,
route history, or emissions results. An optional static map preview may show
illustrative sample points only when it is clearly separated and labeled as
demo data, never as reported vehicle locations.

## 2. Users

| User | Need |
|---|---|
| Fleet dispatcher | Manage vehicles and drivers, create trips, inspect current reported positions, and get a driving route |
| Vehicle integration | Submit authenticated GPS positions for a known vehicle |
| Developer | Run the application and local OSRM dataset using documented setup |

## 3. Functional requirements

### Fleet records and trips

- Create and list vehicles with a name, type, and operating status.
- Create and list drivers; assign or unassign each driver to a vehicle.
- Create trips using geographic origin and destination coordinates and optionally assign an available vehicle.
- Advance trip status through planned, in progress, and completed.
- Preserve the existing PostgreSQL persistence behavior when a database is configured; clearly disclose that memory-backed records do not survive restart.

### Map and live vehicle locations

- Render an interactive Jakarta street map using MapLibre GL and a real basemap.
- Include visible map-data attribution.
- Accept GPS reports containing a vehicle ID, latitude, longitude, optional speed, and optional observation timestamp.
- Require an API key to submit positions; validate coordinates and timestamps.
- In Live GPS mode, display only recent positions actually received by the
  server; expire stale positions and clearly show the empty state.
- An explicitly selected Demo preview may show static sample points for UI
  demonstration, but must label them as illustrative, keep them visually
  distinct from GPS reports, and never send them to or read them from the GPS
  endpoint.
- Poll positions without overlapping requests and show API/map errors instead of silently displaying fabricated data.

### Routing

- Send origin and destination coordinates to the configured OSRM driving service.
- Return the OSRM route geometry, distance, and estimated duration to the console.
- Draw the returned geometry on the map and label its figures as OSRM estimates.
- Clearly report when OSRM is unavailable or cannot find a route. Do not fall back to a home-grown router.
- Use an OpenStreetMap PBF extract covering the Jakarta metropolitan area to build OSRM data with the MLD profile.
- Keep downloaded source data and generated OSRM files out of Git; document their source, geographic coverage, and rebuild steps.

## 4. Data and truthful display

- Map tiles come from OpenFreeMap; map rendering uses MapLibre GL.
- Routing comes from OSRM with an OSM-derived local road dataset. OSM attribution and ODbL terms must be retained.
- A route's distance and duration are OSRM routing estimates, not observed trip measurements or live traffic predictions.
- A vehicle marker represents the last GPS position submitted by that vehicle. It is not interpolated from road-network edges.
- Static demo-preview markers are illustrative map samples, not vehicles, GPS
  reports, or operational data.
- No historical position API currently exists; do not imply that the map shows a complete trip trace.
- Emissions tracking is deferred until there is a documented model or measured input and a clear method for presenting it. Do not show simulation-derived comparisons or unsupported emissions estimates.

## 5. Explicit non-goals

- Simulator controls, demo vehicle runs, synthetic traffic, or generated
  positions presented as live GPS. The opt-in static preview is not a vehicle
  simulator and must remain visibly labeled.
- Baseline-versus-dynamic or other routing comparisons, saved experiment results, or thesis metrics.
- FleetFlow-owned shortest-path routing, custom road-graph building, or SUMO network import in the operator workflow.
- Emissions dashboards or emissions claims in the current MVP.

## 6. Operational requirements

- `GET /healthz` reports process liveness.
- `GET /readyz` checks OSRM availability and PostgreSQL reachability when configured.
- OSRM's base URL is configured by `OSRM_URL`; errors remain visible to the operator.
- The local setup documents the OSRM data download/preprocessing and how the app connects to it.
- Keep the operator console usable at desktop and narrow viewport sizes, with keyboard-accessible controls and visible validation.
- Network errors must not be converted into success-shaped fallback data.

## 7. Acceptance checks

The MVP matches this PRD when a developer can prepare the documented Jakarta
OSRM dataset, start OSRM and the FleetFlow service, then:

1. Open the console and see a real Jakarta street map with attribution.
2. Choose origin and destination points on the map and receive an OSRM route,
   distance, duration, and route line.
3. Submit an authenticated GPS position and see it on the live map until it
   expires.
4. Manage vehicle, driver, and trip records through the console.
5. Receive actionable errors when OSRM is unavailable, route data is absent, a
   GPS report is invalid, or an API request fails.
6. Find no simulator controls, experiment comparisons, fabricated live data
   presented as live, or emissions dashboard in the operator UI; any static
   demo preview is explicit and visibly labeled.
