# FleetFlow

FleetFlow is a fleet operations console for Jakarta: dispatch, trip history, a
live map, and a carbon ledger. Scenarios can run through Eclipse SUMO when a
network is configured, or through the built-in Go generator; every run records
which one produced it, and the console prints that field rather than implying
SUMO always ran. The console also manages fleet records, routes requests through
OSRM, and keeps real GPS reports separate from simulation data.

The console uses MapLibre GL with OpenFreeMap tiles. FleetFlow does not generate
real traffic and does not present simulated positions or modelled emissions as
measured fleet data. The written summary is [docs/summary.md](./docs/summary.md)
(496 words) and the business model is
[docs/business-model.md](./docs/business-model.md); the demo shot list and the
questions this repository can answer are in
[Submission pack](#submission-pack); deployment notes are in
[deploy/README.md](./deploy/README.md).

## Routing, map, and simulation data

Route calculations are delegated to the OSRM driving profile. The application
does not run its own shortest-path implementation. The map is rendered from
OpenFreeMap; the local OSRM service routes over a Jakarta/Jabodetabek extract of
OpenStreetMap roads.

The first-time dataset setup downloads Geofabrik's Java PBF (about 856 MB at the
time this workflow was added), clips it to a buffered Jakarta metropolitan
bounding box, and builds the OSRM MLD data files. Allow several gigabytes of
free disk space and time for preprocessing. The generated PBF and OSRM files
stay under `data/osrm/`, which is ignored by Git.

Requirements: Windows with PowerShell, Docker Desktop, Go 1.25.5+, Node.js/npm,
Python, and a local SUMO installation with `sumo`, `duarouter`, `netconvert`,
and `tools\randomTrips.py` available. Initial OSM downloads are large; allow
several gigabytes of free disk space for preprocessing.

### One-time data setup

Run from the FleetFlow repository root:

```powershell
# Download and prepare the OSRM Jakarta road extract.
.\scripts\osrm\build-jakarta.ps1

# Install the PBF reader and create the DKI SUMO network.
python -m pip install -r scripts/sumo/requirements.txt
.\scripts\sumo\build-jakarta.ps1
```

These generated datasets are local-only, ignored by Git, and are not included
in the application image. The DKI network passes structural validation; full
city SUMO route/connectivity validation has not been completed.

### Start local services

1. Create a local environment file and start PostgreSQL:

   ```powershell
   if (-not (Test-Path .env)) { Copy-Item .env.example .env }
   docker compose up -d postgres
   docker compose --profile tools run --rm migrate
   ```

   If you change credentials in `.env`, use the same values for the Go process
   below. PowerShell does not automatically load `.env`.

2. Start OSRM after its data build has completed:

   ```powershell
   docker compose --profile routing up -d osrm
   ```

   Port 5000 is commonly taken by other local services. Publish OSRM on a
   different host port and point the API at the same one:

   ```powershell
   $env:OSRM_PORT = '5001'
   docker compose --profile routing up -d osrm
   # then start the Go process with OSRM_URL=http://127.0.0.1:5001
   ```

3. In a second terminal, start the Go API from the repository root. Adjust
   `SUMO_HOME` if SUMO is installed elsewhere:

   ```powershell
   $env:OSRM_URL = 'http://127.0.0.1:5000'
   $env:SUMO_HOME = 'C:\Program Files (x86)\Eclipse\Sumo'
   $env:SUMO_NETWORK = (Resolve-Path '.\data\networks\generated\jakarta-dki.net.xml').Path
   $env:POSTGRES_HOST = '127.0.0.1'
   $env:POSTGRES_PORT = '5432'
   $env:POSTGRES_DATABASE = 'fleetflow'
   $env:POSTGRES_USER = 'fleetflow'
   $env:POSTGRES_PASSWORD = 'change-me'
   go run ./cmd/fleetflow
   ```

   The database values match `.env.example`. Use your local values if you
   changed them. To develop without SUMO, leave `SUMO_NETWORK` unset.

4. In a third terminal, start the console:

   ```powershell
   Set-Location -LiteralPath '.\console'
   npm ci
   npm run dev
   ```

   Open the local URL printed by Vite (normally `http://localhost:5173`). Vite
   proxies API calls to port 8080. To serve the production console from Go,
   run `npm run build` in `console\` before starting the Go process, then open
   `http://localhost:8080`.

5. Optionally seed demo drivers and allocate them to the registered fleet:

   ```powershell
   .\scripts\dev\seed-fleet.ps1
   ```

   Start a scenario from **Operations → SUMO Scenarios**. The live GPS map
   remains empty until a device submits authenticated real position reports.

SUMO scenarios require the generated `data/networks/generated/jakarta-dki.net.xml`.
The build clips the Geofabrik
Java extract with pyosmium to the OSM administrative polygon of DKI Jakarta
(relation 6362934), buffered by about 2 km, including the Thousand Islands. It
structurally validates the generated network. `-ValidateWithSumo` additionally
loads it into SUMO and can require substantial memory for a full-city network.
Generated PBF/XML/network files are local data and are ignored by Git. The
tracked `data/networks/jakarta.net.xml` remains a legacy central-Jakarta
artifact and is not used by the app.

Start a scenario from **Operations → SUMO Scenarios**, or call the API:

```sh
curl -sS -X POST http://localhost:8080/api/simulation/runs \
  -H "Content-Type: application/json" \
  -d '{"seed":42,"vehicles":20,"departure_window_s":300}'
```

Scenario output is generated by SUMO and is separate from authenticated GPS
reports. Run metadata/results persist when PostgreSQL is configured; an
in-memory local run loses them when the process restarts. Delivery stops and
package events are generated from simulation traces; they are not real scans.

## Seed local demo records

With the FleetFlow API running locally, seed sample drivers and pair each with a
registered vehicle:

```powershell
.\scripts\dev\seed-fleet.ps1
```

The fleet belongs to the scenarios: a completed run registers the vehicles it
drove, with their specifications, so run one from **Operations → SUMO
Scenarios** first if the register is empty. The seeder then adds five drivers
and gives each a free vehicle, once; a driver who already holds a vehicle keeps
it, so repeat runs change nothing. It creates no vehicles, trips or GPS
positions. Records use the running application's configured store, so a
memory-backed local run loses them when the service restarts. This is a
development convenience, not a production initialization command. Remote targets
are refused unless `-AllowRemote` is
explicitly provided; never use it against production.

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

- **Simulation** starts a seeded scenario, shows its progress, and draws the run's
  animated positions with a playback bar.
- **Trips** lists history from simulation runs, newest first, and is where a scheduled trip appears once the Route Planner creates it. Trip details show the
  vehicle's sampled simulated route, generated delivery stops, and ordered
  package-event timeline when available. These seeded events are simulation
  output, not live package scans.
- **Live Map** shows recently reported GPS locations, refreshed every five
  seconds, and can start a seeded scenario; the run's playback controls live on
  the Simulation page. It starts empty until a connected device sends positions,
  and it does not show static preview markers.
- **Route Planner** sends the selected coordinates to OSRM and draws the returned
  route geometry, distance, and estimated duration.
- **Vehicles** and **Drivers** capture non-sensitive fleet profiles and display
  current allocations. Each record links to its own detail page; manual driver
  assignment is not exposed.
- **General Settings** reports app health and the routing/map providers.

### Emissions: two paths, each named

What a trip emits depends on which runner produced it, and the run says which.

- A **SUMO run** uses SUMO's HBEFA3 model: the route file assigns
  `HBEFA3/PC_G_EU4` (a Euro 4 petrol passenger car) to every generated vehicle,
  and `sumo` reports the emission values, which `internal/simulation/sumo`
  converts from milligrams to kilograms. That class is a generic stand-in, not a
  description of a Jakarta box truck, and the run stores it as such.
- A **synthetic run** is charged by `internal/fleet`, which holds one fuel figure
  per vehicle class and the carbon content of that fuel: 0.30 kg/km at 3.16 kg
  CO2/kg for a 14 t diesel box truck, 0.18 for a 4.5 t truck, 0.11 for a diesel
  van, 0.09 at 3.10 for a petrol van, and zero tailpipe CO2 for the electric
  cargo motorcycle. These are planning assumptions for congested urban delivery
  work, stated so the arithmetic can be checked by hand, and the API returns the
  class next to every figure.

Neither path is a measured inventory, and neither converts to rupiah. The
`sustainability` page prints the model name with the numbers for that reason.

Automatic fleet dispatch, scenario playback on the map, and trip history on
individual fleet records are implemented; `/api/hubs` serves the hub catalogue
the map draws.

## API

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Process liveness |
| `GET /readyz` | OSRM availability and database readiness |
| `POST /api/route` | Route coordinates with OSRM; returns GeoJSON coordinates, distance, and duration |
| `GET /api/locations` | Recent GPS positions |
| `POST /api/locations` | Submit a vehicle GPS position (API key required) |
| `GET /api/vehicles`, `POST /api/vehicles` | Vehicle records |
| `GET /api/drivers`, `POST /api/drivers` | Driver records |
| `POST /api/simulation/runs` | Start a seeded SUMO scenario |
| `GET /api/simulation/runs` | List scenario runs |
| `GET /api/simulation/runs/{id}` | Inspect run status and results |

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
and are lost when the service restarts. Simulation runs use PostgreSQL JSONB
when configured, or in-memory storage otherwise. Start the local database with
`docker compose up -d`; apply migrations with
`docker compose --profile tools run --rm migrate`.

Copy `.env.example` to `.env` for the Compose database values. Set `OSRM_URL`
in the environment of the Go process; local shell processes do not load `.env`
automatically.

`REQUIRE_OPERATOR_KEY=true` closes the seven write routes to callers without an
API key (registering vehicles and drivers, assigning a driver, creating a trip,
changing a trip's status, and starting a scenario). It is unset by default so the
demonstration console, which has no key to present, can write; `/readyz` reports
`writes: "protected"` or `"unauthenticated"` so the running mode is never a guess.
Reads stay public in both modes.

## Where the dynamic-weights idea came from

The routing idea behind FleetFlow comes from the author's thesis proposal,
*Implementation of Stateful Dynamic Edge Weights to Avoid Congestion in SUMO-Based
Routing Systems*: per-edge congestion state derived from probe reports,
exponential smoothing, temporal decay, and metric customization. It is a proposal,
not a finished study, and **this repository does not implement the research it
describes.** There is no CCH here, no TraCI loop and no LuST scenario, and the
console routes through OSRM rather than a FleetFlow-owned router.

An earlier prototype in this repository applied the *idea* at a much smaller
scale: per-edge smoothed congestion written by the vehicles' own speed reports,
decay for edges that stopped reporting, and an immutable metric snapshot behind an
atomic pointer so a route query never takes a write lock. It was removed on
25 October because nothing in the console consumed it, its grid came from a file
that was already gone, and its numbers could not be reproduced from a clone.

It is back, because both reasons are answered. **Settings → Routing Lab** runs
the comparison from a seeded configuration, and the grid is built in code, so a
clone reproduces it with nothing but the repository. `GET /api/lab/dynamic` answers
with the same experiment for anyone reading the API.

The experiment routes identical seeded demand twice on a 6×6 lattice with fast
arterials every third street: once on free-flow times, once on congestion the
dynamic engine learned from the fleet's own reports. On the default configuration,
ten seeds at 50% adoption improve between **4.6% and 8.4%**, mean **6.1%**, and 0%
adoption ties the baseline exactly — kept as a control, so a reader can see the
difference comes from the routing policy and not from noise. Those numbers describe
this model. The screen and the endpoint both print what it is not: no CCH, no TraCI,
no LuST scenario, no Jakarta street plan, no measured traffic. The proposal is where
that research is heading; this repository is the console built alongside it.

## Status

FleetFlow is an MVP built for Road To DevFest Jakarta 2026. Dispatch, scenario
playback on the map and per-vehicle trip history ship. The image bundles the SUMO
packages Debian provides plus `data/networks/jakarta.net.xml`, which is a
central-Jakarta artifact and not verified DKI-wide coverage. The Cloud Run
deployment is blocked on an open billing account; see [deploy/README.md](./deploy/README.md).

## Submission pack

### What this is

A Jakarta fleet operations console: dispatch, trip history, a live map, and a
carbon ledger, built around one rule — a number a model produced must name the
model that produced it.

Real: the fleet records, trip ids, deduplication, attribution, and the arithmetic
behind every figure. Simulated: every vehicle, every trip, every position, and
every emission number. Emissions are per-class planning factors for congested
urban delivery work, not a certified inventory; the electric cargo motorcycle
reports zero tailpipe CO2 and its grid energy is not counted.

### The three-minute demo

Pre-flight: Postgres and OSRM up; API started with `OSRM_URL`; run
`scripts/dev/seed-fleet.ps1` so the five demo drivers exist and are paired with
vehicles; `/readyz` showing `routing: available`, `database: reachable` and the
`writes` mode you expect; `/sustainability` showing three runs with distinct
per-class intensities. Close everything else.

| time | screen | say |
|---|---|---|
| 0:00–0:20 | landing | "A fleet operations console for Jakarta: dispatch, trip history, a live map, a carbon ledger. One rule: a number a model produced has to name the model." |
| 0:20–0:55 | `/operations/routes` | "Twelve Jakarta hubs. Geometry, distance and duration come from OSRM over a Jakarta OpenStreetMap extract; this application computes no route itself." Draw one route. |
| 0:55–1:30 | `/operations/simulations` | Start the peak preset. "Seeded demand, generated trips, positions, deliveries. When it finishes, the panel says what produced it: synthetic generator, because no SUMO process ran. I could have labelled it SUMO." Point at `runner`. |
| 1:30–2:05 | `/operations/trips` | "The run became fleet records: one hundred vehicles with ids and separate plates, two hundred and twenty trips. The five drivers come from the demo seeder and dispatch assigns none. A trip is deduplicated by vehicle, origin and destination, so recording the same delivery twice is refused." Open a vehicle: trips, not a placeholder. |
| 2:05–2:45 | `/sustainability` | "A 14-tonne diesel box truck is 948 grams per kilometre, a 4.5-tonne truck 569, a diesel van 348, an electric cargo motorcycle zero at the tailpipe. The class is on the row and the model is named above it: FleetFlow class factors, planning assumptions — not HBEFA3, not a certified inventory." |
| 2:45–3:00 | close | "Apache-2.0, tests in the repo, and the limits are written down where you can check them." |

Do not say "SUMO simulation" over a synthetic run, do not call the factors
measured or HBEFA3, and do not claim a Cloud Run URL while the service is down.

### Questions this repository can answer

**"Which numbers in this demo were actually produced by SUMO?"** None of the
stored runs. Every result carries `runner`: `"synthetic"` for the Go generator,
`"sumo"` when `duarouter` and `sumo` ran, with `sumo_version` empty otherwise. The
SUMO path is wired in `internal/simulation/sumo` and parses tripinfo, fcd and
emission output under SUMO's HBEFA3 class; no validated Jakarta SUMO run exists on
this machine, and the console does not pretend one does.

**"What is the evidence this helps a Jakarta fleet, in money?"** There is no field
evidence. What can be checked is the mechanism: routing is OSRM or a 503, the
emission arithmetic is one table in `internal/fleet` with the class printed beside
every figure, and `internal/fleet/fleet_test.go` proves a diesel truck and an
electric motorcycle differ on the same kilometre. No operator, no measured Jakarta
speeds, no fuel bills — so any rupiah figure would be invented, and none is given.

**"Your writes are unauthenticated. What stops me scripting your booth?"** This
process can close them: `REQUIRE_OPERATOR_KEY=true` makes every write that changes
fleet data require the same API key as GPS ingestion, and `/readyz` publishes
which mode is running (`writes: "protected"` or `"unauthenticated"`). The
demonstration runs in the open mode because the console has no key to present, and
it says so rather than implying it is safe. Independently of the mode, a rate limit
of 100 requests per second with a burst of 200 and a 64 KiB body cap keep one
caller from wedging the process: availability, not authorisation.
