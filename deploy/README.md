# FleetFlow Cloud Run deployment

FleetFlow deploys to Google Cloud Run as a container service in `asia-southeast2`
(Jakarta).

If Cloud Run is blocked on billing, [free-hosting.md](./free-hosting.md) describes a
public URL in the meantime on a host that asks for no card, with the list of what
that deployment cannot do.

## What the image is

1. **Container image.** Built from the root `Dockerfile`: the Go API binary, the
   compiled React console, the SUMO packages Debian bookworm provides (the build
   fails if `sumo`, `duarouter` or `randomTrips.py` is missing, and prints
   `sumo --version` into the build log so the version is recorded rather than
   assumed), and `data/networks/jakarta.net.xml`.
2. **CPU allocation.** `run.googleapis.com/cpu-throttling: "false"` keeps CPU
   allocated while a scenario runs in a background goroutine, so a simulation is
   not frozen between HTTP requests.
3. **Execution environment.** `gen2`, so SUMO child processes behave as they do
   on a normal Linux host.
4. **PostgreSQL.** An external PostgreSQL 17 instance with the Flyway migrations
   in `db/migration` applied. `deploy/deploy.ps1` reads the connection string
   from the `fleetflow-database-url` secret; it is never committed.
5. **OSRM.** A separately operated OSRM server. `deploy/deploy.ps1` defaults to
   the public demo server (`https://router.project-osrm.org`), which is adequate
   for a showcase and not for production: it is shared, rate limited, and not
   the Jakarta extract. Point `OSRM_URL` at your own instance (`scripts/osrm`
   builds one) for real routing.
6. **Operator writes.** The showcase deploy leaves
   `REQUIRE_OPERATOR_KEY=false`, so vehicle and driver registration/profile
   edits, driver assignments, and trip creation/status changes are public.
   GPS ingestion at `POST /api/locations` always requires an API key. To protect
   the other operator writes instead, set `REQUIRE_OPERATOR_KEY=true`; `/readyz`
   then reports `writes: "protected_except_driver_assignment"`.
   For a key held in Secret Manager, run a one-off job with that secret exposed
   as `OPERATOR_KEY` and the binary's `-register-key` flag. It stores only the
   digest and does not print the key; do not run `-issue-key` in a Cloud Run job,
   because job output is retained in Cloud Logging.
7. **Probes.** `/healthz` is liveness, `/readyz` is readiness. Readiness reports
   `routing: "unavailable"` when OSRM does not answer and `database:
   "unreachable"` when the database does not, and answers 503 in both cases. A
   service that cannot route is not ready, and this deployment says so instead of
   drawing a route nobody computed.

## Prerequisites

Cloud Run's free tier covers a small service, but the **project must have an open
billing account**: Google Cloud refuses API calls, including deploys, without
one. Check before anything else, because it is the failure that looks like a
permissions problem:

```bash
gcloud billing projects describe $PROJECT_ID --format 'value(billingEnabled)'
```

Then:

1. Enable `run`, `cloudbuild`, `artifactregistry`, `sqladmin` and `secretmanager`.
2. Create the Artifact Registry repository: `gcloud artifacts repositories create
   fleetflow --repository-format docker --location asia-southeast2`.
3. Store the connection string:
   `gcloud secrets create fleetflow-database-url --data-file db-url.txt`
   (delete `db-url.txt` afterwards; never commit it).
4. Apply the migrations against that database:
   `docker compose --profile tools run --rm migrate`.

## Deploy

```powershell
.\deploy\deploy.ps1 -Project <project-id> -Tag <release-tag>   # current: v0.1.4
```

It builds with Cloud Build, deploys with `min-instances 0`, `1` vCPU and `1Gi`,
and prints the service URL and its `/readyz` address. Pass the release tag so the
running image, the git tag and the demo are the same code.

`deploy/cloudrun.service.yaml` is the reference for a full `gcloud run services
replace` deployment (probes, secrets, environment). If you use it, update the
image path to your own project and keep `minScale: "0"` unless you have accepted
continuous-instance billing.

## Deploy on a tag

`cloudbuild.yaml` at the repository root is the deployment with a human removed:
it builds the image, tags it with both the release tag and the commit SHA, pushes
it to Artifact Registry, and deploys it. A Cloud Build trigger watching the
repository for `^v[0-9]+\.[0-9]+\.[0-9]+$` makes `git push origin v0.1.9` the
whole release.

The one-time setup, all of it inside the project and none of it in this repo:

1. **Connect the repository.** In the console, Cloud Build → Triggers → *Connect
   repository* → GitHub, which authorises the Cloud Build GitHub App against the
   account that owns the fork. This step needs a browser; a CLI cannot complete
   the OAuth handshake.
2. **Create the trigger** once the repository is connected:
   `gcloud builds triggers create github --name=release-tag --repo-owner=<owner>
   --repo-name=fleetflow --tag-pattern="^v[0-9]+\.[0-9]+\.[0-9]+$"
   --build-config=cloudbuild.yaml --region=asia-southeast2
   --project=<project-id>`.
3. **Give the build permission to deploy.** The Cloud Build service account needs
   `roles/run.admin`, `roles/artifactregistry.writer` and
   `roles/iam.serviceAccountUser` on the project. Without the last one the deploy
   fails at the point of setting the runtime service account, which reads like a
   Cloud Run error and is not.

Why a tag and not a push: a branch build would deploy every intermediate commit,
and this project's history is deliberately many small commits. A tag is the one
moment where "this is the release" was decided by a person.

The image is tagged with `${TAG_NAME}` and `${COMMIT_SHA}`, so
`gcloud run revisions describe` names the code a revision runs. The manual script
still defaults to `:latest`, which does not; pass `-Tag <release>` to it.

Nothing here works while the billing account is closed: Cloud Build and Cloud Run
both refuse the work, and the service keeps the last revision it managed to
deploy, which will not start. The configuration does not expire.

## Cost

Staying inside the free tier means `minScale: "0"`, one vCPU, and no always-on
revision. `cpu-throttling: false` keeps a scenario running while an instance is
alive; with `minScale 0` the instance still scales away when idle. A showcase
service doing a few hundred requests a day sits far inside the monthly free
allowance, but the bill is the project owner's to watch, and this repository
cannot promise a figure.
