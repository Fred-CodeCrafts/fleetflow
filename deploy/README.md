# FleetFlow Cloud Run Deployment

FleetFlow deploys to Google Cloud Run as a containerized service in `asia-southeast2` (Jakarta).

## Architecture & Requirements

1. **Container Image**: Built using the root `Dockerfile`. Bundles the Go API binary, compiled React console, Eclipse SUMO 1.27.1 runtime, and the verified Jakarta road network (`/app/data/networks/jakarta.net.xml`).
2. **CPU Allocation**: `run.googleapis.com/cpu-throttling: "false"` ensures CPU remains allocated during scenario computation so background simulation processes are not throttled between HTTP requests.
3. **Execution Environment**: `gen2` provides standard Linux kernel support and compatibility for SUMO process execution.
4. **PostgreSQL**: Managed Cloud SQL for PostgreSQL 17 instance with Flyway migrations applied. Durable runs and multi-instance concurrency locks use PostgreSQL partial unique indexing and transactions.
5. **OSRM**: Dedicated internal or sidecar OSRM service with the preprocessed Jakarta road network extract.
6. **Probes**:
   - `/healthz`: Liveness and startup check.
   - `/readyz`: Readiness check verifying database connectivity, OSRM reachability, and SUMO runtime.

## Deployment Steps

```bash
# 1. Build and push container image to Google Artifact Registry
gcloud builds submit --tag asia-southeast2-docker.pkg.dev/$PROJECT_ID/fleetflow/fleetflow:latest .

# 2. Deploy service specification
gcloud run services replace deploy/cloudrun.service.yaml --region asia-southeast2
```
