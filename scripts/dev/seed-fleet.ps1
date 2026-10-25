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

# The fleet itself belongs to the scenarios: a completed run registers the
# vehicles it drove, with their profiles. Seeding a second fleet here would give
# the console two of everything, so this script only adds people and allocations.
# It reads the fleet, it does not create it.
$vehicleData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/vehicles' -Body $null
$vehicles = @($vehicleData.vehicles)

$driverData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/drivers' -Body $null
$drivers = @($driverData.drivers)
$driverSeeds = @(
    @{ name = 'Budi Santoso'; contact = '+62 812-3456-7890'; contact_phone = '+62 812-3456-7890'; license_class = 'B2 Umum'; shift_start = '07:00'; shift_end = '16:00'; availability = 'available'; contract_type = 'Full-time' },
    @{ name = 'Agus Setiawan'; contact = '+62 813-9876-5432'; contact_phone = '+62 813-9876-5432'; license_class = 'B1'; shift_start = '08:00'; shift_end = '17:00'; availability = 'available'; contract_type = 'Contract' },
    @{ name = 'Rian Hidayat'; contact = '+62 815-2233-4455'; contact_phone = '+62 815-2233-4455'; license_class = 'A'; shift_start = '08:00'; shift_end = '17:00'; availability = 'available'; contract_type = 'Full-time' },
    @{ name = 'Eko Prasetyo'; contact = '+62 817-6677-8899'; contact_phone = '+62 817-6677-8899'; license_class = 'A'; shift_start = '06:00'; shift_end = '15:00'; availability = 'available'; contract_type = 'Contract' },
    @{ name = 'Dimas Pratama'; contact = '+62 819-3344-5566'; contact_phone = '+62 819-3344-5566'; license_class = 'C'; shift_start = '09:00'; shift_end = '18:00'; availability = 'available'; contract_type = 'Freelance' }
)
$seededDrivers = @()
foreach ($seed in $driverSeeds) {
    $seededDrivers += Get-OrCreateRecord -Records $drivers -Field 'name' -Value $seed.name `
        -Path '/api/drivers' -Body $seed
}

# Put each seeded driver on a registered vehicle so the register shows who is on
# what. A driver who already holds a vehicle keeps it, and a vehicle someone else
# holds is not handed out twice: the seeder makes the demo pairing once, it does
# not reassign work the operator has moved.
$held = @($drivers | Where-Object { $_.vehicle_id } | ForEach-Object { $_.vehicle_id })
$freeVehicles = @($vehicles | Where-Object { $held -notcontains $_.id } | Sort-Object registration, name)
$waiting = @($seededDrivers | Where-Object { -not $_.vehicle_id })
$allocationCount = [Math]::Min($waiting.Count, $freeVehicles.Count)
for ($i = 0; $i -lt $allocationCount; $i++) {
    Invoke-FleetFlowApi -Method 'Put' -Path "/api/drivers/$($waiting[$i].id)/vehicle" -Body @{ vehicle_id = $freeVehicles[$i].id } | Out-Null
}

if ($vehicles.Count -eq 0) {
    Write-Output 'No vehicles are registered yet, so no driver was allocated. Start a scenario to register the fleet, then run this again.'
}

Write-Output "Seeded fleet people at $BaseUrl."
Write-Output "Created or verified $($driverSeeds.Count) drivers and allocated $allocationCount of them to registered vehicles."
Write-Output 'The fleet itself is registered by completed scenarios. No GPS positions were generated.'
