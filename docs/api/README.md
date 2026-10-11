# GoChat API contracts

The canonical HTTP contract is [openapi.json](openapi.json), OpenAPI 3.0.3. It is embedded in the server and available at `GET /openapi.json`; no CDN or external documentation service is required. Import the file into an OpenAPI viewer or client generator. The document includes both builtin and external authentication APIs. The server registers only the configured mode, so requests to the other mode return 404.

All protected HTTP APIs require `Authorization: Bearer <GoChat JWT>`. External authentication uses `POST /api/auth` with the external application's token in `Authorization`; its upstream format is passed unchanged. Successful API JSON contains `code: 0`, `message`, and typed `data`. Errors contain a nonzero code, a safe message and `data: null`. File downloads, range errors, metrics and the OpenAPI document retain their native formats. Health responses retain additional top-level status fields. See [runtime operations](../operations/runtime.md) for error codes, readiness and metrics.

Pagination uses `limit` (1–100, default 50) and a nonnegative item `offset`. Group administration/member/receipt/search APIs cap offset at 1,000,000. User/group IDs are JSON integers. History enrichment also returns numeric message IDs; conversation `last_message.id`, unread `latest_message_id`, and group-member `last_read_message_id` are decimal strings. Use an int64-capable decoder for numeric IDs; JavaScript Number cannot represent every valid ID exactly. HTTP `/api/message/read` accepts `message_id` as either an exact decimal string or int64 JSON integer. WS ACKs retain numeric `message_id` for compatibility and add `message_id_string` for exact decoding in JavaScript.

The route contract test constructs routers in both authentication modes and checks every registered method/path, path parameters, reference resolution, authentication declarations and standard error-envelope schemas. It also fetches the public document over HTTP. The checked-in document was validated against the [official OpenAPI 3.0 schema](https://spec.openapis.org/oas/3.0/schema/2024-10-18); runtime tests are offline.

## WebSocket contract

WebSocket traffic uses the `server.connect_addr` listener, `GET /ws`. Its standalone liveness endpoint is `GET /health`. Authenticate the upgrade with `Authorization: Bearer <GoChat JWT>`; a `token` query parameter is supported for compatible clients. Prefer the header to keep tokens out of proxy URL logs. An invalid/missing token returns HTTP 401 with the error envelope before upgrading. A new connection for the same user replaces the old connection. No in-band `auth` command is required.

Frames are JSON objects with `cmd`, optional client `seq`, and command-specific `data`. The server registers `msg`, `msg_read` and `ping`; other command constants do not imply supported handlers. The maximum incoming frame is 4096 bytes, and excess traffic above 30 frames/second per connection is dropped.

| Direction / cmd | Payload and behavior |
| --- | --- |
| Client `ping` | No data required. Server returns `pong`. |
| Client `msg` | `{from_id, to_id, group_id, type, content}`. `from_id` is a positive numeric compatibility field; authenticated identity determines the persisted sender. Set exactly one positive target: `to_id` or `group_id`. `type` is text/image/file/voice/video/mention. Nonempty content is text or JSON-encoded FileMeta. Group sends require current, unmuted membership. |
| Server `msg_reply` | Same `seq`; `{status:"success", message_id:<JSON integer>, message_id_string:"<exact decimal ID>", duplicate:<boolean>}`. Sent after the message and unread summaries persist atomically. |
| Server `msg` | Chat frame forwarded to online peers/current group members. Sender's client `seq` is preserved. |
| Client `msg_read` | `{message_id:"<decimal ID>"}`. This input ID must be a JSON string. Private receipts require the recipient; group receipts require current membership and advance the member's read watermark. |
| Server `msg_read_reply` | Same `seq`; `{status:"success", message_id:<JSON integer>, message_id_string:"<exact decimal ID>"}`. |
| Server `error` | Same `seq` when available; `{code:<business code>, message:<safe message>}`. No HTTP envelope is added inside WS data. |
| Server `offline_notify` | `{user_id:<string>, total_unread_count:<integer>, unread_list:[{from_id,group_id,count,latest_message?}]}`. Latest message `{id:<decimal string>,sender_id,type,preview,created_at?}`. Sent on connection when unread summaries exist. |

For message retries, preserve the original nonempty `seq` (maximum 128 characters). The sender/seq pair is unique: a retry returns the existing message ID with `duplicate:true` without rewriting unread counts. Empty seq opts out of idempotency. SDK `Send` does not replay failed writes; `Receive` reconnects on unexpected disconnects and refreshes the token through the configured callback. Reconnect does not resend unacknowledged messages.

Transport ping/pong runs independently of JSON `ping`: the server sends a transport ping every 54 seconds and uses a 90-second read deadline renewed by transport pong. Write deadlines are 10 seconds. A malformed JSON frame is logged without raw content and ignored; not every malformed frame receives an error reply.

## Maintaining the contract

When adding a route, update the matching OpenAPI operation and schemas, then run:

```powershell
go test ./internal/api -run TestOpenAPI -count=1
```

Document actual wire types rather than inferring them from SDK names. Keep binary/range responses separate from JSON API responses. New request fields that use byte limits should describe those limits explicitly; `x-max-utf8-bytes` records byte-based bounds that OpenAPI's character-count `maxLength` alone cannot express.

Media operations require ffprobe/ffmpeg for audio/video metadata, WebP thumbnails and video conversion. Images are limited to 20 million pixels; audio/video duration is at most 3600 seconds. Uploaded videos become MP4; output at or above 1 GiB is rejected. Upload sessions expire after 24 hours; completed assets expire only when `expires_hours` is set. Run `im-maintenance` periodically with the same config and upload volume. Its optional `-lifecycle-days` applies staging/incomplete-multipart cleanup rules; zero leaves bucket lifecycle configuration unchanged. Lifecycle configuration is an explicit maintenance action rather than an automatic startup requirement.
