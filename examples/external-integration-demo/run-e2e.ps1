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
} finally {
  if ($KeepData) {
    docker compose -f $compose down
  } else {
    docker compose -f $compose down -v
  }
}
