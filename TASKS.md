# TASKS.md

Task list for FleetFlow, matching `DESIGN.md` and `AGENTS.md`.

**How to use this file**
- One task = one commit (or a few tiny ones). Do them in order.
- Tick the box in the same commit that completes the task.
- The suggested commit message is in `code`. Adjust it if the change differs.
- "Done when" is the check before you tick the box.
- Deadline: **24 Oct** (deployed). Showcase: **27 Oct**.

Legend: 🔴 must-have · 🟡 should-have · 🟢 could-have

---

## Day 1 (Sat 17 Oct): Repo and graph

- [x] 🔴 **T01** Init repo, add `LICENSE` (Apache-2.0 or MIT)
  `chore: initialize repository with license`
  Done when: `git log` shows this as the first commit, dated on or after 11 Oct, no other code in it.
- [x] 🔴 **T02** Add `AGENTS.md`, `DESIGN.md`, `TASKS.md`
  `docs: add agent rules, design and task list`
- [x] 🔴 **T03** Add README skeleton (what it is, status, planned run steps, background note about thesis prototype)
  `docs: add README skeleton`
  Done when: README says the earlier prototype is background only and no code was copied.
- [x] 🔴 **T04** Add `.gitignore` and `.env.example`
  `chore: add gitignore and env example`
- [x] 🔴 **T05** `go mod init`, add `cmd/fleetflow/main.go` that prints a startup line
  `chore: initialize go module and main entrypoint`
  Done when: `go build ./...` passes.
- [x] 🔴 **T06** Add `internal/graph` types (`Node`, `Edge`, `Graph`)
  `feat(graph): add node, edge and graph types`
- [x] 🔴 **T07** Add JSON loader with validation (missing nodes, bad lengths)
  `feat(graph): load graph from json with validation`
  Done when: loader test covers a valid file and two invalid ones.
- [x] 🔴 **T08** Compute derived values (`free_flow_s`, `capacity`) on load
  `feat(graph): derive free-flow time and capacity per edge`
- [x] 🔴 **T09** Add a tiny hand-made test graph in `data/` for tests
  `test(graph): add small test graph fixture`
- [x] 🔴 **T10** Add generated grid-city graph script (fallback map)
  `feat(scripts): generate grid city graph`
  Done when: running the script writes `data/graph.json` that loads without errors.

## Day 2 (Sun 18 Oct): Weight store

- [x] 🔴 **T11** Add `EdgeState` struct with per-edge `RWMutex`
  `feat(weights): add per-edge state struct`
- [x] 🔴 **T12** Add congestion measure `rho = clamp(1 - v/v_free, 0, 1)` with tests
  `feat(weights): add congestion measure from observed speed`
- [x] 🔴 **T13** Add moving-average update on report, with tests
  `feat(weights): smooth congestion with moving average`
- [x] 🔴 **T14** Add cost function `free_flow * (1 + alpha * rho^beta)`, with tests
  `feat(weights): add edge cost function`
- [x] 🔴 **T15** Add decay with hold time, with tests (no report → decays; recent report → does not)
  `feat(weights): decay stale edges toward free-flow`
- [x] 🔴 **T16** Add parameter struct with defaults and validation
  `feat(weights): add tunable parameters with defaults`
- [x] 🔴 **T17** Add concurrent report test run with `-race`
  `test(weights): add race test for concurrent reports`
  Done when: `go test -race ./internal/weights` passes.

## Day 3 (Mon 19 Oct): Engine and router

- [x] 🔴 **T18** Add immutable `Metric` type (costs slice plus version)
  `feat(engine): add immutable metric snapshot`
- [x] 🔴 **T19** Add Dijkstra with binary heap over a given metric, with tests on the fixture graph
  `feat(engine): add dijkstra router`
  Done when: known shortest paths on the fixture graph match expected results.
- [x] 🔴 **T20** Add `Router` interface so another algorithm can replace Dijkstra
  `refactor(engine): put router behind an interface`
- [x] 🔴 **T21** Add customizer: read all edge costs into a fresh metric
  `feat(engine): build metric snapshot from edge state`
- [x] 🔴 **T22** Publish metric with `atomic.Pointer` and load once per query
  `feat(engine): publish active metric with atomic pointer`
- [x] 🔴 **T23** Add periodic customization loop with clean shutdown
  `feat(engine): run customization on an interval`
- [x] 🔴 **T24** Add baseline engine (weights disabled, fixed free-flow metric)
  `feat(engine): add static baseline mode`
- [x] 🔴 **T25** Add race test: queries during customization and reports
  `test(engine): add race test for queries during customization`
  Done when: `go test -race ./...` passes.
- [x] 🟡 **T26** Record customization time and metric age
  `feat(engine): track customization time and metric age`

## Day 4 (Tue 20 Oct): Simulator

- [ ] 🔴 **T27** Add seeded demand generator (origin-destination pairs and departure times)
  `feat(sim): add seeded demand generator`
  Done when: same seed gives identical demand in a test.
- [ ] 🔴 **T28** Add vehicle type and edge occupancy tracking
  `feat(sim): add vehicles and edge occupancy`
- [ ] 🔴 **T29** Add edge speed model from occupancy
  `feat(sim): add occupancy-based edge speed model`
- [ ] 🔴 **T30** Add simulation step: move vehicles along routes, finish trips
  `feat(sim): advance vehicles each step`
- [ ] 🔴 **T31** Add background vehicles with static routes
  `feat(sim): add background vehicles with static routes`
- [ ] 🔴 **T32** Add fleet vehicles that request routes and send reports
  `feat(sim): add fleet vehicles using the engine`
- [ ] 🔴 **T33** Add rerouting policy (interval plus hysteresis threshold)
  `feat(sim): add rerouting with hysteresis`
- [ ] 🔴 **T34** Add trip metrics (average, p95, route changes, saturated edges, utilization std dev)
  `feat(sim): collect trip and congestion metrics`
- [ ] 🔴 **T35** Run baseline and FleetFlow on the same seed in tests
  `test(sim): compare baseline and dynamic runs on same seed`
  Done when: both runs finish and produce metrics. (Whether FleetFlow wins comes later, in tuning.)

## Day 5 (Wed 21 Oct): API and streaming

- [ ] 🔴 **T36** Add HTTP server with `GET /healthz`
  `feat(api): add server and health endpoint`
- [ ] 🔴 **T37** Add `POST /api/reports`
  `feat(api): add report endpoint`
- [ ] 🔴 **T38** Add `POST /api/route`
  `feat(api): add route endpoint`
- [ ] 🔴 **T39** Add `POST /api/sim/start` and `POST /api/sim/reset`
  `feat(api): add simulation control endpoints`
- [ ] 🔴 **T40** Add `GET /api/metrics`
  `feat(api): add metrics endpoint`
- [ ] 🔴 **T41** Add `GET /api/stream` (Server-Sent Events) for positions, congestion and metrics
  `feat(api): stream simulation state over sse`
- [ ] 🔴 **T42** Run simulation only while a stream client is connected
  `feat(api): tie simulation loop to open stream`
  Done when: closing the browser tab stops the simulation (needed for default Cloud Run CPU behavior).
- [ ] 🟡 **T43** Add API handler tests for bad input
  `test(api): cover invalid requests`

## Day 6 (Thu 22 Oct): Dashboard

- [ ] 🔴 **T44** Add `web/` with static page, embedded via `go:embed`
  `feat(web): serve embedded dashboard page`
- [ ] 🔴 **T45** Draw road graph on canvas
  `feat(web): draw road network on canvas`
- [ ] 🔴 **T46** Connect to SSE stream and draw vehicles
  `feat(web): render vehicles from stream`
- [ ] 🔴 **T47** Color edges by congestion
  `feat(web): color roads by congestion`
- [ ] 🔴 **T48** Show two panels side by side (Baseline, FleetFlow)
  `feat(web): add side-by-side comparison panels`
- [ ] 🔴 **T49** Add metrics strip (average trip time, p95, saturated %, route changes)
  `feat(web): show comparison metrics`
- [ ] 🔴 **T50** Add controls: start, reset, seed, vehicles, fleet share
  `feat(web): add simulation controls`
- [ ] 🔴 **T51** Auto-reconnect on stream drop
  `feat(web): reconnect stream automatically`
- [ ] 🟡 **T52** Mobile-friendly layout and basic styling pass
  `style(web): improve layout and styling`

## Day 7 (Fri 23 Oct): Deploy and tune

- [ ] 🔴 **T53** Add multi-stage `Dockerfile` that listens on `$PORT`
  `build: add dockerfile`
  Done when: `docker run -p 8080:8080` serves the dashboard locally.
- [ ] 🔴 **T54** Deploy to **Cloud Run** (`max-instances=1`, `min-instances=1`)
  `docs: add cloud run deployment steps`
  Done when: the public URL loads and a demo run completes. Put the URL in the README.
- [ ] 🟡 **T55** Add `fly.toml` and test a Fly preview deploy
  `build: add fly.io config for preview deploys`
- [ ] 🔴 **T56** Tune parameters (alpha, beta, gamma, lambda, delta, hold, threshold) until the demo scenario shows a clear, honest difference
  `chore(weights): tune default parameters from experiments`
  Done when: results are recorded in `docs/results.md` with seeds, including any cases where FleetFlow does not win.
- [ ] 🟡 **T57** Add multi-seed batch script that prints mean and spread
  `feat(scripts): add multi-seed comparison script`
- [ ] 🟡 **T58** Test several fleet shares (for example 25%, 50%, 100%)
  `docs: record results across fleet shares`
- [ ] 🟡 **T59** Add `NOTICE` with attributions (OpenStreetMap data if used, libraries)
  `docs: add notice with third-party attributions`

## Day 8 (Sat 24 Oct): Ship

- [ ] 🔴 **T60** Final README: what it is, screenshot or GIF, run locally, deploy, limitations, license, attribution
  `docs: finalize readme`
- [ ] 🔴 **T61** Verify a fresh clone builds and runs from the README steps only
  `docs: fix setup steps found during clean-clone check`
- [ ] 🔴 **T62** Run `go vet`, `gofmt`, `go test -race ./...` and fix findings
  `chore: fix lint and race findings`
- [ ] 🔴 **T63** Tag release `v0.1.0`
  `chore: release v0.1.0`
- [ ] 🔴 **T64** Confirm deployed Cloud Run link works from another network or device

## 25 to 27 Oct: Pitch and submission

- [ ] 🔴 **T65** Write the project summary (500 words maximum)
- [ ] 🔴 **T66** Record the demo video (3 minutes maximum): problem, side-by-side run, result, how it works
- [ ] 🔴 **T67** Build the presentation deck (PDF plus PPT/Keynote backup on a flash drive)
- [ ] 🔴 **T68** Record the backup presentation video (15 minutes maximum): technology, business model, future plan
- [ ] 🔴 **T69** Prepare for Q&A (see list below) and rehearse the live demo with a backup device

---

## Stretch (only if everything above is done)

- [ ] 🟢 **S1** Fleetbase adapter: read orders and vehicles via its API, return routes from FleetFlow (verify their endpoints in the docs first)
  `feat(adapter): add fleetbase integration`
- [ ] 🟢 **S2** Customizable Contraction Hierarchies router behind the existing `Router` interface
- [ ] 🟢 **S3** OSM-based Jakarta extract replacing the generated grid
- [ ] 🟢 **S4** ADK agent that suggests parameter settings
- [ ] 🟢 **S5** Optional SUMO adapter

## Cut order if time runs short

1. S1 to S5 (all stretch work)
2. T57 and T58 (batch script and fleet-share sweep)
3. T52 (styling pass)
4. T26 (metric age tracking)
5. T55 (Fly preview)

**Never cut:** the side-by-side dashboard (T48, T49), the Cloud Run deployment (T54), and the honest results record (T56).

---

## Questions to prepare for

- Why not just use Fleetbase or Google Maps? (They decide who goes where or optimize each vehicle separately. FleetFlow coordinates which roads the fleet uses.)
- What is the evidence? (Simulation only, across several seeds, including where it does not help.)
- What happens when avoided roads stop reporting? (Hold time, slow decay and hysteresis, and the route-change metric to detect it.)
- What would real deployment need? (Real GPS ingestion, validated capacity data, persistent state, more than one instance.)
- Is the project new? (Yes: new repo with a visible history starting after 11 Oct. The earlier thesis prototype is background only and nothing was copied.)

---

## Rules that apply to every task

- Commit in small steps and never rewrite history (see `AGENTS.md`).
- Do not copy code from `../sumo-routing/` or `../fleetbase/`.
- Keep the build green: `go build ./...` and `go test ./...` before each commit.
