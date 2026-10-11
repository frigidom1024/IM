# Standalone SDK integration consumer

This is a separate Go module importing only the public `gochat/sdk` package.
The local `replace` points to the checkout; replace it with your private proxy release for deployment.

Start the Docker GoChat stack and register a builtin test user first. Run:

```powershell
$env:GOCHAT_URL="http://localhost:8080"
$env:GOCHAT_WS_URL="ws://localhost:8081/ws"
$env:GOCHAT_TEST_USERNAME="sdk-test"
$env:GOCHAT_TEST_PASSWORD="your-test-password"
go test -v ./...
```

Without `GOCHAT_URL`, the live integration test skips explicitly. A skip does not verify a running GoChat server. The root `go test ./sdk` suite additionally exercises HTTP and real WebSocket connections with local test servers.
