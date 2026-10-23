param(
    [switch]$Force,
    [switch]$ValidateWithSumo
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dataDir = Join-Path $root 'data'
$sourceFile = Join-Path $dataDir 'osrm\java-latest.osm.pbf'
$networkDir = Join-Path $dataDir 'networks\generated'
$boundaryFile = Join-Path $networkDir 'dki-jakarta.geojson'
$osmFile = Join-Path $networkDir 'jakarta-dki.osm'
$networkFile = Join-Path $networkDir 'jakarta-dki.net.xml'
$metadataFile = Join-Path $networkDir 'jakarta-dki.network.json'
$extractor = Join-Path $PSScriptRoot 'extract_osm_roads.py'
$validator = Join-Path $PSScriptRoot 'validate_sumo_net.py'
$sourceUrl = 'https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf'
$boundaryUrl = 'https://nominatim.openstreetmap.org/lookup?osm_ids=R6362934&format=geojson&polygon_geojson=1'

$python = Get-Command python.exe -ErrorAction SilentlyContinue
$netconvert = Get-Command netconvert.exe -ErrorAction SilentlyContinue
if (-not $python -or -not $netconvert) {
    throw 'Python and SUMO netconvert.exe must be installed and available on PATH.'
}
if ($ValidateWithSumo) {
    $sumo = Get-Command sumo.exe -ErrorAction SilentlyContinue
    if (-not $sumo) { throw 'sumo.exe must be installed and available on PATH.' }
}

New-Item -ItemType Directory -Force -Path (Split-Path $sourceFile), $networkDir | Out-Null
if (-not (Test-Path $sourceFile)) {
    $temporaryFile = "$sourceFile.download"
    try {
        Invoke-WebRequest -Uri $sourceUrl -OutFile $temporaryFile
        Move-Item -Force $temporaryFile $sourceFile
    } finally {
        if (Test-Path $temporaryFile) { Remove-Item -LiteralPath $temporaryFile -Force }
    }
}

if (-not (Test-Path $boundaryFile)) {
    $temporaryBoundary = "$boundaryFile.download"
    try {
        Invoke-WebRequest -Uri $boundaryUrl -Headers @{ 'User-Agent' = 'FleetFlow local network builder' } `
            -OutFile $temporaryBoundary
        $boundary = Get-Content -LiteralPath $temporaryBoundary -Raw | ConvertFrom-Json
        $relation = @($boundary.features | Where-Object {
            $_.properties.osm_type -eq 'relation' -and $_.properties.osm_id -eq 6362934
        })
        if ($relation.Count -ne 1) {
            throw 'The downloaded GeoJSON does not contain the DKI Jakarta OSM boundary relation.'
        }
        Move-Item -Force $temporaryBoundary $boundaryFile
    } finally {
        if (Test-Path $temporaryBoundary) { Remove-Item -LiteralPath $temporaryBoundary -Force }
    }
}

if ($Force -or -not (Test-Path $networkFile)) {
    & $python.Source $extractor $sourceFile $osmFile `
        --boundary-geojson $boundaryFile --buffer-degrees 0.02
    if ($LASTEXITCODE -ne 0) { throw 'Could not extract the Jakarta OSM road network.' }

    & $netconvert.Source --osm-files $osmFile --output-file $networkFile `
        --geometry.remove --junctions.join --ramps.guess --tls.guess
    if ($LASTEXITCODE -ne 0) { throw 'SUMO netconvert could not build the Jakarta network.' }
}

if (-not (Test-Path $networkFile) -or (Get-Item $networkFile).Length -eq 0) {
    throw 'The generated SUMO network is missing or empty.'
}

& $python.Source $validator $networkFile
if ($LASTEXITCODE -ne 0) { throw 'The generated SUMO network failed structural validation.' }
if ($ValidateWithSumo) {
    & $sumo.Source --net-file $networkFile --begin 0 --end 1 --no-step-log true --duration-log.disable true
    if ($LASTEXITCODE -ne 0) { throw 'SUMO could not load the generated network.' }
}

$version = (& $netconvert.Source --version 2>&1 | Select-Object -First 1).ToString()
$metadata = [ordered]@{
    source = $sourceUrl
    source_region = 'Geofabrik Java extract'
    osm_boundary = 'OpenStreetMap relation 6362934 (Daerah Khusus Ibukota Jakarta)'
    osm_boundary_bounds = '106.3146732,-6.3744575,106.9739750,-4.9993635'
    boundary_buffer_degrees = 0.02
    generated_at_utc = [DateTime]::UtcNow.ToString('o')
    netconvert_version = $version
    sumo_load_test = [bool]$ValidateWithSumo
    network_file = 'jakarta-dki.net.xml'
}
$metadata | ConvertTo-Json | Set-Content -LiteralPath $metadataFile -Encoding utf8

Write-Output "Generated and validated $networkFile"
Write-Output "Network extent includes the OSM DKI Jakarta administrative bounds; the extract also includes surrounding road context."
Write-Output "SUMO network metadata: $metadataFile"
