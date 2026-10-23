param(
    [string]$BaseUrl = 'http://127.0.0.1:8080',
    [switch]$AllowRemote
)

$ErrorActionPreference = 'Stop'
if (-not [Uri]::IsWellFormedUriString($BaseUrl, [UriKind]::Absolute)) {
    throw 'BaseUrl must be an absolute HTTP(S) URL.'
}
$baseUri = [Uri]$BaseUrl
if (($baseUri.Scheme -ne 'http' -and $baseUri.Scheme -ne 'https') -or
    $baseUri.AbsolutePath -ne '/' -or $baseUri.Query -or $baseUri.Fragment) {
    throw 'BaseUrl must be an HTTP(S) origin without a path, query, or fragment.'
}
if (-not $baseUri.IsLoopback -and -not $AllowRemote) {
    throw 'Refusing to seed a remote server. Use -AllowRemote only when you intend to write demo records there.'
}
$BaseUrl = $baseUri.GetLeftPart([UriPartial]::Authority).TrimEnd('/')
if ($AllowRemote -and -not $baseUri.IsLoopback) {
    Write-Warning "Remote seeding enabled for $BaseUrl. Confirm this is not production."
}

function Invoke-FleetFlowApi {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
        [object]$Body
    )

    $request = @{
        Uri         = "$BaseUrl$Path"
        Method      = $Method
        ErrorAction = 'Stop'
    }
    if ($null -ne $Body) {
        $request.ContentType = 'application/json'
        $request.Body = ConvertTo-Json -InputObject $Body -Depth 10 -Compress
    }
    Invoke-RestMethod @request
}

function Get-OrCreateRecord {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Records,
        [Parameter(Mandatory = $true)][string]$Field,
        [Parameter(Mandatory = $true)][string]$Value,
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][object]$Body
    )

    $found = @($Records | Where-Object { $_.$Field -eq $Value })
    if ($found.Count -gt 1) {
        throw "Multiple records already match '$Value'; refusing to seed duplicates."
    }
    if ($found.Count -eq 1) {
        return $found[0]
    }

    Invoke-FleetFlowApi -Method 'Post' -Path $Path -Body $Body
}

Invoke-FleetFlowApi -Method 'Get' -Path '/healthz' -Body $null | Out-Null

$vehicleData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/vehicles' -Body $null
$vehicles = @($vehicleData.vehicles)
$vehicleSeeds = @(
    @{ name = 'DEMO - Jakarta Van 01'; type = 'van'; status = 'available' },
    @{ name = 'DEMO - Jakarta Truck 01'; type = 'truck'; status = 'available' },
    @{ name = 'DEMO - Jakarta Car 01'; type = 'car'; status = 'available' }
)
foreach ($seed in $vehicleSeeds) {
    $vehicle = Get-OrCreateRecord -Records $vehicles -Field 'name' -Value $seed.name `
        -Path '/api/vehicles' -Body $seed
    $vehicles += $vehicle
}

$driverData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/drivers' -Body $null
$drivers = @($driverData.drivers)
$driverSeeds = @(
    @{ name = 'DEMO - Driver 01'; contact = 'demo-driver-01@example.invalid' },
    @{ name = 'DEMO - Driver 02'; contact = 'demo-driver-02@example.invalid' }
)
foreach ($seed in $driverSeeds) {
    $driver = Get-OrCreateRecord -Records $drivers -Field 'name' -Value $seed.name `
        -Path '/api/drivers' -Body $seed
    $drivers += $driver
}

$tripData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/trips' -Body $null
$trips = @($tripData.trips)
$tripSeeds = @(
    @{
        origin = 'DEMO - Monas, Jakarta (-6.1754, 106.8272)'
        destination = 'DEMO - Gelora Bung Karno, Jakarta (-6.2186, 106.8025)'
        status = 'planned'
    },
    @{
        origin = 'DEMO - Kota Tua, Jakarta (-6.1352, 106.8133)'
        destination = 'DEMO - Bundaran HI, Jakarta (-6.1945, 106.8229)'
        status = 'planned'
    }
)
foreach ($seed in $tripSeeds) {
    $found = @($trips | Where-Object {
        $_.origin -eq $seed.origin -and $_.destination -eq $seed.destination
    })
    if ($found.Count -gt 1) {
        throw "Multiple trips already match '$($seed.origin)' to '$($seed.destination)'; refusing to seed duplicates."
    }
    if ($found.Count -eq 1) {
        $trip = $found[0]
    } else {
        $trip = Invoke-FleetFlowApi -Method 'Post' -Path '/api/trips' -Body $seed
    }
    $trips += $trip
}

Write-Output "Seeded demo fleet records at $BaseUrl."
Write-Output 'Created or reused 3 demo vehicles, 2 demo drivers, and 2 planned trips.'
Write-Output 'No GPS positions were generated. DEMO records are sample data, not live fleet activity.'
