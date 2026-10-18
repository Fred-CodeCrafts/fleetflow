# TASKS.md

Task list for FleetFlow. `PRD.md` is the source of truth; this file, `DESIGN.md`, and `AGENTS.md` follow it.

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

- [x] 🔴 **T27** Add seeded demand generator (origin-destination pairs and departure times)
  `feat(sim): add seeded demand generator`
  Done when: same seed gives identical demand in a test.
- [x] 🔴 **T28** Add vehicle type and edge occupancy tracking
  `feat(sim): add vehicles and edge occupancy`
- [x] 🔴 **T29** Add edge speed model from occupancy
  `feat(sim): add occupancy-based edge speed model`
- [x] 🔴 **T30** Add simulation step: move vehicles along routes, finish trips
  `feat(sim): advance vehicles each step`
- [x] 🔴 **T31** Add background vehicles with static routes
  `feat(sim): add background vehicles with static routes`
- [x] 🔴 **T32** Add fleet vehicles that request routes and send reports
  `feat(sim): add fleet vehicles using the engine`
- [x] 🔴 **T33** Add rerouting policy (interval plus hysteresis threshold)
  `feat(sim): add rerouting with hysteresis`
- [x] 🔴 **T34** Add trip metrics (average, p95, route changes, saturated edges, utilization std dev)
  `feat(sim): collect trip and congestion metrics`
- [x] 🟡 **T34a** Add time lost versus free-flow time and fleet vs. background trip times (US-6 fairness check)
  `feat(sim): add time lost and fairness metrics`
  Done when: both numbers reach the metrics endpoint and are labelled as a fairness check, not a headline result.
- [x] 🔴 **T35** Run baseline and FleetFlow on the same seed in tests
  `test(sim): compare baseline and dynamic runs on same seed`
  Done when: both runs finish and produce metrics. (Whether FleetFlow wins comes later, in tuning.)

## Day 5 (Wed 21 Oct): API and streaming

- [x] 🔴 **T36** Add HTTP server with `GET /healthz`
  `feat(api): add server and health endpoint`
- [x] 🔴 **T36a** Add `GET /readyz`, env-validated config, and graceful shutdown
  `feat(api): add readiness, config validation and graceful shutdown`
  Done when: bad env fails fast with a clear error; `/readyz` reports graph loaded and DB reachable when configured; SIGTERM stops the customizer and simulation and drains requests.
- [x] 🔴 **T36b** Add typed domain errors, request limits, request IDs, timeouts and rate limits
  `feat(api): add typed errors, validation and limits`
  Done when: domain errors map to status codes in one place; oversized/invalid input is rejected at the handler; logs carry a request ID; public endpoints have timeouts and rate limits.
- [x] 🔴 **T36c** Add CI that builds, vets, lints, formats and tests (including `-race`) on every push
  `ci: add build, vet and test workflow`
  Done when: a push runs build, vet, lint, a `gofmt` check and `go test -race ./...`, and fails on any finding (US-8). Done early so it guards every later commit.
- [x] 🔴 **T36d** Guard the routing core boundary with a test
  `test: guard the routing core boundary`
  Done when: a test fails if `graph`, `weights` or `engine` import an HTTP or database package, so the hot path cannot reach the database by accident (US-5, US-12).
- [x] 🔴 **T37** Add `POST /api/reports`
  `feat(api): add report endpoint`
  Done when: reports write to the weight store only and never touch PostgreSQL.
- [x] 🔴 **T38** Add `POST /api/route`
  `feat(api): add route endpoint`
  Done when: a query returns the ordered edges and cost from the current weights, and an unreachable destination returns a clear error (US-5). Hot path reads the engine snapshot only and never touches PostgreSQL.
- [x] 🔴 **T39** Add fleet records in PostgreSQL: vehicles, drivers, trips (handler → service → repository per entity + `internal/store` migrations + API)
  `feat(fleet): add vehicles, drivers and trips with persistence`
  Done when: every entity follows handler → service → repository with the interface in the service package and `postgres/` + `memory/` behind it; vehicle create/list with validation, driver create and assign/unassign with one-vehicle enforcement, trip create/assign with availability rules in one transaction; records survive a server restart; concurrent double-assign is rejected; schema changes come from versioned migrations; pooling and indexes on filtered FK/status columns; unit tests use `memory/`, integration tests use real Postgres.
- [x] 🔴 **T39a** Drive trip and vehicle status from the run (planned to in progress to completed)
  `feat(fleet): update trip and vehicle status from the simulation`
  Done when: assigning a trip marks the vehicle on trip, finishing it frees the vehicle, and the trip list shows both transitions.
- [x] 🟡 **T39b** Save and list finished comparison runs (US-9)
  `feat(fleet): save and list comparison runs`
  Done when: a finished run is stored with its seed and metrics and is still viewable after a server restart.
- [x] 🔴 **T40** Add API key auth on `POST /api/reports` and `POST /api/route` (`internal/auth`)
  `feat(auth): require api key on reports and route`
  Done when: missing or wrong key returns 401 and does nothing, keys are stored hashed and never logged, and the README shows an example request for each endpoint.
- [ ] 🔴 **T40a** Send simulated reports and route requests through the public API (US-7)
  `feat(sim): report through the public api`
  Done when:
  - FleetFlow-arm fleet vehicles use an HTTP client against `/api/reports`
    and `/api/route`; baseline-arm fleet vehicles and background traffic use
    the in-process static engine (same router code, same free-flow metric).
  - `POST /api/route` accepts optional `current_edges`; the server prices it on
    the same metric snapshot as the new route and returns
    `{edges, cost_s, current_cost_s, metric_version}`. Invalid or
    non-contiguous `current_edges` returns 400 and changes nothing.
  - `FleetService` has one route method taking the held path; `PathCost` is gone.
  - The simulator authenticates with an ephemeral in-memory key minted at
    startup (hashed, never logged, never persisted), through the real
    middleware. No auth bypass.
  - A second start while a run is in progress returns 409, in every mode. The
    run replaces nothing and stops nothing.
  - Simulation is opt-in: `-demo` serves `/api/sim/start` and `/api/sim/reset`,
    and without it both are 404. A run resets the engine's weight state at
    start, which is safe because `-demo` asserts nothing live is feeding it.
    A saved run records that it was a demo run.
  - Simulated time advances in ticks; each tick blocks until its HTTP calls
    return. An HTTP error fails the run loudly, with no silent fallback.
  - Tests: same seed twice gives identical demand and identical baseline
    results; baseline arm equals a FleetFlow engine with weights disabled;
    both costs in one response carry the same `metric_version`.
  Needs T37 and T40.

  Already shipped, so rebuild none of it: `internal/sim/httpfleet.go` (the client,
  attached by `WithFleetService`), `current_edges` / `current_cost_s` /
  `metric_version` on the route response, the held-path route method with
  `PathCost` gone, synchronous reports, and a test that simulated traffic meets the
  API policy. Still owed above: the ephemeral key (today `SIM_API_KEY`, a persisted
  config secret, plus a rate-limit exemption in `WithInternalKey` — still
  authenticated, so a secret rather than a hole), the 409 (a second start swaps the
  running one out), the weight reset (not done: the state is not reset, so a run
  inherits whatever the engine already held), and `-demo`.

  `POST /api/sim/start` and `POST /api/sim/reset` stay public, which is its own
  decision and not part of this task: the dashboard is a static page with no
  credential, so a key would have to ship in it and protect nothing. Reset is
  therefore still a way for any caller to stop a run. It stays stoppable on
  purpose — the dashboard's stop control posts to it — and the rate limit on the
  public routes covers spam. Operator-only controls, with a read-only dashboard,
  would be a later task.
- [x] 🔴 **T41** Add `POST /api/sim/start` and `POST /api/sim/reset`
  `feat(api): add simulation control endpoints`
  Done when: both routes are registered and answer `POST` only; a start decodes
  its body, fills in the demo defaults, refuses out-of-range values with a named
  400, builds both arms and answers `202` with the run's status; a reset answers
  `204` whether or not a run was running, and clears one that was; a start with
  no engine configured answers `503`.
- [x] 🔴 **T42** Add `GET /api/metrics`
  `feat(api): add metrics endpoint`
- [x] 🔴 **T42a** Measure and report route query latency (p50, p99) (US-6)
  `feat(engine): measure route query latency`
  Done when: a query records its duration without taking a lock or blocking, and the metrics endpoint reports p50 and p99 next to the customization time.
- [x] 🔴 **T43** Add `GET /api/stream` (Server-Sent Events) for positions, congestion and metrics
  `feat(api): stream simulation state over sse`
  Done when: each vehicle is labeled simulated or live.
- [x] 🔴 **T44** Run simulation only while a stream client is connected
  `feat(api): tie simulation loop to open stream`
  Done when: closing the browser tab stops the simulation (needed for default Cloud Run CPU behavior).
- [x] 🟡 **T45** Add API handler tests for bad input (including bad reports and bad keys)
  `test(api): cover invalid requests`
- [x] 🟡 **T46** Document the reports format and reject invalid/out-of-range values (US-14, should)
  `docs(api): document reports format and validation`
  Done when: format is documented, bad values get a clear error, and the dashboard labels simulated vs. live.

## Day 6 (Thu 22 Oct): Dashboard

- [x] 🔴 **T47** Add `web/` with static page, embedded via `go:embed`
  `feat(web): serve embedded dashboard page`
- [x] 🔴 **T48** Add fleet views: vehicles, drivers, trips, trip history and saved runs
  `feat(web): add fleet management views`
  Done when: dispatcher can manage vehicles/drivers/trips and review history from the dashboard.
- [x] 🔴 **T49** Draw road graph on canvas
  `feat(web): draw road network on canvas`
- [x] 🔴 **T50** Connect to SSE stream and draw vehicles
  `feat(web): render vehicles from stream`
- [x] 🔴 **T51** Color edges by congestion
  `feat(web): color roads by congestion`
- [x] 🔴 **T52** Show two panels side by side (Baseline, FleetFlow)
  `feat(web): add side-by-side comparison panels`
- [x] 🔴 **T53** Add metrics strip (average trip time, p95, saturated %, route changes, query latency)
  `feat(web): show comparison metrics`
  Done when: the strip states how capacity is derived (lanes and length), so the saturation figure is not a black box (US-6).
- [x] 🔴 **T54** Add controls: start, reset, seed, vehicles, fleet share
  `feat(web): add simulation controls`
  Done when: start loads a ready-made demo scenario, so one click shows a run (US-7).
- [x] 🔴 **T55** Auto-reconnect on stream drop, label each vehicle simulated or live
  `feat(web): reconnect stream automatically`
- [ ] 🟡 **T56** Mobile-friendly layout and basic styling pass
  `style(web): improve layout and styling`

## Day 7 (Fri 23 Oct): Deploy and tune

- [ ] 🔴 **T57** Add multi-stage `Dockerfile` that listens on `$PORT`
  `build: add dockerfile`
  Done when: `docker run -p 8080:8080` serves the dashboard locally.
- [ ] 🔴 **T58** Deploy to **Cloud Run** (`max-instances=1`, `min-instances=1`) with PostgreSQL connected
  `docs: add cloud run deployment steps`
  Done when: the public URL loads, a demo run completes, and fleet records survive a restart. Put the URL in the README. Record the Postgres hosting choice and cost.
- [ ] 🟡 **T59** Add `fly.toml` and test a Fly preview deploy
  `build: add fly.io config for preview deploys`
- [x] 🔴 **T60** Tune parameters (alpha, beta, gamma, lambda, delta, hold, threshold) until the demo scenario shows a clear, honest difference
  `chore(weights): tune default parameters from experiments`
  Done when: results are recorded in `docs/results.md` with seeds, including any cases where FleetFlow does not win.
- [x] 🟡 **T61** Add multi-seed batch script that prints mean and spread
  `feat(scripts): add multi-seed comparison script`
- [x] 🟡 **T62** Test several fleet shares (for example 25%, 50%, 100%)
  `docs: record results across fleet shares`
- [ ] 🟡 **T63** Add `NOTICE` with attributions (OpenStreetMap data if used, libraries)
  `docs: add notice with third-party attributions`

## Day 8 (Sat 24 Oct): Ship

- [ ] 🔴 **T64** Final README: what it is, screenshot or GIF, run locally, deploy, limitations, license, attribution, API key example requests
  `docs: finalize readme`
  Done when: the README calls the project "production-shaped architecture, MVP feature set" and never claims it is production-ready (US-8).
- [ ] 🔴 **T65** Verify a fresh clone builds and runs from the README steps only
  `docs: fix setup steps found during clean-clone check`
- [ ] 🔴 **T66** Run `go vet`, `gofmt`, `go test -race ./...` and fix findings
  `chore: fix lint and race findings`
- [ ] 🔴 **T67** Tag release `v0.1.0`
  `chore: release v0.1.0`
- [ ] 🔴 **T68** Confirm deployed Cloud Run link works from another network or device

## 25 to 27 Oct: Pitch and submission

- [ ] 🔴 **T69** Write the project summary (500 words maximum)
- [ ] 🔴 **T70** Record the demo video (3 minutes maximum): problem, side-by-side run, result, how it works
- [ ] 🔴 **T71** Build the presentation deck (PDF plus PPT/Keynote backup on a flash drive)
- [ ] 🔴 **T72** Record the backup presentation video (15 minutes maximum): technology, business model, future plan
- [ ] 🔴 **T73** Prepare for Q&A (see list below) and rehearse the live demo with a backup device

---

## Stretch (only if everything above is done)

- [ ] 🟢 **S1** Post-sprint roadmap items from PRD section 9 (in order): real-fleet pilot, driver GPS web app, map matching, organizations and roles + tenant separation, key rotation/revocation, persisted congestion state + multi-instance plan, CCH router, Fleetbase adapter, backups + runbook, retention/consent
- [ ] 🟢 **S2** OSM-based Jakarta extract replacing the generated grid
- [ ] 🟢 **S3** ADK agent that suggests parameter settings
- [ ] 🟢 **S4** Optional SUMO adapter

## Cut order if time runs short

1. S1 to S4 (all stretch work)
2. T56 (dashboard polish)
3. T59 (Fly preview)
4. T61 and T62 (batch script and fleet-share sweep)
5. T34a and T39b (fairness metrics and saved runs; both should-have)
6. T46 (US-14 reports docs; demote to could if tight)

**Never cut:** fleet records in Postgres (T39, T39a, T48), API keys (T40), simulated traffic on the public API (T40a), route query latency (T42a), the side-by-side dashboard (T52, T53), the Cloud Run deployment (T58), and the honest results record (T60).

---

## Found along the way

Findings that are not tasks in the plan, kept in writing so they cannot evaporate.
A finding that is a bug in shipped behaviour does not belong here: it is fixed at
once, with a test that fails before the fix and passes after, and it lives in the
history. What stays below is what is still owed.

- **The database layer is never run by automation.** Every `postgres` test skips
  unless `TEST_DATABASE_URL` is set, and CI has no database, so `go test -race ./...`
  passes without executing a single Postgres store or a single migration. Checked by
  hand on 2026-10-18 against the dev container (`127.0.0.1:55432`): all four suites
  pass. Remedy: a Postgres service in CI, the migrations applied to it, and the
  database tests run in their own step.
- **That suite is only correct with `-p 1`, or one database per package.** The
  packages share one schema and truncate it between tests, so run in parallel they
  erase each other's rows. The failure looks like a bug in the store rather than in
  the harness: running four of them in one `go test` invocation failed three on
  duplicate keys and leftover rows, and passed unchanged with `-p 1`. It is
  documented in `internal/store/storetest`, which is not where somebody running the
  suite will look. Remedy: hold a shared advisory lock for the length of a test, or
  make CI the only place these run.
- **The demo configuration cannot congest, so the arms tie -- but a scenario that can is
  now measured.** Both arms come out identical at every density, and the cause is
  arithmetic rather than routing. Capacity is `lanes x length_m / 7.5`, so a 450 m
  two-lane edge holds 120 vehicles and the whole graph holds 16,800. Even at the API's
  maximum of 5,000 vehicles a run peaks near 1,000 on the network, and `EdgeSpeed` is BPR
  with a fourth power, so nothing moves until occupancy approaches capacity: measured
  mid-run at 5,000 vehicles, `cost_s / free_flow_s` is exactly 1 on every edge,
  `route_changes` is 0 for both arms, and time lost is 0.00066 s per trip.

  The lever is the peak, not the vehicle count: what congests a road is how many trips
  overlap, so shortening the network alone does not help -- shorter edges make shorter
  trips and the concurrent count falls with them. Measured on a capacity-matched network
  (150 m blocks, one lane, 20 vehicles per edge) with 2,000 vehicles and a 120 s
  departure window instead of the default 600 s, the difference stops being subtle: the
  baseline averages 189.1 s per trip with a p95 of 2,257 s, and FleetFlow 42.5 s with a
  p95 of 84.5 s, off 37 reroutes. That is this project's own claim demonstrated -- static
  shortest paths jam and dynamic ones spread -- and it is a scenario choice, not a model
  change. T60 did both halves: the demo network and departure window were resized to that
  scenario (DESIGN 3.1, `docs/results.md`), and the uniform default's tie is recorded there
  as a case where FleetFlow does not win, so the record does not rest on the scenario being
  the only one measured. Raising the BPR coefficient instead would have made the demo look
  good for the wrong reason. `internal/sim`'s demo-scenario test fails if the adopted
  scenario ever stops congesting.
- **The congestion columns of a finished run are always zero.** They are a snapshot of
  the network at the moment `Collect` runs, and a run that has settled has no vehicles
  left on it, so `saturated_edges_pct` and `utilization_std_dev` read 0.0000 in every row
  of the comparison CSV -- including the run above, whose baseline trip is five times
  slower than free flow. They mean something mid-run, which is what `GET /api/metrics`
  and the dashboard show, but not in a record. `docs/results.md` therefore says which
  figures are end-of-run and does not use them at all. Keeping the peak during the run -- a
  running maximum in the simulator -- is still the better answer if a congestion figure is
  ever wanted in the record, and it is a small one.
- **The baseline panel stays green while its own traffic jams.** The stream carries one
  engine's metric, and the baseline's weights never move, so its panel is drawn at free
  flow by definition while its vehicles queue behind each other: 0.83% of edges at capacity
  mid-run on the demo scenario, against 0 for FleetFlow. The jam is in the metrics strip,
  in the route-change count and in how slowly its dots move, but not in the colour of the
  map a viewer is looking at. Streaming each arm's occupancy would put it there, at the
  cost of a contract change (DESIGN 3.5, `docs/results.md`). Worth deciding when the
  dashboard is next opened in a browser rather than reasoned about.
- **A saved run can be partial, and nothing in the record says so.** The save captures the
  run as it stands, so a run saved mid-flight stores the metrics of the trips finished so
  far. The figures count those trips, so a partial record does not read as a complete one,
  but a reader comparing two saved runs cannot tell whether they cover the same amount of
  the run. Refusing to save an unfinished run instead would tie the endpoint to a clock
  that parks while nobody is watching (T44), which is exactly when a run is unfinished and
  worth keeping. The dashboard now says which it is for the one case that could mislead --
  an arm with no finished trips shows an em dash, not `0.0 s` (fixed in the T48 branch) --
  but a run that finished half its trips still reads like a whole one. A field in the
  stored metrics would say it for every reader.
- **The fleet views are checked in a browser, but only against the in-memory server.** T48
  was verified by driving headless Chrome over CDP: view switching, each create form,
  driver assignment, a trip walked to completed and into history, a saved run listed, and
  the `409 no_run` refusal. That run of the check used the dev Postgres; the committed
  state has no such test, so the next change to `app.js` is unguarded again. What the
  page's requests are (paths, bodies, statuses) is pinned only by the API tests, not by
  anything that runs the JavaScript.
- **The dashboard's JavaScript is verified by hand, not by a test in the repo.** The
  page is exercised against the running binary (all three event kinds arrive, both arms
  are labelled, 120 edges per congestion frame) and its drawing decisions were checked
  outside the repo with a stub DOM: each panel receives its own arm plus live traffic,
  the FleetFlow panel colours all three bands its legend names, and the baseline panel
  never leaves free flow. What no check covers is how it looks. Adding a JS test runner
  for one page would cost more than it protects, so this stays a manual step and is
  recorded as one rather than described as covered.

---

## Questions to prepare for

- Why not just use Fleetbase or Google Maps? (They decide who goes where or optimize each vehicle separately. FleetFlow coordinates which roads the fleet uses.)
- What is the evidence? (Simulation only, across several seeds, including where it does not help.)
- What happens when avoided roads stop reporting? (Hold time, slow decay and hysteresis, and the route-change metric to detect it.)
- What would real deployment need? (Real GPS ingestion via the reports API, validated capacity data, PostgreSQL, UU PDP review before any pilot.)
- How do outside systems authenticate? (API key on reports and route; 401 on missing or wrong key; keys stored hashed.)
- Is the project new? (Yes: new repo with a visible history starting after 11 Oct. The earlier thesis prototype is background only and nothing was copied.)

---

## Rules that apply to every task

- Commit in small steps and never rewrite history (see `AGENTS.md`).
- Do not copy code from `../sumo-routing/` or `../fleetbase/`.
- Keep the build green: `go build ./...` and `go test ./...` before each commit.
