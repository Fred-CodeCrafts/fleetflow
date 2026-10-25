# How to run the demo

A three-minute demonstration of FleetFlow, plus the ten minutes before it that
decide whether those three go well. The narrative and the expected questions are
in the README's [Submission pack](./README.md#submission-pack); this file is the
operator's checklist.

Record it, or run it live: the steps below are the same. Do them once, end to end,
with a timer, before you do them in front of anyone.

---

## 1. Pre-flight (about 10 minutes, once)

Run from the repository root.

```powershell
git checkout v0.1.5          # or the release tag you intend to demo
docker compose up -d postgres
docker compose --profile tools run --rm migrate
docker compose --profile routing up -d osrm
```

Port 5000 is often taken locally. If OSRM is published elsewhere, keep the number
consistent: `$env:OSRM_PORT='5001'; docker compose --profile routing up -d osrm`
and start the API with `OSRM_URL=http://127.0.0.1:5001`.

Build the console before the Go process, because the console is embedded in it:

```powershell
Set-Location console; npm ci; npm run build; Set-Location ..
go build -o fleetflow.exe ./cmd/fleetflow
```

Start the API with the database values from `.env` (PowerShell does not load it):

```powershell
$env:POSTGRES_HOST='127.0.0.1'; $env:POSTGRES_PORT='5432'
$env:POSTGRES_DATABASE='fleetflow'; $env:POSTGRES_USER='fleetflow'
$env:POSTGRES_PASSWORD='change-me'          # your local value
$env:OSRM_URL='http://127.0.0.1:5000'
# $env:SUMO_NETWORK='...\data\networks\generated\jakarta-dki.net.xml'   # optional
.\fleetflow.exe
```

Open <http://localhost:8080>. Leave `REQUIRE_OPERATOR_KEY` unset: the console has
no key to present, and the demo needs to write. `/readyz` will say
`writes: "unauthenticated"`, which is the truth and is fine to say out loud.

### Seed the state the demo assumes

```powershell
.\scripts\dev\seed-fleet.ps1      # five drivers, paired with registered vehicles
```

The three stored scenario presets (light 20, peak 50, heavy 100 vehicles) are
created and dispatched on startup, giving the register **100 vehicles and 170
trips** across the three runs. If they are missing, see [Reset](#6-reset-to-a-clean-state).

### The sixty-second check

```powershell
curl.exe -s http://127.0.0.1:8080/readyz
# {"database":"reachable","routing":"available","simulation":"available","status":"ok","writes":"unauthenticated"}

curl.exe -s http://127.0.0.1:8080/api/hubs | ConvertFrom-Json | % { $_.hubs.Count }        # 12
curl.exe -s http://127.0.0.1:8080/api/trips | ConvertFrom-Json | % { $_.trips.Count }      # 170
curl.exe -s http://127.0.0.1:8080/api/drivers | ConvertFrom-Json | % { $_.drivers.Count }  # 5
curl.exe -s http://127.0.0.1:8080/api/emissions/summary | ConvertFrom-Json | % { $_.totals }
# trips 170, distance_km ~2942, co2_kg ~1296, g_co2_per_km ~441
```

If `routing` is not `available`, the route-card scene has nothing to show. Start
OSRM before you start talking; **do not** present a route the server did not
compute.

---

## 2. The three minutes, click by click

Times are targets, not rules. Every screen is in the sidebar; nothing here needs a
typed URL.

| clock | click | what is on screen | say | the trap |
|---|---|---|---|---|
| 0:00–0:20 | **Operations → Live Map** | Hub markers, vehicle positions, the sidebar | "A fleet operations console for Jakarta: dispatch, trip history, a map, a carbon ledger, and one rule — a number a model produced has to name the model." | The GPS layer is empty unless a device reports. Say so if asked; it is not a bug. |
| 0:20–0:55 | **Operations → Route Planner** | Two hub selects, a drawn route card | Pick two hubs, press the button. "Twelve Jakarta hubs. This geometry, distance and duration come from OSRM over a Jakarta OpenStreetMap extract — this application computes no route itself." | If the card shows an error, stop and say routing is down. Do not describe a route that is not drawn. |
| 0:55–1:30 | **Operations → Simulation** | Preset cards, a run list, `runner` on each result | Start the **peak** preset. "Seeded demand, generated trips, positions, deliveries. When it finishes the panel says what produced it — synthetic generator, because no SUMO process ran. I could have labelled that SUMO; the field exists so I cannot." | A run takes a few seconds. If you are still waiting at 1:20, talk over it and open a previous completed run instead. |
| 1:30–2:05 | same screen → **Dispatch** | Fleet counts rise, then hold | Dispatch the run, then read the register: "100 vehicles with ids and separate plates, 170 trips, accumulated from the three stored presets. Press dispatch again and the counts do not move — a trip is deduplicated by vehicle, origin and destination, so a second click is not second work. The five drivers come from the demo seeder; dispatch assigns none." Open a vehicle and show its trips. | If the counts do not move on the **first** press, the run was already dispatched — say that, and open a vehicle instead of pressing again. |
| 2:05–2:45 | **Sustainability** | Totals, per-run ledger, per-vehicle rows with class | "A 14-tonne diesel box truck is 948 grams per kilometre, a 4.5-tonne truck 569, a diesel van 348, an electric cargo motorcycle zero at the tailpipe. The class is on the row; the model that produced it is named above the table: FleetFlow class factors — planning assumptions, not HBEFA3, not a certified inventory." Point at an electric row and the zero. Then the headroom panel: "if every van moved to a cargo motorcycle, about 30% of the ledger goes; that is an upper bound, it ignores payload, and a 1.2-tonne load does not fit a 60 kg motorcycle." | Do not call the factors "measured", and do not call the headroom a saving you achieved. |
| 2:45–3:00 | **any screen** | — | "Apache-2.0, tests in the repository, and the limits written down where you can check them." | — |

### If you have time left

- **Resources → Vehicles → a heavy truck**: plate, class, maintenance, trip history.
- **Operations → Trips**: newest first, with the route and package events on a trip.
- `/readyz` in a browser: five fields, all true, including the engine and the write mode.

---

## 3. What to say when it breaks

| symptom | what it means | what to do |
|---|---|---|
| Route card shows an error, `/readyz` says `routing: "unavailable"` | OSRM is not running or the URL is wrong | Say routing is down, show the hub map instead, move on. Never narrate a route that is not drawn. |
| `/readyz` says `database: "unreachable"` | Postgres is down or the credentials differ | Restart the container and re-check; the console will be empty until it reconnects. |
| Simulation returns 503 with a routing message | A run needs OSRM too | Same as above; the run is refused rather than faked. |
| A page is blank or 404 | The Go binary was built before the console | `npm run build` in `console\`, rebuild, restart. |
| Port 8080 busy | Another copy is running | Stop it, or start with `PORT=8090`. |
| Dispatch adds nothing | That run was already dispatched | Expected. Say the dedupe line and show a vehicle's trips. |
| The run says `high` traffic and 59 minutes of trips | The scenario clock is simulated | Say so: the console shows simulated timestamps, not wall clock. |

The one unrecoverable failure is a fabricated answer. A missing number is a
sentence; an invented one is the end of the demonstration.

---

## 4. Reset to a clean state

Only when the counts in §1 are wrong. This deletes the fleet records and the
stored presets; it is a development database operation.

```powershell
docker exec fleetflow-postgres psql -U fleetflow -d fleetflow -c `
  "BEGIN; DELETE FROM trips; DELETE FROM drivers; DELETE FROM vehicles; DELETE FROM simulation_runs; COMMIT;"
```

Stop the API, start it again: it re-seeds the three presets and dispatches them,
which is where the 100 vehicles and 170 trips come from. Then run
`.\scripts\dev\seed-fleet.ps1` for the five drivers, and re-check §1.

---

## 5. Recording it

- 1920×1080, the console at full width, no second monitor.
- Record the run: do not stop mid-scene to fix a typo; a second take is cheaper.
- Zoom the browser to 100%. At 125% the sustainability table loses its class column.
- Keep the cursor still while talking. The click is the emphasis.
- End on `/readyz` if you have five spare seconds: five fields, all true.
- The README's shot list is the same script; if you change one, change both.

---

## 6. After the demo

```powershell
docker compose --profile routing stop osrm
docker compose stop postgres
```

Nothing in this flow writes a secret to disk: `.env` stays untracked, and the
process reads credentials from its own environment.
