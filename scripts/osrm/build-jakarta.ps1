param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dataDir = Join-Path $root 'data\osrm'
$sourceFile = Join-Path $dataDir 'java-latest.osm.pbf'
$extractFile = Join-Path $dataDir 'jakarta-metro.osm.pbf'
$osrmFile = Join-Path $dataDir 'jakarta-metro.osrm'
$mount = "${dataDir}:/data"
$osrmImage = 'osrm/osrm-backend:v5.25.0'

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker Desktop is required to download, extract, and prepare the Jakarta routing data.'
}

$previousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
& docker info --format '{{.ServerVersion}}' 2>&1 | Out-Null
$dockerExitCode = $LASTEXITCODE
$ErrorActionPreference = $previousErrorActionPreference
if ($dockerExitCode -ne 0) {
    throw 'Docker Desktop must be running before building the Jakarta routing data.'
}

New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

if (-not (Test-Path $sourceFile)) {
    $temporaryFile = "$sourceFile.download"
    try {
        Invoke-WebRequest -Uri 'https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf' -OutFile $temporaryFile
        Move-Item -Force $temporaryFile $sourceFile
    } finally {
        if (Test-Path $temporaryFile) { Remove-Item -Force $temporaryFile }
    }
}

if ($Force -or -not (Test-Path $extractFile)) {
    & docker build -f (Join-Path $root 'scripts\osrm\osmium.Dockerfile') -t fleetflow/osmium:local $root
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the Osmium extraction image.' }

    & docker run --rm -v $mount fleetflow/osmium:local extract `
        --bbox 106.2,-6.9,107.3,-5.7 `
        --output /data/jakarta-metro.osm.pbf /data/java-latest.osm.pbf
    if ($LASTEXITCODE -ne 0) { throw 'Could not extract the Jakarta metropolitan OSM network.' }
}

if ($Force -or -not (Test-Path $osrmFile)) {
    & docker run --rm -v $mount $osrmImage osrm-extract -p /opt/car.lua /data/jakarta-metro.osm.pbf
    if ($LASTEXITCODE -ne 0) { throw 'OSRM could not extract the driving network.' }

    & docker run --rm -v $mount $osrmImage osrm-partition /data/jakarta-metro.osrm
    if ($LASTEXITCODE -ne 0) { throw 'OSRM could not partition the driving network.' }

    & docker run --rm -v $mount $osrmImage osrm-customize /data/jakarta-metro.osrm
    if ($LASTEXITCODE -ne 0) { throw 'OSRM could not customize the driving network.' }
}

Write-Output 'Jakarta OSRM data is ready. Start it with: docker compose --profile routing up -d osrm'
