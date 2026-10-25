# FleetFlow — submission summary

**Road To DevFest Jakarta 2026 · Apache-2.0 · Go + React**

## The problem

A Jakarta last-mile operator plans the day from memory: which van goes to
Cakung, how much fuel it costs, which driver is free. FleetFlow is a console for
that work — dispatch, trip history, a live map, and a carbon ledger — built
around one rule: a number a model produced must name the model that produced it.

## What it does

- **Live operations map.** Vehicles, the eleven Jakarta logistics hubs, stops
  and routes on MapLibre/OpenFreeMap tiles. Route geometry, distance and
  duration come from an OSRM server over a Jakarta extract of OpenStreetMap; the
  application computes no route of its own, and when OSRM is unreachable
  `/api/route` returns 503 instead of drawing something that looks like a road.
- **Scenarios.** Three seeded presets (peak, off-peak, night) generate a fleet's
  demand, trips, positions and deliveries. A scenario can be produced by the
  built-in Go generator or, when a SUMO network is configured, by running
  `duarouter` and `sumo`; the run records which one ran (`runner`) rather than
  implying SUMO always did.
- **Dispatch.** A completed run becomes fleet records: 100 vehicles with real
  ids and separate plates, 5 drivers, 170 trips, deduplicated by
  vehicle + origin + destination, each linked back to the run that produced it.
- **Sustainability.** One page: fleet totals, per-run ledger, per-vehicle
  attribution. Fuel and CO2 come from per-class factors: a 14 t diesel box truck
  burns 0.30 kg/km at 3.16 kg CO2/kg, a 4.5 t truck 0.18, a diesel van 0.11, a
  petrol van 0.09 at 3.10, an electric cargo motorcycle zero at the tailpipe.
  The class and the model name travel with every figure, and the page prints
  them, so two vehicles on the same kilometre no longer report the same gram.

## What is real, and what is not

Real: the fleet records, trip ids, dedupe, attribution and the arithmetic. Not
real: no vehicle, no operator, no GPS feed, and no measured Jakarta traffic.
Emissions are planning factors, not a certified inventory, and grid energy for
the electric motorcycle is not counted.

## Evidence

Mechanism evidence, not field evidence. On a synthetic 36-node grid with a
zero-share control, routing on own observations cut mean trip time by **20.21 s
over ten seeds (9 wins, 1 loss)**, and the effect is exactly zero when no vehicle
reports. `go test ./...` and `go test -race ./...` pass; the emissions difference
between a diesel truck and an electric motorcycle is a test, not a screenshot. I
cannot convert either result to rupiah: I have no operator data, and saying so
is cheaper than being asked.

## Running it

`docker compose up` for Postgres, then the API and console; OSRM is optional and
documented in `README.md`. Deployment notes and the Cloud Run service spec are in
`deploy/`.
