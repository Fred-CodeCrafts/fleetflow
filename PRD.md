# FleetFlow: PRD-lite

Road To DevFest Jakarta 2026 / Roro Jonggrang Sprint
Author: Frederick Garner Wibowo
Status: draft v0.1, 17 October 2026

---

## 1. Problem statement (one sentence)

Fleet operators in Jakarta send every vehicle down the same "fastest" roads, so the fleet congests its own routes and trips take longer than they need to, and FleetFlow is an open-source fleet management system that routes vehicles using live, smoothed congestion weights so the fleet spreads out instead of competing for the same roads.

## 2. Who it is for

| User | Needs | Priority |
|---|---|---|
| **Fleet operator / dispatcher** (primary) | Track the fleet, assign trips, and get routes that avoid self-made congestion | Primary |
| **Driver** (secondary) | Follows the route assigned. In the demo, drivers are simulated vehicles | Secondary |
| **Developer / contributor** (open source) | Runs the project locally in a few minutes and understands how it works | Secondary |

Example operator: a delivery company with 30 to 100 vehicles in Jakarta. Used as a story for the pitch, not a validated customer.

## 3. Goals and success measures

| Goal | How we measure it |
|---|---|
| A working, deployed MVP that an operator can use end to end | Live Cloud Run link, demo works from a fresh page load |
| Show that congestion-aware routing helps the fleet | Side-by-side run on the same seed: average trip time, 95th percentile trip time, saturated roads, route changes |
| Be honest about the evidence | Results reported as simulation only, over several seeds, including cases where it does not help |
| Easy for others to run and extend | Fresh clone builds and runs using the README steps only |

No numeric improvement target is promised in advance. We report what the experiments show.

---

## 4. User stories and acceptance criteria

Priority: **M** = must (MVP), **S** = should, **C** = could.

### US-1 (M) Manage vehicles
*As a dispatcher, I want to add and view vehicles and their status, so that I know what is in my fleet.*

Acceptance criteria:
- [ ] I can create a vehicle with a name or plate, type and status (available, on trip, offline).
- [ ] I can see a list of all vehicles with their current status.
- [ ] Data is still there after the page is reloaded and after a server restart (PostgreSQL, see US-12).
- [ ] Invalid input (empty name, unknown status) shows a clear error and saves nothing.

### US-2 (M) Manage drivers
*As a dispatcher, I want to register drivers and assign them to vehicles, so that every trip has someone responsible.*

Acceptance criteria:
- [ ] I can create a driver with a name and contact.
- [ ] I can assign or unassign a driver to a vehicle.
- [ ] A driver cannot be assigned to two vehicles at the same time.

### US-3 (M) Create and assign trips
*As a dispatcher, I want to create a trip (origin, destination) and assign it to a vehicle, so that deliveries get done.*

Acceptance criteria:
- [ ] I can pick an origin and destination on the map or from a list of known places.
- [ ] I can assign the trip to an available vehicle.
- [ ] The trip shows a status: planned, in progress, completed.
- [ ] A vehicle that is offline or already on a trip cannot be assigned a new one.

### US-4 (M) See the live fleet map
*As a dispatcher, I want to see all vehicles moving on a map, so that I know where the fleet is right now.*

Acceptance criteria:
- [ ] Vehicles appear as markers and move as new positions arrive.
- [ ] Roads are colored by congestion level.
- [ ] The map updates without a manual refresh, and reconnects on its own if the connection drops.
- [ ] Positions come from simulated telemetry and the page says so.

### US-5 (M) Get a congestion-aware route
*As a dispatcher, I want a recommended route for each trip that avoids roads the fleet is already clogging, so that trips are faster.*

Acceptance criteria:
- [ ] For a given origin and destination, the system returns an ordered road sequence and an estimated travel time.
- [ ] The route is calculated from the current dynamic weights, not fixed free-flow costs.
- [ ] Congestion per road is smoothed with an exponential moving average over speed reports.
- [ ] Roads with no recent report decay toward free-flow after a hold time.
- [ ] A route query returns without waiting for weight updates (it uses the latest published immutable snapshot, atomically swapped).
- [ ] Hot path stays in memory and out of the database: `POST /reports` writes to the weight store only, `POST /api/route` reads the engine snapshot only. Neither touches PostgreSQL.
- [ ] Routing core (`graph`, `weights`, `engine`) imports nothing from the HTTP or database layers.
- [ ] A static shortest-path baseline mode is available for comparison.
- [ ] If no route exists, the system says so clearly.
- [ ] Public API surface: `POST /reports`, `POST /route`, `GET /metrics` (versioned under `/api/` in this design).

### US-6 (M) Compare against standard routing
*As a dispatcher (and as a judge), I want to see standard shortest-path routing and FleetFlow routing side by side on the same demand, so that I can judge whether it helps.*

Acceptance criteria:
- [ ] Two panels run the same seed and demand: Baseline and FleetFlow.
- [ ] Metrics are shown for both: average trip time, 95th percentile trip time, saturated roads (%), route changes.
- [ ] Road capacity used for the saturation metric is defined and stated (derived from lanes and length).
- [ ] I can set the seed, number of vehicles, and fleet share, then start and reset a run.
- [ ] Re-running the same seed gives the same demand.
- [ ] Route query latency (p50/p99) and weight refresh time are measured and reported.

### US-7 (M) Run a demo scenario without real vehicles
*As a presenter, I want a one-click demo scenario with simulated vehicles sending telemetry, so that the product can be shown anywhere.*

Acceptance criteria:
- [ ] One button starts a ready-made scenario on a Jakarta-style road network.
- [ ] Simulated vehicles report road and speed to the system the same way an external client would.
- [ ] The simulation stops when no one is watching (saves resources on Cloud Run).

### US-8 (M) Deploy and run it easily
*As a contributor, I want to clone, run and deploy FleetFlow quickly, so that I can try or extend it.*

Acceptance criteria:
- [ ] The README has setup steps that work from a fresh clone.
- [ ] The app runs locally with one command or one Docker command.
- [ ] A public Cloud Run link is available and works from another network.
- [ ] Single instance (`max-instances=1`); persistent records live in PostgreSQL while live congestion state stays in memory.
- [ ] All settings come from environment variables, validated at startup; the app refuses to start on bad config.
- [ ] `GET /healthz` (liveness) and `GET /readyz` (readiness: graph loaded, DB reachable if configured) exist.
- [ ] Graceful shutdown: stop the customizer and simulation, drain in-flight requests.
- [ ] CI builds, vets, tests and lints on every push; deploy is repeatable from the README.
- [ ] The repo has an open-source license (Apache-2.0 or MIT, chosen at first commit) and attributions for third-party data (e.g. OpenStreetMap, ODbL).
- [ ] Honest claim: "production-shaped architecture, MVP feature set" — never "production-ready" (no real traffic, backups, or security review yet).

### US-12 (M) Store data reliably
*As a dispatcher, I want vehicles, drivers, trips and saved runs to be stored in a database with rules enforced, so that nothing is lost or contradictory.*

Acceptance criteria:
- [ ] Data is stored in PostgreSQL and survives a server restart.
- [ ] Every entity (vehicles, drivers, trips, runs, API keys) follows handler → service → repository. Consistency matters more than saving a file.
- [ ] Repository interfaces are defined in the service package; `postgres/` and `memory/` implementations sit behind them. Unit tests use `memory/`; production uses `postgres/`.
- [ ] A driver cannot be assigned to two vehicles.
- [ ] A vehicle that is offline or already on a trip cannot be assigned a new trip, even if two requests arrive at the same time (one transaction per assignment plus database constraints).
- [ ] Database changes are made through versioned migrations, not by hand.
- [ ] Connection pooling is configured; indexes exist on the foreign keys and status columns that are filtered by.
- [ ] Hot path stays out of the database: reports and route queries never touch PostgreSQL.

### US-13 (M) Let outside systems use the API
*As a developer, I want to call the reports and route endpoints from my own system using an API key, so that I can use FleetFlow without its dashboard.*

Acceptance criteria:
- [ ] `POST /api/reports` and `POST /api/route` require a valid API key.
- [ ] A missing or wrong key returns 401 and does nothing.
- [ ] API keys are stored hashed, never in plain text, and are never written to logs.
- [ ] Typed domain errors are mapped to status codes in one place; handlers validate input and enforce request size limits.
- [ ] Structured logs carry a request ID; public endpoints have rate limits and timeouts.
- [ ] The README shows an example request for each endpoint.

### US-14 (S) Accept real vehicle reports
*As a fleet owner, I want to send real vehicle positions and speeds to FleetFlow, so that routes reflect my actual fleet.*

Acceptance criteria:
- [ ] The reports endpoint documents its format (vehicle, road or location, speed, timestamp).
- [ ] Reports with invalid or out-of-range values are rejected with a clear error.
- [ ] The dashboard labels each vehicle's data as simulated or live.

### US-9 (S) Trip history and run results
*As a dispatcher, I want to see completed trips and saved comparison runs, so that I can review performance later.*

Acceptance criteria:
- [ ] Completed trips are listed with start, end and duration.
- [ ] A finished comparison run can be saved and viewed again.

### US-10 (S) Stable routing under change
*As a dispatcher, I want routes not to flip back and forth every few seconds, so that drivers are not confused.*

Acceptance criteria:
- [ ] A vehicle switches routes only if the new route is better than the current one by a set margin (hysteresis threshold).
- [ ] Route changes per vehicle are counted and shown.
- [ ] Concurrency correctness is covered by tests, including `go test -race`.

### US-11 (C) Plug into an existing fleet tool
*As a developer, I want a small adapter for an existing platform such as Fleetbase, so that FleetFlow can serve as its routing layer.*

Acceptance criteria:
- [ ] An external client can send reports and request routes through the public API.
- [ ] Exact integration endpoints are verified in the other platform's docs before building.
- [ ] FleetFlow stays a separate service talking to Fleetbase over its REST API or webhooks (adapter, not a fork), so Fleetbase's AGPL-3.0 obligations do not extend to this repo. Fleetbase itself is not deployed to Cloud Run (needs Docker Compose, ~4 GB RAM); integration is shown locally or in a recorded clip.
- [ ] Fleet-Ops route optimization overlap is checked and positioning is congestion-aware coordination that complements fleet tools.

Post-sprint roadmap items (driver app, map matching, organizations, persisted weights, CCH, Fleetbase adapter, real-fleet pilot) live in section 9, not here.

---

## 5. Non-goals (for this sprint)

- Driver mobile or web app for sending GPS (roadmap)
- Converting raw GPS points into road IDs, known as map matching (roadmap)
- Multiple organizations, user roles and billing (roadmap)
- API key rotation and revocation UI, multi-instance state (roadmap; single instance only)
- Billing, invoicing, fuel, maintenance, inventory or warehouse features
- Multi-stop and multi-vehicle optimization (vehicle routing problem)
- Scaling to a whole city (roadmap, needs a faster routing algorithm)
- Saving congestion weights across restarts (roadmap)
- Proving the benefit with a real fleet (roadmap, planned as a pilot)
- Competing with Fleetbase or similar platforms on breadth of features
- Running more than one server instance (state is in memory)
- Backups, runbook beyond a sketch, full security review (roadmap; see section 9)
- Pre-existing code. No code is copied from the thesis prototype or from Fleetbase

Production-shaped, not production-ready: the sprint ships layered code, migrations, hashed API keys, typed errors, health/readiness endpoints, graceful shutdown, env-validated config, and tests in CI. Backups, key rotation/revocation, tenant separation, persisted weights, multi-instance support, retention/consent handling, and security review stay on the roadmap.

## 6. Constraints and assumptions

**Constraints (from the competition playbook)**
- Project must start on or after 11 October 2026, shown by the initial GitHub commit.
- Must be deployed via Cloud Run. MVP only.
- All work must be original. Third-party code and data need proper attribution and license compliance.
- Use of LLMs is permitted.
- Deliverables: source code and README, demo video (max 3 minutes), written summary (max 500 words), final deck, backup presentation video (max 15 minutes), live demo.
- Key dates: building 17 Oct, deploy by 24 Oct, pitch 25 Oct, showcase 27 Oct, winner announcement 1 Nov.
- Judging weights: Impact 20, Technical Feasibility 20, Innovation 20, Demo Readiness 15, Presentation 10, Uniqueness 5, Novelty 5.

**Project separation (originality evidence)**
- New repo `fleetflow/` with its own `git init`, LICENSE, and README before any code. Initial commit on or after 11 Oct 2026.
- Do not copy files or code from `sumo-routing/` (thesis prototype, reference only) or `fleetbase/` (separate clone, reference / local integration test only).
- Do not commit Fleetbase or SUMO code inside the FleetFlow repo. Describe the earlier prototype and thesis as background only.
- Do not reuse thesis text in the repo or the submission.

**Assumptions**
- Telemetry comes from the built-in simulator for the demo. The same API accepts reports from real clients, so real sources can be added later without changing the system.
- Persistent data (vehicles, drivers, trips, saved runs) lives in PostgreSQL. Live congestion state stays in memory.
- A simplified Jakarta-style road network is enough to show the effect.
- Capacity of each road is estimated from lanes and length, which is a simplification.
- One operator demo login (or none) is acceptable.

## 7. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Scope too large for about 7 days | Unfinished MVP | Cut order: stretch goals, then extras, never the comparison view and the deployment |
| FleetFlow does not clearly beat baseline in the demo | Weak pitch | Tune parameters, report honestly, and show where it breaks as an engineering finding |
| Route oscillation or the avoidance blind spot | Unstable results | Hold time, slow decay, switching margin |
| Judges see overlap with existing fleet tools | Low Uniqueness and Novelty scores | Position as congestion-aware coordination, not a general fleet management suite |
| Originality question over earlier thesis work | Disqualification risk | New repo, fresh history, nothing copied, disclosed to organizers |
| In-memory state lost on restart | Demo interruption | Demo is seed-based and restartable, persistent records go in the database |
| Real-world benefit is unproven | Overclaiming | Describe results as simulation only and plan a small pilot after the sprint |
| Fleet is a small share of real traffic | Little measurable effect on real roads | Test several fleet shares and report the size at which it starts to matter |
| Location data is personal data | Privacy and legal risk if used with real people | Do not collect real driver locations during the competition and review Indonesia's data protection law (UU PDP) before any pilot |

## 8. Open questions

1. Is FleetFlow a standalone management system, or a routing layer that plugs into one like Fleetbase? (This PRD assumes a lean standalone system with routing as the headline, so confirm.)
2. Is the earlier thesis prototype acceptable as disclosed background, given the "start from 11 October" rule? (Awaiting organizers.)
3. Is an extra Fly.io preview deployment acceptable alongside the required Cloud Run one?
4. Decided: PostgreSQL. Open: where to host it (Cloud SQL vs. an external free tier) and the cost during the competition.
5. Which part of Jakarta, and how many roads, gives the clearest demo in the time available?
6. Does a demo login or any authentication matter for judging?
7. Which of the three metrics should lead the pitch: trip time, saturated roads, or route stability?
8. Do we need a plan for who maintains the project after the competition?

## 9. Roadmap after the sprint

1. Pilot with one small real fleet and measure the real effect.
2. Driver web app that sends GPS from a phone.
3. Map matching so raw GPS becomes road segments.
4. Organizations and roles (tenant separation).
5. API key rotation and revocation.
6. Persist congestion state so restarts keep it; plan for multi-instance state.
7. Faster routing (Customizable Contraction Hierarchies) for city-scale graphs.
8. Adapter for existing platforms such as Fleetbase.
9. Operations: Postgres backups, documented runbook, architecture note for contributors.
10. Privacy: retention limits and consent for location data (UU PDP review before any pilot).
