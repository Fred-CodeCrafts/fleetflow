#!/usr/bin/env bash
# FleetFlow Cloud Run deployment automation script.
# Usage:
#   export PROJECT_ID="your-gcp-project-id"
#   export REGION="asia-southeast2"           # Default: Jakarta
#   export DATABASE_URL="postgres://..."      # Optional: stores in Secret Manager
#   ./deploy/setup-cloudrun.sh

set -euo pipefail

PROJECT_ID="${PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
REGION="${REGION:-asia-southeast2}"
SERVICE_NAME="fleetflow"
REPO_NAME="fleetflow"

if [ -z "${PROJECT_ID}" ]; then
  echo "Error: PROJECT_ID is not set. Set with 'export PROJECT_ID=my-project-id' or 'gcloud config set project my-project-id'." >&2
  exit 1
fi

echo "==> Configuring Google Cloud Project: ${PROJECT_ID} (${REGION})"

echo "==> Enabling required GCP APIs..."
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  secretmanager.googleapis.com \
  --project="${PROJECT_ID}" \
  --quiet

echo "==> Ensuring Artifact Registry repository '${REPO_NAME}' exists..."
if ! gcloud artifacts repositories describe "${REPO_NAME}" --location="${REGION}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud artifacts repositories create "${REPO_NAME}" \
    --repository-format=docker \
    --location="${REGION}" \
    --description="FleetFlow container repository" \
    --project="${PROJECT_ID}" \
    --quiet
fi

IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/${SERVICE_NAME}:latest"

echo "==> Building and pushing container image: ${IMAGE}..."
gcloud builds submit --tag "${IMAGE}" --project="${PROJECT_ID}" .

SECRET_NAME="fleetflow-database-url"
if [ -n "${DATABASE_URL:-}" ]; then
  echo "==> Updating Secret Manager secret '${SECRET_NAME}'..."
  if gcloud secrets describe "${SECRET_NAME}" --project="${PROJECT_ID}" >/dev/null 2>&1; then
    echo -n "${DATABASE_URL}" | gcloud secrets versions add "${SECRET_NAME}" --data-file=- --project="${PROJECT_ID}" --quiet
  else
    echo -n "${DATABASE_URL}" | gcloud secrets create "${SECRET_NAME}" --data-file=- --replication-policy="automatic" --project="${PROJECT_ID}" --quiet
  fi
fi

echo "==> Deploying Cloud Run service '${SERVICE_NAME}'..."
gcloud run deploy "${SERVICE_NAME}" \
  --image="${IMAGE}" \
  --region="${REGION}" \
  --execution-environment="gen2" \
  --cpu="2" \
  --memory="2Gi" \
  --no-cpu-throttling \
  --port="8080" \
  --allow-unauthenticated \
  --project="${PROJECT_ID}" \
  --quiet

SERVICE_URL="$(gcloud run services describe "${SERVICE_NAME}" --region="${REGION}" --project="${PROJECT_ID}" --format='value(status.url)')"
echo "==> Deployment successful!"
echo "==> Service URL: ${SERVICE_URL}"
echo "==> Health check: ${SERVICE_URL}/healthz"
echo "==> Readiness check: ${SERVICE_URL}/readyz"
