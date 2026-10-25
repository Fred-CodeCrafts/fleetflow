---
title: FleetFlow
emoji: 🚚
colorFrom: green
colorTo: blue
sdk: docker
app_port: 7860
pinned: false
license: apache-2.0
short_description: Jakarta fleet operations console with a carbon ledger
---

# FleetFlow

A fleet-operations console for Jakarta: seeded traffic scenarios, dispatch into
fleet records, trip history on a map, and a carbon ledger whose every figure names
the model that produced it.

Source, tests and the full documentation:
**https://github.com/Fred-CodeCrafts/fleetflow**

## What runs here

This Space runs the console and the API in memory: three seeded scenario runs,
about a hundred vehicles and a hundred and seventy trips, the emissions ledger, and
the Routing Lab that compares static with dynamic edge weights.

## What does not

- **The Route Planner needs OSRM**, a separate routing service with a multi-gigabyte
  Jakarta extract. It is not here, so that screen reports routing unavailable
  instead of drawing a route nobody computed. Run the Docker Compose stack in the
  repository for the complete demonstration.
- **Data resets** when the Space restarts, because nothing persists it.
- **The Space sleeps** after a period without visitors; the first request after that
  takes a minute to wake.

Apache-2.0. The console embeds no third-party code beyond its declared dependencies;
attribution for OSRM, SUMO and OpenStreetMap data is in the repository's NOTICE.
