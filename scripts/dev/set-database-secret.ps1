$ErrorActionPreference = 'Stop'

$project = 'fleetflow-4448'
$secretName = 'fleetflow-database-url'
$placeholder = '[YOUR-PASSWORD]'

$template = Read-Host "Supabase URL with $placeholder or an empty password (username:@host)"
if ($template -match '[\r\n]') {
  throw 'The URL must be a single line. No secret was sent.'
}

$match = [regex]::Match(
  $template,
  '^(?<prefix>postgres(?:ql)?://[^:/@]+:)(?<password>[^@]*)(?<suffix>@[^/\r\n]+/[^?\r\n]+(?:\?.*)?)$'
)
if (-not $match.Success) {
  throw 'Enter a PostgreSQL URL with a username and host. No secret was sent.'
}

$templatePassword = $match.Groups['password'].Value
if ($templatePassword -and $templatePassword -cne $placeholder) {
  throw "Leave the URL password empty or use the literal $placeholder. Do not put the password in the URL prompt."
}
$template = $match.Groups['prefix'].Value + $placeholder + $match.Groups['suffix'].Value

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
