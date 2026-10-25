# Three-minute demo script

One take, 180 seconds, screen recording plus voice. Nothing on screen may claim
more than the API says: if a run is synthetic, the panel says "FleetFlow synthetic
generator", and you say the same word.

## Pre-flight (10 minutes before recording)

1. `docker compose up -d postgres` and `docker compose --profile routing up -d osrm`
   (OSRM on 5001 if 5000 is taken). Confirm `curl localhost:5001/route/v1/...`
   answers.
2. Start the API with `OSRM_URL=http://127.0.0.1:5001`; open `/readyz` and check
   `routing: available`, `database: reachable`.
3. Confirm `/sustainability` shows three runs and distinct per-class intensities;
   if it shows one identical figure, the old runs are still in the database —
   delete them so the presets regenerate.
4. Close every other tab, terminal, and notification. Zoom the browser to 100%.
5. Do one silent run-through on `/route-planning`, `/operations`, `/sustainability`
   so no page is loading for the first time on camera.

## Shot list

**0:00–0:20 — What this is.**
Landing page. "FleetFlow is a fleet operations console for Jakarta: dispatch,
trip history, a live map, and a carbon ledger. One rule the whole thing is built
on: a number a model produced has to name the model."

**0:20–0:55 — The map.**
Open `/route-planning`. "Eleven Jakarta logistics hubs, the Soekarno-Hatta
cluster included. The geometry, the distance and the duration come from OSRM over
a Jakarta OpenStreetMap extract. This application computes no route itself."
Hover a hub, draw one route. Do not narrate the fallback — there isn't one.

**0:55–1:30 — A scenario runs.**
`/operations/simulations`, start the peak preset. While it runs: "Seeded demand,
generated trips, positions and deliveries. When it finishes, the panel tells you
what produced it: this one says synthetic generator, because no SUMO process ran
here." Point at the runner field. "I could have labelled it SUMO. It would have
been a lie."

**1:30–2:05 — Dispatch.**
Dispatch the completed run. "That run becomes fleet records: 100 vehicles with
ids and separate plates, five drivers, 170 trips. Repeat trips are refused by
vehicle, origin and destination, so a double click does not become double work."
Open one vehicle: show trips, not a placeholder.

**2:05–2:45 — The carbon ledger.**
`/sustainability`. Point at the class column. "A 14-tonne diesel box truck is
948 grams per kilometre. A 4.5-tonne truck 569. A diesel van 348. An electric
cargo motorcycle zero at the tailpipe. The arithmetic is per class, the class is
on the row, and the model name is printed above it: FleetFlow class factors,
planning assumptions, not HBEFA3 and not a certified inventory."

**2:45–3:00 — The honest close.**
"Evidence is mechanism evidence, not field evidence: 20.21 seconds off mean trip
time over ten seeds on a synthetic grid, and it is zero when no vehicle reports.
I cannot give you rupiah, because I have no operator. Apache-2.0, tests in the
repo, and the Q&A is in `docs/qa.md`."

## Do not

- Do not say "SUMO simulation" over a synthetic run; say "scenario".
- Do not call the emissions figures measured, certified, or HBEFA3.
- Do not claim a Cloud Run URL on camera unless that revision is serving; the
  deploy is blocked on the project's closed billing account.
- Do not show `/readyz` while OSRM is down unless the point is that it says so.
