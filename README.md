# FleetFlow

FleetFlow is a fleet operations console for Jakarta. Dispatchers can manage
vehicles, drivers, and trips; plan driving routes through OSRM; and see GPS
positions that connected vehicles have actually reported.

The console uses MapLibre GL with OpenFreeMap tiles. FleetFlow does not generate
traffic, run a simulator, compare routing experiments, or invent vehicle
positions or emissions measurements.

## Routing and map data

Route calculations are delegated to the OSRM driving profile. The application
does not run its own shortest-path implementation. The map is rendered from
OpenFreeMap; the local OSRM service routes over a Jakarta/Jabodetabek extract of
OpenStreetMap roads.

The first-time dataset setup downloads Geofabrik's Java PBF (about 856 MB at the
time this workflow was added), clips it to a buffered Jakarta metropolitan
bounding box, and builds the OSRM MLD data files. Allow several gigabytes of
free disk space and time for preprocessing. The generated PBF and OSRM files
stay under `data/osrm/`, which is ignored by Git.

Requirements: Docker Desktop, PowerShell, Go 1.25.5+, and Node.js/npm.

```powershell
# From the repository root. First run downloads the OSM PBF and builds OSRM files.
.\scripts\osrm\build-jakarta.ps1

# Start OSRM after the data build has completed.
docker compose --profile routing up -d osrm

# Build and embed the console.
Set-Location console
npm ci
npm run build
Set-Location ..

# The Go app defaults to http://127.0.0.1:5000 for OSRM.
go run ./cmd/fleetflow
```

Open `http://localhost:8080`. For console-only development, start the Go service
in one terminal and run `npm run dev` from `console/` in another; Vite proxies
API requests to port 8080.

To run the Go app in a container while OSRM is running on the host, set
`OSRM_URL=http://host.docker.internal:5000`. If both are services in the same
Compose network, use `OSRM_URL=http://osrm:5000`.

The Java extract includes Jakarta and surrounding routes; the configured
bounding box is a routing buffer, not an official administrative border. To
change coverage, update the `--bbox` in `scripts/osrm/build-jakarta.ps1` and
rebuild the dataset with `-Force`.

OpenStreetMap data is licensed under ODbL. Attribution: © OpenStreetMap
contributors. OpenFreeMap basemap tiles require an internet connection and
their attribution is displayed on the map.

## Operations

- **Live Map** shows only recently reported GPS locations. It starts empty until
  a connected device sends positions.
- **Trips** lets dispatchers choose actual map coordinates and assign an
  available vehicle.
- **Route Planner** sends the selected coordinates to OSRM and draws the returned
  route geometry, distance, and estimated duration.
- **Vehicles** and **Drivers** manage fleet records and assignments.
- **General Settings** reports app health and the routing/map providers.

There is no simulation-run or comparison screen. Emissions tracking is not part
of this implementation; do not treat a model estimate as measured output.

## API

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Process liveness |
| `GET /readyz` | OSRM availability and database readiness |
| `POST /api/route` | Route coordinates with OSRM; returns GeoJSON coordinates, distance, and duration |
| `GET /api/locations` | Recent GPS positions |
| `POST /api/locations` | Submit a vehicle GPS position (API key required) |
| `GET /api/vehicles`, `POST /api/vehicles` | Vehicle records |
| `POST /api/vehicles/{id}/driver` | Assign or unassign a driver |
| `GET /api/drivers`, `POST /api/drivers` | Driver records |
| `GET /api/trips`, `POST /api/trips` | Trip records |
| `POST /api/trips/{id}/status` | Advance a trip through its lifecycle |

Example route request:

```json
{
  "from": { "lat": -6.2088, "lon": 106.8456 },
  "to": { "lat": -6.1754, "lon": 106.8272 }
}
```

Example authenticated vehicle position:

```sh
curl -sS -X POST http://localhost:8080/api/locations \
  -H "Authorization: Bearer $FLEETFLOW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"vehicle_id":"van-17","lat":-6.2088,"lon":106.8456,"speed_mps":8.5}'
```

Issue an API key with `go run ./cmd/fleetflow -issue-key "vehicle integration"`.
Key issuance requires PostgreSQL; plaintext is printed once and only its hash
is stored.

## Persistence

Fleet records use PostgreSQL when `DATABASE_URL` or the `POSTGRES_*` settings
are configured. Without a database, records and API keys use in-memory stores
and are lost when the service restarts. Start the local database with
`docker compose up -d`; apply migrations with
`docker compose --profile tools run --rm migrate`.

Copy `.env.example` to `.env` for the Compose database values. Set `OSRM_URL`
in the environment of the Go process; local shell processes do not load `.env`
automatically.

## Status

FleetFlow is an MVP in active development for Road To DevFest Jakarta 2026. It
is not production-ready: live vehicle integrations, access control for the
operator console, backups, multi-instance support, and a verified deployment
remain outstanding.
