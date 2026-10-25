# Builds the image, deploys it to Cloud Run, and prints the URL.
#
# Prerequisites, in order:
#   1. An open billing account on the project (Cloud Run needs it even when the
#      month stays inside the free tier).
#   2. The APIs: run, cloudbuild, artifactregistry, sqladmin, secretmanager.
#   3. Secret Manager holds fleetflow-database-url.
#   4. An Artifact Registry repository named fleetflow in asia-southeast2.
#
# Usage:
#   .\deploy\deploy.ps1 -Project fleetflow-4448
#   .\deploy\deploy.ps1 -Project fleetflow-4448 -Tag <release-tag>

param(
  [Parameter(Mandatory = $true)][string]$Project,
  [string]$Region = 'asia-southeast2',
  [string]$Tag = 'latest',
  [string]$Repository = 'fleetflow',
  [string]$Service = 'fleetflow'
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  $image = "$Region-docker.pkg.dev/$Project/$Repository/fleetflow`:$Tag"

  Write-Host "Building and pushing $image"
  gcloud builds submit --project $Project --tag $image .
  if ($LASTEXITCODE -ne 0) { throw "image build failed" }

  Write-Host "Deploying $Service to $Region"
  gcloud run deploy $Service `
    --project $Project `
    --region $Region `
    --image $image `
    --platform managed `
    --allow-unauthenticated `
    --min-instances 0 `
    --max-instances 3 `
    --cpu 1 `
    --memory 1Gi `
    --timeout 300 `
    --no-cpu-throttling `
    --set-secrets "DATABASE_URL=fleetflow-database-url:latest" `
    --set-env-vars "ENV=production,LOG_LEVEL=info,OSRM_URL=https://router.project-osrm.org"
  if ($LASTEXITCODE -ne 0) { throw "deploy failed" }

  $url = gcloud run services describe $Service --project $Project --region $Region --format 'value(status.url)'
  Write-Host ""
  Write-Host "Service URL: $url"
  Write-Host "Readiness:   $url/readyz"
  Write-Host ""
  Write-Host "Probes and resource limits in deploy/cloudrun.service.yaml are the"
  Write-Host "reference for a full service-replace deployment."
} finally {
  Pop-Location
}
