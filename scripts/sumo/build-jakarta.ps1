param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dataDir = Join-Path $root 'data'
$sourceFile = Join-Path $dataDir 'osrm\java-latest.osm.pbf'
$networkDir = Join-Path $dataDir 'networks\generated'
$extractFile = Join-Path $networkDir 'jakarta-dki.osm.pbf'
$osmFile = Join-Path $networkDir 'jakarta-dki.osm'
$networkFile = Join-Path $networkDir 'jakarta-dki.net.xml'
$metadataFile = Join-Path $networkDir 'jakarta-dki.network.json'
$dockerfile = Join-Path $root 'scripts\osrm\osmium.Dockerfile'
$osmiumImage = 'fleetflow/osmium:local'
$dataMount = "${dataDir}:/data"
$bbox = '106.25,-6.45,107.05,-4.95'
$sourceUrl = 'https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf'

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker Desktop is required to extract the Jakarta OSM network.'
}

$netconvert = Get-Command netconvert.exe -ErrorAction SilentlyContinue
$sumo = Get-Command sumo.exe -ErrorAction SilentlyContinue
if (-not $netconvert -or -not $sumo) {
    throw 'SUMO netconvert.exe and sumo.exe must be installed and available on PATH.'
}

$previousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
& docker info --format '{{.ServerVersion}}' 2>&1 | Out-Null
$dockerExitCode = $LASTEXITCODE
$ErrorActionPreference = $previousErrorActionPreference
if ($dockerExitCode -ne 0) {
    throw 'Docker Desktop must be running to extract the OSM PBF.'
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

if ($Force -or -not (Test-Path $networkFile)) {
    & docker build -f $dockerfile -t $osmiumImage (Join-Path $root 'scripts\osrm')
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the Osmium extraction image.' }

    & docker run --rm -v $dataMount $osmiumImage extract `
        --strategy=complete_ways --bbox $bbox `
        --output "/data/networks/generated/jakarta-dki.osm.pbf" `
        "/data/osrm/java-latest.osm.pbf"
    if ($LASTEXITCODE -ne 0) { throw 'Could not extract the Jakarta OSM network.' }

    & docker run --rm -v $dataMount $osmiumImage cat `
        --output-format=xml `
        --output "/data/networks/generated/jakarta-dki.osm" `
        "/data/networks/generated/jakarta-dki.osm.pbf"
    if ($LASTEXITCODE -ne 0) { throw 'Could not convert the extracted PBF to OSM XML.' }

    & $netconvert.Source --osm-files $osmFile --output-file $networkFile `
        --geometry.remove --junctions.join --ramps.guess --tls.guess
    if ($LASTEXITCODE -ne 0) { throw 'SUMO netconvert could not build the Jakarta network.' }
}

if (-not (Test-Path $networkFile) -or (Get-Item $networkFile).Length -eq 0) {
    throw 'The generated SUMO network is missing or empty.'
}

& $sumo.Source --net-file $networkFile --begin 0 --end 1 --no-step-log true --duration-log.disable true
if ($LASTEXITCODE -ne 0) { throw 'SUMO could not load the generated network.' }

$version = (& $sumo.Source --version 2>&1 | Select-Object -First 1).ToString()
$metadata = [ordered]@{
    source = $sourceUrl
    source_region = 'Geofabrik Java extract'
    osm_boundary = 'OpenStreetMap relation 6362934 (Daerah Khusus Ibukota Jakarta)'
    osm_boundary_bounds = '106.3146732,-6.3744575,106.9739750,-4.9993635'
    extract_bbox = $bbox
    generated_at_utc = [DateTime]::UtcNow.ToString('o')
    sumo_version = $version
    network_file = 'jakarta-dki.net.xml'
}
$metadata | ConvertTo-Json | Set-Content -LiteralPath $metadataFile -Encoding utf8

Write-Output "Generated and validated $networkFile"
Write-Output "Network extent includes the OSM DKI Jakarta administrative bounds; the extract also includes surrounding road context."
Write-Output "SUMO network metadata: $metadataFile"
