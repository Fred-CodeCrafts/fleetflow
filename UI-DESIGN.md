# FleetFlow console design

The console is a dispatcher workspace for real fleet records and reported
locations, not a simulation-results dashboard. Keep the interface map-centered,
operational, and explicit about the source and freshness of data.

## Primary screens

- **Operations:** MapLibre street map, recent GPS markers with last-reported
  status, and a route planner using origin/destination map selection.
- **Fleet:** Vehicle records, type, and operating status.
- **Drivers:** Driver records and vehicle assignment.
- **Trips:** Planned, in-progress, and completed trip records.
- **Settings:** Operational configuration and API guidance; do not expose
  simulator or comparison settings.

## Map behavior

- Use the real OpenFreeMap basemap through MapLibre GL and retain map
  attribution.
- Display the route geometry returned by OSRM, with distance and duration
  labeled as routing estimates.
- Display only GPS positions returned by the FleetFlow API. Show when each
  position was reported and make stale/empty states clear.
- Provide loading, no-route, service-unavailable, and location-fetch error
  states. Never draw fallback routes or synthetic vehicle markers.
- Keep map controls and forms usable with keyboard navigation and on narrow
  screens.

## Product language

Use clear labels such as “Last reported”, “Route estimate”, and “OSRM”. Do not
claim live traffic prediction, observed trip history, fuel usage, or emissions
unless those values are actually measured or produced by a documented model.
