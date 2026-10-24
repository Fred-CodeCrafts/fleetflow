# FleetFlow Cloud Run deployment automation script (PowerShell).
# Usage:
#   $env:PROJECT_ID = "your-gcp-project-id"
#   $env:REGION = "asia-southeast2"
#   $env:DATABASE_URL = "postgres://..."
#   .\deploy\setup-cloudrun.ps1

[CmdletBinding()]
param(
    [string]$ProjectId = $env:PROJECT_ID,
    [string]$Region = $(if ($env:REGION) { $env:REGION } else { "asia-southeast2" }),
    [string]$DatabaseUrl = $env:DATABASE_URL
)

$ErrorActionPreference = 'Stop'

if (-not $ProjectId) {
    $ProjectId = (& gcloud config get-value project 2>$null)
}
if (-not $ProjectId) {
    throw "Project ID is required. Pass -ProjectId or set `$env:PROJECT_ID."
}

Write-Host "==> Target GCP Project: $ProjectId ($Region)" -ForegroundColor Cyan

Write-Host "==> Enabling required GCP APIs..." -ForegroundColor Cyan
& gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com --project $ProjectId --quiet
if ($LASTEXITCODE -ne 0) { throw "Failed to enable GCP services." }

$repoName = "fleetflow"
$serviceName = "fleetflow"
$image = "$Region-docker.pkg.dev/$ProjectId/$repoName/${serviceName}:latest"

Write-Host "==> Ensuring Artifact Registry repository '$repoName' exists..." -ForegroundColor Cyan
& gcloud artifacts repositories describe $repoName --location $Region --project $ProjectId 2>$null
if ($LASTEXITCODE -ne 0) {
    & gcloud artifacts repositories create $repoName --repository-format=docker --location=$Region --description="FleetFlow container repository" --project=$ProjectId --quiet
}

Write-Host "==> Building and pushing container image ($image)..." -ForegroundColor Cyan
& gcloud builds submit --tag $image --project $ProjectId .
if ($LASTEXITCODE -ne 0) { throw "Cloud Build failed." }

if ($DatabaseUrl) {
    Write-Host "==> Setting up Secret Manager for DATABASE_URL..." -ForegroundColor Cyan
    $secretName = "fleetflow-database-url"
    & gcloud secrets describe $secretName --project $ProjectId 2>$null
    if ($LASTEXITCODE -ne 0) {
        $DatabaseUrl | & gcloud secrets create $secretName --data-file=- --replication-policy="automatic" --project=$ProjectId --quiet
    } else {
        $DatabaseUrl | & gcloud secrets versions add $secretName --data-file=- --project=$ProjectId --quiet
    }
}

Write-Host "==> Deploying Cloud Run service '$serviceName'..." -ForegroundColor Cyan
& gcloud run deploy $serviceName `
    --image $image `
    --region $Region `
    --execution-environment gen2 `
    --cpu 2 `
    --memory 2Gi `
    --no-cpu-throttling `
    --port 8080 `
    --allow-unauthenticated `
    --project $ProjectId `
    --quiet

if ($LASTEXITCODE -ne 0) { throw "Cloud Run deployment failed." }

$serviceUrl = (& gcloud run services describe $serviceName --region $Region --project $ProjectId --format 'value(status.url)').Trim()
Write-Host "==> Deployment successful!" -ForegroundColor Green
Write-Host "==> Service URL: $serviceUrl" -ForegroundColor Green
Write-Host "==> Health check: $serviceUrl/healthz"
Write-Host "==> Readiness check: $serviceUrl/readyz"
