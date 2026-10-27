$ErrorActionPreference = 'Stop'

$project = 'fleetflow-4448'
$secretName = 'fleetflow-database-url'
$placeholder = '[YOUR-PASSWORD]'

$template = Read-Host "Supabase PostgreSQL URL containing $placeholder"
if ($template -notmatch '^postgres(?:ql)?://.*\[YOUR-PASSWORD\]') {
  throw "Enter a PostgreSQL URL containing $placeholder. No secret was sent."
}
if ($template -match '[\r\n]') {
  throw 'The URL must be a single line. No secret was sent.'
}

$securePassword = Read-Host 'Supabase database password (input is hidden)' -AsSecureString
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
$password = $null
$encodedPassword = $null
$connectionUrl = $null
$process = $null

try {
  $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
  if ([string]::IsNullOrEmpty($password)) {
    throw 'Password is empty. No secret was sent.'
  }

  $encodedPassword = [Uri]::EscapeDataString($password)
  $connectionUrl = $template.Replace($placeholder, $encodedPassword)
  if ($connectionUrl -match '[?&]sslmode=') {
    if ($connectionUrl -notmatch '[?&]sslmode=require(?:&|$)') {
      throw 'The URL has a different sslmode. No secret was sent.'
    }
  } elseif ($connectionUrl.Contains('?')) {
    $connectionUrl += '&sslmode=require'
  } else {
    $connectionUrl += '?sslmode=require'
  }

  $gcloud = (Get-Command gcloud.cmd -ErrorAction Stop).Source
  $commandLine = '/d /s /c ""{0}" secrets versions add {1} --data-file=- --project={2} --quiet"' -f `
    $gcloud, $secretName, $project
  $startInfo = New-Object System.Diagnostics.ProcessStartInfo
  $startInfo.FileName = $env:ComSpec
  $startInfo.Arguments = $commandLine
  $startInfo.UseShellExecute = $false
  $startInfo.RedirectStandardInput = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $startInfo.CreateNoWindow = $true
  $startInfo.EnvironmentVariables['CLOUDSDK_METRICS_REQUEST_ATTRIBUTION'] = 'agent-skills/1.0 (skill:gcloud)'

  $process = New-Object System.Diagnostics.Process
  $process.StartInfo = $startInfo
  if (-not $process.Start()) {
    throw 'Could not start gcloud. No secret was sent.'
  }

  $stdoutTask = $process.StandardOutput.ReadToEndAsync()
  $stderrTask = $process.StandardError.ReadToEndAsync()
  $process.StandardInput.Write($connectionUrl)
  $process.StandardInput.Close()
  $process.WaitForExit()

  if ($process.ExitCode -ne 0) {
    $safeError = $stderrTask.Result.Replace($connectionUrl, '[REDACTED]')
    $safeError = $safeError.Replace($encodedPassword, '[REDACTED]')
    throw "Secret Manager failed to add the version: $safeError"
  }

  Write-Output 'Secret version added. The connection URL was not printed or saved to a file.'
} finally {
  if ($process) {
    $process.Dispose()
  }
  $connectionUrl = $null
  $encodedPassword = $null
  $password = $null
  $securePassword.Dispose()
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
}
