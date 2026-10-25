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
    @{ name = 'B-9001-JKT'; registration = 'B-9001-JKT'; make = 'Isuzu'; model = 'Giga FVM Heavy Cargo'; type = 'truck'; status = 'available'; body_type = 'Heavy Cargo Box'; capacity = 14000; year = 2022; color = 'White' },
    @{ name = 'B-9002-JKT'; registration = 'B-9002-JKT'; make = 'Mitsubishi'; model = 'Fuso Canter 136 HDL'; type = 'truck'; status = 'available'; body_type = 'Box Truck'; capacity = 4500; year = 2023; color = 'Yellow' },
    @{ name = 'B-9003-JKT'; registration = 'B-9003-JKT'; make = 'Toyota'; model = 'HiAce Commuter Cargo'; type = 'van'; status = 'available'; body_type = 'Van'; capacity = 1200; year = 2023; color = 'Silver' },
    @{ name = 'B-9004-JKT'; registration = 'B-9004-JKT'; make = 'Daihatsu'; model = 'Gran Max Blind Van'; type = 'van'; status = 'available'; body_type = 'Blind Van'; capacity = 800; year = 2024; color = 'White' },
    @{ name = 'B-9005-JKT'; registration = 'B-9005-JKT'; make = 'Honda'; model = 'PCX Electric Cargo'; type = 'motorcycle'; status = 'available'; body_type = 'Motorcycle Cargo'; capacity = 60; year = 2024; color = 'Blue' },
    @{ name = 'B-9006-JKT'; registration = 'B-9006-JKT'; make = 'Hino'; model = 'Dutro 130 HD Cargo'; type = 'truck'; status = 'available'; body_type = 'Box Truck'; capacity = 4500; year = 2023; color = 'Green' },
    @{ name = 'B-9007-JKT'; registration = 'B-9007-JKT'; make = 'Wuling'; model = 'Formo Blind Van'; type = 'van'; status = 'available'; body_type = 'Blind Van'; capacity = 650; year = 2023; color = 'White' }
)
foreach ($seed in $vehicleSeeds) {
    $vehicle = Get-OrCreateRecord -Records $vehicles -Field 'name' -Value $seed.name `
        -Path '/api/vehicles' -Body $seed
    $vehicles += $vehicle
}

$driverData = Invoke-FleetFlowApi -Method 'Get' -Path '/api/drivers' -Body $null
$drivers = @($driverData.drivers)
$driverSeeds = @(
    @{ name = 'Budi Santoso'; contact = '+62 812-3456-7890'; contact_phone = '+62 812-3456-7890'; license_class = 'B2 Umum'; shift_start = '07:00'; shift_end = '16:00'; availability = 'available'; contract_type = 'Full-time' },
    @{ name = 'Agus Setiawan'; contact = '+62 813-9876-5432'; contact_phone = '+62 813-9876-5432'; license_class = 'B1'; shift_start = '08:00'; shift_end = '17:00'; availability = 'available'; contract_type = 'Contract' },
    @{ name = 'Rian Hidayat'; contact = '+62 815-2233-4455'; contact_phone = '+62 815-2233-4455'; license_class = 'A'; shift_start = '08:00'; shift_end = '17:00'; availability = 'available'; contract_type = 'Full-time' },
    @{ name = 'Eko Prasetyo'; contact = '+62 817-6677-8899'; contact_phone = '+62 817-6677-8899'; license_class = 'A'; shift_start = '06:00'; shift_end = '15:00'; availability = 'available'; contract_type = 'Contract' },
    @{ name = 'Dimas Pratama'; contact = '+62 819-3344-5566'; contact_phone = '+62 819-3344-5566'; license_class = 'C'; shift_start = '09:00'; shift_end = '18:00'; availability = 'available'; contract_type = 'Freelance' }
)
foreach ($seed in $driverSeeds) {
    $driver = Get-OrCreateRecord -Records $drivers -Field 'name' -Value $seed.name `
        -Path '/api/drivers' -Body $seed
    $drivers += $driver
}

# Give each seeded driver the vehicle at the same position, so the register shows
# who is on what. Allocation belongs to the vehicle record, so the request is
# made against the driver and the vehicle it is handed.
$allocationCount = [Math]::Min($driverSeeds.Count, $vehicleSeeds.Count)
for ($i = 0; $i -lt $allocationCount; $i++) {
    $driver = $drivers[$i]
    $vehicle = $vehicles[$i]
    if ($null -eq $driver -or $null -eq $vehicle) { continue }
    Invoke-FleetFlowApi -Method 'Put' -Path "/api/drivers/$($driver.id)/vehicle" -Body @{ vehicle_id = $vehicle.id } | Out-Null
}

Write-Output "Seeded realistic fleet records at $BaseUrl."
Write-Output "Created or verified $($vehicleSeeds.Count) fleet vehicles with cargo capacity and $($driverSeeds.Count) drivers with contract types."
Write-Output 'No GPS positions were generated. DEMO records are sample data, not live fleet activity.'
