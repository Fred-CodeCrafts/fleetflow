# Free hosting before Cloud Run

Cloud Run is the target and the Dockerfile is built for it, but a project needs an
open billing account even for free-tier usage, and that is currently what blocks
the deploy. This is a public URL in the meantime, on a host that asks for no card.

**What this deployment is:** the console and the API, running in memory, without a
routing service. **What it is not:** the demonstration described in the README. The
Route Planner will answer 503 because there is no OSRM next to it, and the fleet
data resets whenever the instance restarts. Read the table below before sending the
link to a judge.

## Render (about ten minutes, no card)

This is the path that works. Render's free web service asks for no card and runs
the slim image in [`free/Dockerfile`](./free/Dockerfile), which builds the console
and the Go binary from the release tag and leaves out SUMO and OSRM.

1. Push this repository to GitHub (already done).
2. Sign up at render.com with GitHub. No payment method is requested for the free
   instance type.
3. **New → Blueprint**, pick the `fleetflow` repository and the `main` branch.
   Render reads [`render.yaml`](../render.yaml), which names the service, the
   Dockerfile and the health check.
4. Apply. The first build compiles the console, then the Go binary; expect four to
   six minutes.
5. Open the URL Render prints, or find it under the service's **Overview** tab. It
   looks like `https://fleetflow-xxxx.onrender.com`.

Render's free instance, in the words of their own documentation (October 2026):

- **512 MB of RAM and 0.1 of a CPU.** The console and API idle well under 100 MB,
  but the Routing Lab's sweep, which takes a fraction of a second on a laptop,
  takes a few seconds here.
- **It sleeps after fifteen minutes without traffic**, and takes about a minute to
  wake. Render shows its own loading page while that happens, so the first click
  after a quiet spell looks like nothing is happening. Ping the URL a minute
  before a judge opens it.
- **750 free instance hours per workspace per month**, and a service that is spun
  down does not spend them. One service awake every hour of the month is about 730
  hours, so a single always-warm deployment fits with very little to spare. Run a
  second free service and the workspace suspends both when the hours run out.
- **The filesystem is ephemeral and free instances cannot attach a disk.** Every
  redeploy, restart or spin-down erases whatever the process wrote, which is why
  the seeded runs are regenerated at each start and why persistence means a
  database, not a file.
- **One instance, no scaling, no shell.** That suits this demo: a single instance
  cannot serve a request from one store and a half-written record from another.
- **No card is required for the free plan.** Outbound bandwidth and build minutes
  are monthly allowances shared by the workspace; exceeding them without a payment
  method suspends the free services until the month turns over.

That is the price of zero.

### Hugging Face Spaces, if you have PRO

Hugging Face stopped offering free CPU hardware to new Spaces: their documentation
states that Gradio and Docker Spaces "require a paid plan to create: PRO for
personal accounts", and the only free hardware a new account sees is ZeroGPU, which
is for GPU demos. Without PRO the Docker SDK is not selectable, so this is not a
configuration mistake to work around.

With PRO, the same image works: upload [`huggingface/README.md`](./huggingface/README.md)
as the Space's `README.md`, upload [`free/Dockerfile`](./free/Dockerfile) as the
Space's `Dockerfile`, and change `ENV PORT=8080` to `ENV PORT=7860` to match the
`app_port` in the Space card.

## What works, and what does not

| Feature | On this deployment | Why |
|---|---|---|
| Console, all screens | works | embedded in the binary |
| Seeded scenarios, 3 runs, ~170 trips, 100+ vehicles | works, regenerated at every start | in-memory stores |
| Dispatch, trip history, emissions ledger | works | computed from the runs |
| Routing Lab (`/api/lab/dynamic`) | works | computed in memory, no external service |
| Route Planner (`/api/route`) | **503** | no OSRM: it needs a multi-gigabyte extract and more RAM than the free instance has |
| `/readyz` | reports `database: "not_configured"`, `routing: "unavailable"` | that is true, and it is why the health check is `/healthz` |
| Data across restarts | **resets** | no database configured |

The 503 is deliberate. The console shows the routing error instead of drawing a
route nobody computed, which is the same rule the rest of the project follows.

## Adding a database (still free, still no card)

Neon's free tier is the least work: create a project, copy its connection string,
then add it to the Render service as `DATABASE_URL` and redeploy. Render's own free
Postgres works too, but it expires thirty days after creation, which is shorter than
the interval between the submission and the judging. Run the migrations once from
your machine against that database:

```powershell
$env:DATABASE_URL = 'postgres://...'   # the Neon connection string
docker run --rm -v "$PWD/db/migration:/flyway/sql" flyway/flyway -url=$env:DATABASE_URL -locations=filesystem:/flyway/sql migrate
```

Then `/readyz` reports `database: "reachable"` and records survive a restart. The
routing line still says `unavailable`, because nothing has changed there.

## Getting the routing scene back

Two honest options, neither free in effort:

- **Hugging Face Spaces** allows a Docker Space with 16 GB of RAM, which is enough
  to run OSRM beside the app in one container. It needs a pre-built Jakarta extract
  baked into the image and a small supervisor to run both processes; it sleeps after
  48 hours of inactivity, and the platform is meant for ML demos.
- **A small VM** (Oracle's always-free tier, for instance) runs the Compose stack
  exactly as it runs locally, but signing up asks for a card for identity checks.

## Moving to Cloud Run

Nothing in this deployment changes the target. Once billing is open on the project:

```powershell
.\deploy\deploy.ps1 -Project fleetflow-4448 -Tag <release-tag>
```

That builds the same Dockerfile, pushes to Artifact Registry and deploys; the
README's status section is the place to record the URL. Delete the Render service
when the Cloud Run URL answers, so there is one place to look and one number to
believe.
