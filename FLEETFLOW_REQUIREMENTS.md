# FleetFlow: Requirements

**An open-source fleet navigation platform with dynamic weights.**
In plain terms: it keeps a fleet from congesting its own roads by re-weighting routes from live traffic.

Competition: Road To DevFest Jakarta 2026 (GDG Jakarta)
Author: Frederick Garner Wibowo
Last updated: 17 October 2026

---

## 1. Competition constraints (from the Playbook)

- [ ] Project must start on or after **11 October 2026**, verified by the **initial commit on GitHub**. Pre-existing projects are strictly prohibited.
- [ ] Deliver an **MVP**, deployed and live.
- [ ] Deploy via **Cloud Run** (mandatory).
- [ ] All work must be original. Third-party code and data need proper attribution and license compliance.
- [ ] Use of LLMs is permitted.

**Key dates**

| Date | Phase |
|---|---|
| 17 Oct | Building the Project (token usage, local LLMs) |
| 24 Oct | Ship to Production |
| 25 Oct | Pitch Perfect & Talent Insight |
| 27 Oct | Project Showcase (live evaluation) |
| 1 Nov | Winner announcement |

**Judging weights:** Impact 20, Technical Feasibility 20, Innovation 20, Demo Readiness 15, Presentation 10, Uniqueness 5, Novelty 5.

---

## 2. Project separation and folder layout

Three sibling folders, each its own project, with **no parent git repo**:

```
workspace/
├── sumo-routing/   # existing thesis prototype (reference only, never copied)
├── fleetbase/      # separate clone of github.com/fleetbase/fleetbase (reference / local integration test)
└── fleetflow/      # NEW competition repo: fresh git init, license, initial commit first
```

Rules:

- [ ] `fleetflow/` gets its own `git init`, a LICENSE file, and a README before any code.
- [ ] Do **not** copy files from `sumo-routing/` or `fleetbase/` into `fleetflow/`.
- [ ] Do **not** commit Fleetbase or SUMO code inside the FleetFlow repo.
- [ ] Describe the earlier prototype and thesis as background only, in the README.
- [ ] Do not reuse thesis text in the repo or the submission.

---

## 3. Must-have requirements (MVP)

### 3.1 Routing service (Go)

| ID | Requirement |
|---|---|
| R1 | Accept vehicle reports (road/edge ID, speed, timestamp) and maintain per-road congestion state. |
| R2 | Smooth congestion with an exponential moving average. |
| R3 | Decay stale values toward free-flow when a road has no recent report. |
| R4 | Refresh routing weights periodically without blocking route queries (immutable snapshot, atomic pointer swap). |
| R5 | Provide a route query returning a road sequence for an origin and destination. |
| R6 | Provide a baseline mode (static shortest path) for comparison. |
| R7 | Expose an API: `POST /reports`, `POST /route`, `GET /metrics`. |

### 3.2 Fleet simulator

| ID | Requirement |
|---|---|
| S1 | Simulate N vehicles with origin-destination trips on a simplified Jakarta road graph. |
| S2 | Vehicles send reports to the service and follow the routes it returns. |
| S3 | Deterministic with seeds, so baseline and FleetFlow runs are comparable. |
| S4 | A built-in demo scenario that runs with one command. |

### 3.3 Operator dashboard (web)

| ID | Requirement |
|---|---|
| D1 | Live map showing vehicles and congested roads. |
| D2 | Side-by-side comparison of baseline vs. FleetFlow: average trip time, saturated roads, route changes. |
| D3 | Start / reset controls for the demo scenario. |

### 3.4 Deployment and open source

| ID | Requirement |
|---|---|
| O1 | Runs on Cloud Run. State is in memory, so run a single instance (`max-instances=1`) or externalize state. |
| O2 | Public GitHub repo with an open-source license (Apache-2.0 or MIT), chosen at the first commit. |
| O3 | README with setup and run instructions, and the deployment link. |
| O4 | Attribution for third-party data and code (for example OpenStreetMap, ODbL). |

---

## 4. Should-have

- [ ] **Fleetbase integration (adapter, not a fork):** read orders and vehicles from the Fleetbase REST API or webhooks and return recommended routes. Verify endpoints in the Fleetbase docs before committing to this.
- [ ] Oscillation protection, such as a hysteresis threshold before a vehicle switches routes.
- [ ] Tests, including `go test -race` for concurrency correctness.
- [ ] Measured numbers: route query latency (p50/p99) and weight refresh time.
- [ ] Defined road capacity for the saturation metric (state how it is derived).

## 5. Could-have (roadmap only)

- Optional SUMO adapter
- An ADK agent that tunes parameters
- Multi-stop routing
- Real GPS ingestion

---

## 6. Fleetbase notes

What the Fleetbase README says: a modular open-source logistics operating system covering dispatch, fleet management, live tracking, route optimization, warehouse and finance, with a REST API, webhooks, WebSockets and an extension system. It is dual-licensed (AGPL-3.0 and a commercial license).

- **License:** under AGPL-3.0, modifying Fleetbase and offering it over a network requires releasing those modifications under AGPL-3.0. Keep FleetFlow a separate service that talks to Fleetbase over an API, and read their licensing page before bundling anything. This is not legal advice.
- **Deployment risk:** Fleetbase needs Docker Compose and about 4 GB of RAM for several services. Do not try to deploy it to Cloud Run. Deploy only FleetFlow there, and run Fleetbase locally for the integration demo or a recorded clip.
- **Overlap:** Fleet-Ops already advertises route optimization. Read what it does before claiming a difference. Position FleetFlow as congestion-aware coordination across the fleet, designed to complement fleet management tools.
- **Naming:** names starting with "Fleet" will be confused with Fleetbase in search. Search GitHub for existing "FleetFlow" projects before committing to the name.

---

## 7. Submission deliverables

- [ ] Source code with configuration, README and deployment link
- [ ] Demo video, maximum 3 minutes (core functionality)
- [ ] Written project summary, maximum 500 words
- [ ] Final presentation deck (PDF plus a backup PPT/Keynote on a flash drive)
- [ ] Backup presentation video, maximum 15 minutes (technology, business model, future plan)
- [ ] Live demo, fully deployed and operational

---

## 8. Suggested priority order

1. Repo setup: `git init`, license, README skeleton, initial commit.
2. Routing service core (R1 to R7) with baseline mode.
3. Simulator with the demo scenario.
4. Dashboard comparison view (the centerpiece of the demo).
5. Cloud Run deployment (do this early, not on the last day).
6. Tests, metrics, README polish.
7. Fleetbase adapter (first thing to cut if time runs short).
8. Demo video, summary, deck.

---

## 9. Open questions for the organizers

- Is a new repo that builds on a concept I researched earlier (with separate, uncopied code) acceptable under the project timeline rule?
- Is an open-source license on the submission welcome or required?
