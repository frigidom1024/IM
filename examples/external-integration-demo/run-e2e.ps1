param(
  [switch]$KeepData
)

$dockerServer = docker info --format '{{.ServerVersion}}' 2>$null
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($dockerServer)) {
  $context = docker context show 2>$null
  Write-Error @"
Docker Engine is unavailable, so the integration test cannot start.
Current Docker context: $context
Start Docker Desktop, wait for the Linux Engine to become ready, then retry:
  docker info
"@
  exit 1
}
$compose = Join-Path $PSScriptRoot 'compose.yaml'

Write-Host 'Starting the external integration demo...' -ForegroundColor Cyan
docker compose -f $compose up -d --build
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}

try {
  node (Join-Path $PSScriptRoot 'e2e.mjs')
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
  node (Join-Path $PSScriptRoot 'p1-e2e.mjs')
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  $sdkEnvironment = @{}
  foreach ($key in @('GOCHAT_URL', 'GOCHAT_WS_URL', 'GOCHAT_EXTERNAL_TOKEN')) {
    $sdkEnvironment[$key] = [Environment]::GetEnvironmentVariable($key)
  }
  Push-Location (Join-Path $PSScriptRoot '../sdk-integration')
  try {
    $env:GOCHAT_URL = 'http://localhost:18080'
    $env:GOCHAT_WS_URL = 'ws://localhost:17000/ws'
    $env:GOCHAT_EXTERNAL_TOKEN = 'demo-alice'
    go test -race -v ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  } finally {
    Pop-Location
    foreach ($key in $sdkEnvironment.Keys) {
      [Environment]::SetEnvironmentVariable($key, $sdkEnvironment[$key])
    }
  }
} finally {
  if ($KeepData) {
    docker compose -f $compose down
  } else {
    docker compose -f $compose down -v
  }
}
