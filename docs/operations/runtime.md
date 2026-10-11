# Runtime health and metrics

The API listener exports `/livez`, `/healthz`, `/readyz`, and `/metrics` without authentication. Keep the metrics endpoint on the trusted application network. The standalone WS listener retains `/health`.

`/livez` and `/healthz` report process liveness. `/readyz` returns 200 only after both listeners are bound and MySQL and the configured MinIO bucket respond successfully. Dependency checks run concurrently with a shared two-second timeout. Failed dependencies return HTTP 503 and code 50300; dependency states appear in `data.dependencies` as `ok`, `unavailable`, or `disabled`, without connection strings or internal errors. Draining resets application readiness.

Redis and etcd are optional in the single-instance deployment. Their configured addresses do not imply they are enabled. Set `redis.enabled: true` or `etcd.enabled: true` to include an authenticated Redis PING or an etcd read in readiness. Enabled Redis needs a host/port; enabled etcd needs endpoints. Clients close during application cleanup.

## Prometheus scraping

`/metrics` uses Prometheus text format, version 0.0.4. Example configuration:

```yaml
scrape_configs:
  - job_name: gochat
    metrics_path: /metrics
    scrape_interval: 15s
    static_configs:
      - targets: ['im-server:8080']
```

Use the API address configured in `server.api_addr`. Metrics are process-local and reset on restart:

| Metric | Meaning |
| --- | --- |
| `gochat_http_requests_total` | HTTP count by method, registered route template and status |
| `gochat_http_request_duration_seconds` | Duration histogram with matching labels |
| `gochat_auth_total` | Authentication results by `http`/`ws` and `success`/`failure` |
| `gochat_upload_failures_total` | Failed upload requests, including rejected requests |
| `gochat_ws_connections` | Active WebSocket connections |
| `gochat_ws_users` | Active unique users, including correct replacement/disconnect accounting |
| `gochat_messages_total` | `persisted` and `persistence_failure` outcomes of message+unread transactions |
| `gochat_db_connections_open`, `_in_use`, `_idle`, `_max_open` | Database pool gauges |
| `gochat_db_wait_total`, `gochat_db_wait_duration_seconds_total` | Database pool wait counters |
| `gochat_redis_connections` | Enabled Redis pool total/idle connection gauges |
| `gochat_redis_pool_total` | Enabled Redis pool hit/miss/timeout counters |

Request paths use route templates such as `/api/user/info/:id`. Unmatched routes share `unmatched`; unsupported methods share `OTHER`. User IDs, filenames, message content, URLs, query strings and tokens never become metric labels. HTTP access logs contain the method, route template, status and duration. Panic logs exclude request headers, query strings, bodies and panic values. WebSocket malformed-message logs exclude raw message content.

Authentication counts include login/register/external-auth outcomes and each protected HTTP token validation. Thus a protected request with a valid token but an application error counts as successful token validation. Duplicate-message ACKs do not increment the persisted counter. Failed attempts to persist count as failures even if a concurrent duplicate is subsequently found and acknowledged.

## Response contracts

API success and error JSON use `{ "code": 0, "message": "ok", "data": ... }`, with `data: null` for errors and empty results. Health responses retain their existing top-level status fields. Metrics, streaming file responses, and HTTP 204 responses retain their native formats. File responses should set `Content-Disposition`, which bypasses JSON envelope normalization.

| HTTP status | General code |
| --- | --- |
| 400 | 40000 |
| 401 | 40100 |
| 403 | 40300 |
| 404 (missing route) | 40400 |
| 405 | 40500 |
| 429 | 42900 |
| 500 (panic/unclassified legacy failure) | 50000 |
| 503 (not ready) | 50300 |

Existing module-specific five-digit business codes remain intact. Legacy HTTP-like codes normalize to five digits. Unclassified legacy `code: 1` failures become a generic 500 response rather than exposing raw database errors. WebSocket handshake authentication failures also return the error envelope. JSON normalization preserves numeric precision, including 64-bit snowflake IDs, and does not buffer streaming downloads.
