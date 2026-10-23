# FleetFlow console design

The console is an operations workspace for seeded SUMO runs, generated trips,
fleet records, and separately reported real locations. Keep the interface
operational and explicit about the source and freshness of data.

## Primary screens

- **Operations:** SUMO scenario controls, generated trip history, a live-GPS
  map with last-reported status, and an OSRM route planner.
- **Fleet:** Vehicle records, type, and operating status.
- **Drivers:** Driver records and read-only current vehicle allocation.
- **Trips:** Read-only generated trip records and per-trip route history.
- **Settings:** Operational configuration and API guidance; do not expose
  comparison or AI controls.

## Map behavior

- Use the real OpenFreeMap basemap through MapLibre GL and retain map
  attribution.
- Display the route geometry returned by OSRM, with distance and duration
  labeled as routing estimates.
- Display only GPS positions returned by the FleetFlow API on the live-GPS
  layer. Show when each position was reported and make stale/empty states clear.
- Display SUMO positions only on a clearly labeled simulation map. Never
  represent a simulated route or position as live GPS.
- Provide loading, no-route, service-unavailable, and location-fetch error
  states. Never draw fallback routes or synthetic vehicle markers.
- Keep map controls and forms usable with keyboard navigation and on narrow
  screens.

## Product language

Use clear labels such as “Last reported”, “Route estimate”, “OSRM”, and
“Simulation output”. Label fuel and emissions as SUMO model estimates; never
claim those values are measured or represent verified carbon savings.
