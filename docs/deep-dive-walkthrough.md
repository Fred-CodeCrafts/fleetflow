# FleetFlow Deep-Dive Walkthrough

**Target:** 12–14 minutes · **Audience:** GDE, GDG, and industry judges

## Core message

FleetFlow is a Jakarta fleet-operations console built around a trust rule: each
number should identify the system or model that produced it. A route comes from
OSRM or fails; each scenario names SUMO or the synthetic runner; real GPS stays
separate from simulation; and emissions name their assumptions.

## Run of show

| Time | Segment | Show |
|---|---|---|
| 0:00–0:35 | Hook | Existing zoom-edited UI montage with voiceover |
| 0:35–1:45 | Origin | Presenter or a simple title card |
| 1:45–3:15 | Architecture | One diagram and short code cutaways |
| 3:15–5:15 | Scenario | Seeded run flow and its `runner` value |
| 5:15–6:45 | Guardrails | GPS, routing errors, emission labels |
| 6:45–9:45 | Live demo | Route, seeded run, trips, emissions |
| 9:45–11:30 | Routing Lab | Experiment, results, method notes |
| 11:30–12:40 | Profiling and workflow | Allocation improvement and honest token-trial summary |
| 12:40–13:30 | Close | Limits and the core message |

## Narration and shots

### 0:00–0:35 — Hook

**Show:** The existing zoom-edited UI clip. Keep key labels visible long enough to
read; explain the highlights with voiceover instead of expecting viewers to infer
their meaning.

**Say:** “FleetFlow is a fleet-operations console for Jakarta. It connects routes,
seeded delivery scenarios, fleet records, and an emissions ledger. The central
engineering question is: can someone tell where each result came from?”

### 0:35–1:45 — Why I built it

**Say:** “My thesis proposal is still ongoing. Dynamic edge weights are not a new,
fashionable technique; they are a fundamental routing idea. I wanted to show how
that idea can be implemented and why fundamentals still matter. For DevFest, that
grew into FleetFlow: an operations product with a small, reproducible lab for
exploring the routing idea.”

“The lab demonstrates a mechanism. It is not the completed thesis and it is not a
model of Jakarta traffic.”

### 1:45–3:15 — Architecture

**Show:**

```text
React + TypeScript console ──HTTP──> Go API ──> PostgreSQL
                                       ├──────> OSRM route service
                                       └──────> SUMO or synthetic scenario runner
MapLibre ──> OpenFreeMap tiles
Real GPS ──authenticated API──> separate GPS data path
```

**Say:** “The console is React and TypeScript. The Go API owns validation, fleet
and trip operations, scenario lifecycle, and persistence. PostgreSQL stores
operational records. OSRM supplies driving-route estimates; FleetFlow does not
implement a fallback shortest-path router. MapLibre renders the map.”

“The Routing Lab is a separate experiment, not the route planner.”

**Code cutaway:** Show the wiring in `cmd/fleetflow/main.go`, then one API or
service boundary. Keep each cutaway to one idea.

### 3:15–5:15 — How a scenario becomes records

**Show:** Scenario page, seed, completion, and `runner` in the UI or API response.
Use short cutaways to `internal/simulation` and `internal/simulation/sumo`.

**Say:** “A scenario starts from explicit inputs, including a seed. When SUMO and
a network are configured, FleetFlow can invoke SUMO and parse its outputs. Without
that runtime, the built-in generator can produce a scenario instead. Each run
records which engine actually ran.”

“Completed runs can register vehicles and dispatch trips into fleet records.
Repeating a seeded input makes the scenario reproducible; it does not make it real
traffic. Generated package events are scenario output, not live scans.”

### 5:15–6:45 — Guardrails

**Show:** An empty Live Map, the route provider, `/readyz`, and the emissions
model note.

**Say:** “The map does not invent live vehicle positions. Authenticated GPS reports
stay distinct from simulated positions. Routing is OSRM or an explicit error.
Readiness reports whether dependencies such as routing and the database are
available. Operator writes can require an API key, and readiness reports whether
they are protected.”

“Emissions are model estimates, not measured fuel use or a certified inventory.
The vehicle class and assumptions are shown so a reviewer can check what the
number means.”

**Do not claim:** every run used SUMO, demo-mode writes are secure, or emissions
are measured.

### 6:45–9:45 — Live demo

Follow [DEMO.md](./DEMO.md) for setup and exact interactions. Check readiness and
the actual runner before recording; read the values on screen rather than relying
on stale counts.

1. **Route Planner (6:45–7:35):** Find a route between two hubs. Say: “The geometry
   and estimate came from OSRM. If OSRM is unavailable, the correct behavior is an
   error, not a made-up line.”
2. **Scenario (7:35–8:25):** Start a seeded run and read the runner aloud. Say:
   “This run is marked [actual value], so we know which engine produced it.”
3. **Trips (8:25–9:05):** Open a trip detail. Say: “The route and delivery events
   are inspectable. These are scenario events, not live package scans.”
4. **Sustainability (9:05–9:45):** Show diesel and electric vehicle rows with the
   model label. Say: “Zero means zero tailpipe CO₂ under this model; it does not
   include electricity generation.”

If a dependency is down, acknowledge it and move on; do not narrate a result that
did not appear.

### 9:45–11:30 — Routing Lab

**Show:** Settings → Routing Lab: adoption, seed count, per-seed result, and method
notes.

**Say:** “This is the thesis-inspired experiment, not the OSRM route planner. It
routes identical seeded demand through a static free-flow policy and a dynamic
policy that learns congestion from simulated vehicle reports. The generated grid
makes the experiment reproducible.”

“The result describes this model only. It is not measured Jakarta traffic or the
research in my proposal: there is no CCH, TraCI loop, or LuST scenario here. The
method notes sit beside the numbers so the boundary stays visible.”

Do not present the improvement percentage as an expected real-world gain.

### 11:30–12:40 — Profiling and development lesson

**Say:** “I profiled the Routing Lab's default five-seed sweep instead of guessing
at a backend optimization. It took about 37 milliseconds and allocated roughly
14 megabytes per sweep. Most of the allocation came from building a new random
generator for every informed simulated vehicle.”

“I changed it to reseed and reuse one generator, and reused the graph router. The
same benchmark then allocated about 4.5 megabytes, roughly 68% less. Runtime was
in the same range, so I would claim reduced allocation pressure, not a major speed
improvement. The result remains deterministic.”

“I also tracked one agent-orchestration trial: 708,044 worker tokens total, with
55,939 spent on merge and conflict resolution. The estimated saving versus the
lead reading the diff was only about 2,600 tokens. I would not call that a
token-saving success; the useful lesson was to delegate only bounded work and keep
the lead context short.”

The profile figures are one local benchmark, not production telemetry. The token
figures are one trial recorded in [AGENTS.md](../AGENTS.md), not general benchmarks.

### 12:40–13:30 — Limits and close

**Say:** “FleetFlow is an MVP. Synthetic scenarios are not real traffic; the
Routing Lab is not the thesis or a Jakarta study; emissions are planning
estimates; and deployment depends on external services and configuration.”

“The goal is not to make every number look certain. It is to show where it came
from, what assumptions produced it, and what it cannot tell us.”

## Recording checklist

- Add voiceover or brief captions to the zoom montage; avoid text-heavy overlays.
- Hold on provider, runner, and model labels long enough to read.
- Confirm routing readiness and the actual runner before the live demo.
- Check that the emissions class column is visible at the chosen zoom level.
- Keep the total below 15 minutes; this script targets about 13½ minutes.
- Never imply a failed dependency succeeded.

## Project references

- [Architecture and implementation boundaries](../DESIGN.md)
- [Product scope](../PRD.md)
- [Demo setup and interactions](../DEMO.md)
- [Written summary](./summary.md)
- [Cloud Run notes](../deploy/README.md)
