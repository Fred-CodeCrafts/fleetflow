# FleetFlow — master prompt for a new agent

You are continuing work on **FleetFlow**, a Go fleet-navigation service with a
React console, in `C:\Users\frede\documents\fleet project\Fleetflow`.
This document is your complete brief. Read it, then read the files it names, then
work. Do not stop until the Definition of Done at the end is met.

---

## 1. What the project is

FleetFlow routes a fleet over a road network using **live, smoothed congestion
weights**, and proves the benefit by running the same seeded demand twice at once:
one arm on static free-flow shortest paths (the *baseline*), one on dynamic
weights (the *FleetFlow* arm). It is a Road To DevFest Jakarta 2026 submission.

`PRD.md` is the source of truth. `DESIGN.md` explains the architecture.
`TASKS.md` is the task tracker. `AGENTS.md` holds the rules you must follow.
Read all four before your first commit.

**Non-negotiable product claims.** The results are simulation only. The README
must say "production-shaped architecture, MVP feature set" and never
"production-ready". The service is containerized but has not been deployed.

---

## 2. Current state (verified 2026-10-19)

Branch: **`feat/console`** (branched from `main` at `f2b1025`).
Working tree is clean. The console, backend integration, container workflow, and
documentation are committed in these changes:

```
59a4231 docs: finalize console setup guidance
f68834e build(deploy): add local container workflow
2cb4ce0 feat(api): expand runtime data APIs
b3c47c2 feat(web): replace legacy dashboard with React console
f3fcacb feat(trip): record when a trip started and finished
7d6907a feat(emissions): estimate a trip's exhaust emissions from speed
dd8f7f6 feat(graph): import a SUMO .net.xml network
4c19388 docs: make the front end build to fleetbase's architecture
d36d283 docs: remove the attribution rule agpl does not ask for
03df3b6 docs: correct the README where it described a different program
```

`go build ./...`, `go test ./...`, `go vet ./...`, formatting checks, and the
WSL race suite pass. The console build and four tests pass. A clean local clone
serves the embedded console without Node. The local Docker image also serves the
console and `/healthz`.

### Environment facts you will need

- **Go 1.25.5** on Windows. PowerShell 5.1: no `&&`, no `-SkipHttpErrorCheck`,
  `Invoke-WebRequest` needs `-UseBasicParsing`, `COMPOSE_FILE` separator is `;`.
  `.NET` file APIs need **absolute** paths (they use the process directory, not
  PowerShell's location).
- **`go test -race` needs WSL** (Windows has no gcc):
  `wsl.exe bash -lc "cd '/mnt/c/Users/frede/documents/fleet project/Fleetflow'; ~/tools/go/bin/go test -race -count=1 ./..."`.
  WSL cannot reach Windows-hosted HTTP servers — use PowerShell for that.
- **Node 24.12 / npm 11.6** are installed.
- **Docker Desktop** must be running. `fleetflow-postgres` listens on
  **127.0.0.1:55432**, database `fleetflow`, user `fleetflow`, password in
  project `.env` (never print it, never commit it).
- Dev databases: `fleetflow`, `fleetflow_run`, `fleetflow_vehicle`,
  `fleetflow_driver`, `fleetflow_trip`, `fleetflow_auth`.
- Migrations are **Flyway**, `db/migration/V<n>__<desc>.sql`. Applied migrations
  are immutable. V1–V5 are applied. Apply with:
  `docker exec -i -e PGPASSWORD=$p fleetflow-postgres psql -U fleetflow -d <db> -v ON_ERROR_STOP=1 < file.sql`
- **Ports in use:** 8080/3000 by another project, 3306 by native `MySQL80`,
  55432 by Postgres, 8081 free for FleetFlow.
- The **MCP** `toolbox-postgres` is connected via a project `opencode.json`
  (gitignored, carries the dev credential). Primary agents are denied its tools
  by design; delegate live queries to the `db` subagent, and only when real data
  is needed.

---

## 3. The user's actual request (verbatim intent)

> "I want the map to be like in sumo routing — the library it uses, how it shows
> the routes — and then the ability to use city XML like Jakarta, with the
> addition of the ability to make the XML in this site. And instead of showing
> all background vehicles, FleetFlow can toggle background cars or just the cars
> they track. The point is to optimise the front. Build with an actual framework
> like React. Right now all configurations are in one page; each should have its
> own page. Base the front end exactly like Fleetbase — I mean exactly. Any
> features we don't have, implement them. I'm very interested in how they track
> emissions."

Plus: **"not just front end but backend as well"**, and **"keep going until the
project is done."**

### What was learned from the references

`../sumo-routing/sumo-routing-frontend` is **React 19 + Vite + TypeScript +
MapLibre GL + deck.gl + proj4**. Its `NetworkMap.tsx` is the pattern to rebuild:

- `PathLayer` for roads, coloured by `load/capacity` ratio in bands
  (≤0.5 green, 0.85 amber, ≥1 red), width by road class, wider when selected.
- `ScatterplotLayer` for junctions, zoom-gated, and for origin/destination
  markers in a "pick a node" mode.
- `IconLayer` for vehicles with **per-vehicle-ID lerp** (250 ms), not deck.gl
  transitions — index-based transitions make dead vehicles "continue" as new
  ones. Also birth/death hold markers (500 ms hold, 2 s TTL).
- `MapboxOverlay` from `@deck.gl/mapbox` over a MapLibre basemap
  (`https://tiles.openfreemap.org/styles/positron`).
- `proj4` to convert the network's projection to WGS84.
- Toggles: **peak load vs instantaneous**, junction dots, re-center.

`../fleetbase` console is **Ember 5.4**. The fleet UI packages are **empty
submodules**, so the reference screens were cloned to
`C:\Users\frede\AppData\Local\Temp\opencode\fb-ref\{fleetops,ember-ui,ember-core}`
(5,096 / 1,903 / 616 files). Read them there. Its architecture:

- **Navigation is a registry, not hardcoded JSX.** Sidebar composes core branches
  plus registry items, each with `id/label/icon/route/defaultRoute/keywords/
  children/permission`. Branches: Operations, Resources, Maintenance,
  Connectivity, Analytics, Settings.
- **Page shell**: `Layout::Container / Main / Sidebar / Section / Header / Body /
  Footer`.
- **One CRUD abstraction per entity**: `Layout::Resource::Tabular` /
  `CardsGrid` / `Panel` / `TabularActions`, driven by `@resource`, `@columns`,
  `@registry`, `@page`, `@emptyStateComponent`, `@actionButtons`, `@bulkActions`.
- **Route shape per entity**: `index / index.details / index.details.<tab> /
  index.new / index.edit`.
- Controllers own `columns`, `actionButtons`, `bulkActions`, `*Actions`.
- Permissions gate every action and menu item at render time.
- **One settings route per group** (`settings.map`, `settings.routing`, …) —
  exactly the fix the user asked for.
- **Their emissions feature is a gap.** `vehicle.emission_standard` is a string
  and "Minimize Fuel / Emissions" is an objective label. **Nothing computes CO₂.**
  FleetFlow already beats this (see §4).

**Licensing boundary (settled — do not relitigate).** Fleetbase is AGPL-3.0,
this repo is Apache-2.0. Build to their **architecture, layout and feature set**;
that is explicitly the goal per `AGENTS.md` §4. The **only** prohibition is
pasting their source files, templates, styles or assets, or vendoring their
packages. **No attribution, NOTICE entry or credit line is owed** for studying a
reference — AGPL's notice duties attach only to conveying the Program or a work
based on it. `AGENTS.md` says this; do not add credits.

---

## 4. What is already built (backend)

### `internal/graph` — SUMO network import ✅ committed (`dd8f7f6`)
`ImportSUMONetwork(io.Reader, ImportOptions) (*Graph, error)` parses `.net.xml`.
Junctions→nodes, edges→directed segments, lane speed/length derived. Coordinates
map through the file's own `convBoundary`/`origBoundary` pair (do **not**
subtract `netOffset` — junction x/y are already in the conv frame; doing so puts
Jakarta in the Indian Ocean, and the test catches it). `Edge.Shape []Point` holds
the drawn centreline. Internal junction edges (`function="internal"` or `:` ids)
are dropped by default; `MinLengthM` drops connector stubs.

Sentinel errors: `ErrNotSUMONetwork`, `ErrNoRoutableEdges`,
`ErrMissingProjection`, `ErrTooManyElements`.

Proven on the real 12 MB Jakarta Sudirman extract:
**3,926 junctions, 4,310 edges, all with shape, placed in Jakarta.**

### `internal/emissions` — COPERT model ✅ committed (`7d6907a`)
`Trip(class, distanceKM, meanSpeedKmh) (Result, error)` using
`EF(v) = (a + b·v + c·v² + d/v)/(e·v + f)`. Classes: `PassengerCar` (Euro 5
petrol), `LightCommercial` (Euro 5 diesel). CO₂ = reference factor × U-shaped
speed correction. Clamps to the 10–130 km/h fit range. Returns CO₂, NOx, CO, HC,
PM in grams plus litres of fuel. `Method` constant carries the disclaimer (no
cold starts, gradient, payload, ambient temperature).

Tests assert congested traffic emits strictly more than free flow — if that
inverts, the console's environmental claim is false.

### `internal/trip` — start/finish timestamps ✅ committed (`f3fcacb`)
`Trip.StartedAt`, `Trip.FinishedAt`, `Trip.Duration() (time.Duration, bool)`.
Set by the transition, not the client: start on first `in_progress`, finish on
first `completed`, idempotent (`coalesce` in SQL). A trip that skips straight to
completed keeps a null start and reports no duration — never a fabricated one.
V5 migration adds both columns (nullable), a `finished_at >= started_at` check,
and a partial index on completed trips.

### Integrated implementation (committed)

| file | what it does |
|---|---|
| `internal/sim/sim.go`, `vehicle.go`, `metrics.go` | per-trip `distanceM`, accumulated in `enterEdge` next to `freeFlowS` so a reroute's detour counts. `Metrics.Emissions` + `DistanceKM`, computed **per trip** then summed (not from a run mean speed — the curve is non-linear). |
| `internal/api/metrics.go` | `armMetrics.Emissions` (`emissionsMetrics` with `Method`) and `DistanceKM`. |
| `internal/api/trips.go` | `tripView` gains `started_at`, `finished_at`, `duration_s` as **pointers**, so "not recorded" is `null`, not `0`. |
| `internal/api/emissions.go` | `GET /api/emissions` — live run's two arms + `savings` (baseline − fleet_flow) + optional `?class=&distance=&speed=` trip estimate + `classes` + `method`. `queryFloat` helper, `perKM` returns 0 rather than NaN/Inf. |
| `internal/api/network.go` | `GET /api/network` (description: name, source, nodes, edges, shaped_edges, capacity, length_km, bounds) and `POST /api/network` (raw `.net.xml` body, `MaxBytesReader` 200 MiB, refused with `409 network_busy` while a run is going). `installGraph` swaps graph+engine+labels under `networkMu`, starts the new customizer, stops the old one after the swap. |
| `internal/api/engine_build.go` | `newEngineFor(g)` and the `engineForBuilds` seam. |
| `internal/api/server.go` | `networkMu`, `networkName`, `networkSource`, `stopCustomizer` fields; `WithNetworkName`, `WithCustomizerStopper` options; three new routes registered. |
| `console/` | Complete React 19 + TypeScript operator console with Vite build, typed API client, resource screens, per-group settings, MapLibre/deck.gl map, map-based trip picker, and emissions/network pages. `npm ci`, build, and tests pass. |

The work above has been tested and committed. `console/node_modules/` and
`.env` are ignored and not tracked; generated `web/dist/` assets are intentionally
committed because `go:embed` ships them in the binary.

---

## 5. Remaining work

- Verify the imported map with a real Jakarta `.net.xml`. The browser flow was
  verified using a synthetic two-node SUMO fixture; no real city network file
  is available in this checkout.
- Expose custom network selection through CLI/config so a deployment can start
  on an imported graph. Runtime upload via the Network page is implemented.
- Deploy to Cloud Run with one instance and production PostgreSQL. This is
  blocked until a GCP project/authenticated `gcloud` environment and a production
  database are provided; no deployment or public URL is claimed.
- The optional Fly preview, release tag, and pitch deliverables remain open;
  see `TASKS.md` for T59 and T67–T72. T58 and T68 require a real deployment.
- The README, screenshot, clean-clone check, container build, Go checks, race
  tests, and local browser flows are complete; their evidence is recorded in
  `TASKS.md`.

---

## 6. Rules you must follow

From `AGENTS.md`; these are enforced by review, not preference.

- **Small commits.** One idea, 1–3 files, under ~100 changed lines (generated
  files exempt, say so in the message). The repo must build and tests must pass
  at every commit. Never leave work uncommitted at the end of a task.
- **Conventional Commits**, imperative, ≤72 char summary, body explains *why*.
  Scopes: `graph`, `weights`, `engine`, `sim`, `fleet`, `store`, `auth`, `api`,
  `web`, `config`, `domain`, `arch`, `scripts`, `deploy`.
- **Never rewrite history.** No rebase, amend of pushed commits, force-push, or
  backdating. Fix mistakes in new commits.
- **No secrets.** Never commit `.env` or `opencode.json`. Never print a password.
  Never connect to a non-loopback host.
- **`gofmt` + `go vet`** before every Go commit. `go test -race` via WSL for
  anything touching concurrency.
- **Every "met" claim cites evidence** — a test name or a line number. Do not
  tick a task whose done-when is not literally satisfied.
- **Verify, don't narrate.** Read the code; do not trust this document or your
  own memory over the file.

---

## 7. How to work

1. `git status`, `git log --oneline -5`, read `AGENTS.md`, `PRD.md`,
   `DESIGN.md`, `TASKS.md`.
2. Verify the uncommitted backend work: `go build ./... && go test ./...`, then
   read each diff. Commit it in logical pieces.
3. Then the console, in the order of §5A, committing each step.
4. Prefer dedicated tools over shell. Parallelise independent reads.
5. Delegate live database questions to the `db` subagent only; never for
   code-only work.
6. When something fails, read the error and fix the cause. Do not paper over it,
   and do not stop.

---

## 8. Definition of done

- [x] `go build ./...`, `go vet ./...`, `go test ./...` all pass.
- [x] `go test -race ./...` passes in WSL.
- [x] `cd console && npm run build` succeeds and its output is what `go:embed`
      ships; `go run ./cmd/fleetflow -demo` serves the console with no Node
      present.
- [x] Every branch and item in `navigation.ts` resolves to a real page; no
      configuration lives on a shared page.
- [ ] The map draws an imported Jakarta `.net.xml` with shaped roads, coloured by
      load, with a working background-vehicle toggle and peak/instantaneous
      switch. Synthetic import verified; real Jakarta file still needed.
- [x] Uploading a `.net.xml` from the Network page switches the running graph and
      says so; a bad file gets a clear, specific error.
- [x] The Emissions page shows both arms, the savings with an unambiguous sign,
      the method disclaimer, and a per-trip calculator.
- [x] A completed trip shows start, end and duration; one that never started
      shows "not recorded" rather than a fabricated duration.
- [ ] `PRD.md` acceptance criteria are ticked only where a test or a line of code
      proves them; `TASKS.md` ticks match the commits.
- [x] README's quick start, endpoint list and deployment claims match reality.
- [x] `git status` is clean and changes are committed in logical units.

---

## 9. Traps that already cost time

- **PowerShell 5.1 writes a UTF-8 BOM** with `Set-Content -Encoding UTF8`, which
  silently breaks JSON config. Write bytes with
  `[System.IO.File]::WriteAllText($abs, $text, (New-Object System.Text.UTF8Encoding($false)))`.
- **`.NET` file APIs resolve relative to the process directory**, not
  PowerShell's `cd`. Use absolute paths.
- A chained `TrimStart` on a null string once truncated a config file to 0 bytes.
  Check `.Length` after rewriting a file.
- **`netOffset` must not be subtracted** from SUMO junction coordinates.
- `graph.Edge` is **non-comparable** now that it carries `Shape`; look up via
  `edgeIndices`, never `==`.
- **Vite dev server on 5173** proxies `/api` to `127.0.0.1:8080`; FleetFlow
  usually runs on **8081** (`PORT=8081`), so adjust the proxy or the env.
- WSL cannot reach Windows-hosted HTTP; use PowerShell for live checks.
- `docker` and `git` write to stderr, which PowerShell surfaces as a spurious
  non-zero exit; check `$LASTEXITCODE`, not the stream.
