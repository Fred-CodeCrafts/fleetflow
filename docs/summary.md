# FleetFlow — written summary

**The problem.** A Jakarta delivery fleet answers two questions badly every day: which
vehicle should take this load, and what did the run actually emit? The answers usually
live in a spreadsheet, a chat group, and a fuel receipt. The hard part is knowing which
number came from where. A dashboard that mixes a
measured GPS position with a modelled emission figure is worse than none, because it
looks auditable and is not.

**What is built.** FleetFlow is an operations console built around one rule: a number a
model produced must name the model that produced it. The stack is Go 1.25 with
PostgreSQL and Flyway migrations, a React and Vite console, MapLibre GL over
OpenFreeMap tiles, and OSRM over a Jakarta/Jabodetabek extract of OpenStreetMap. An
operator starts a seeded scenario; the run registers its vehicles, records trips
deduplicated by vehicle, origin and destination, and dispatches them to fleet records
with trip history. Routing is OSRM or a 503 — this application computes no
route itself and invents no fallback. Every stored run carries a `runner` field that
says `"sumo"` when SUMO actually ran and `"synthetic"` when the built-in Go generator
produced it, and the console prints that field instead of implying the simulator always
ran. GPS reports from real devices are authenticated by API key, kept in their own
store, and never blended with simulated positions on a map.

**What is measured.** Two things, and both name their source. Emissions come from one
table in `internal/fleet` — 948 g CO₂/km for a 14-tonne diesel box truck, 569 for a
4.5-tonne truck, 348 for a diesel van, zero at the tailpipe for an electric cargo
motorcycle — with the class printed beside every figure and a test that proves the
arithmetic rather than a lookup. Settings → Routing Lab runs the author's thesis idea at
grid scale: identical seeded demand routed twice on a 6×6 lattice, once on free-flow
times and once on congestion learned from the fleet's own speed reports, over ten seeds.
It improves trip time by 4.6% to 8.4%, mean 6.1%, and 0% adoption ties the baseline
exactly, kept as a control so a reader can see the gain comes from the policy and not
from noise. The grid is built in code, so a clone reproduces the result.

**What is not claimed.** There is no field evidence, no operator, no measured Jakarta
traffic and no fuel bills, so no rupiah figure appears anywhere. The emission factors
are planning assumptions for congested urban delivery work, not a certified inventory
and not HBEFA3. The routing lab describes a model, not the city, and not the research
the proposal describes: no CCH, no TraCI, no LuST scenario. Writes are unauthenticated
by default so the demonstration console can operate; `REQUIRE_OPERATOR_KEY=true` closes
them to API keys and `/readyz` publishes which mode is running. Apache-2.0, tests in the
repository, and every limit written down where a reader can check it.
