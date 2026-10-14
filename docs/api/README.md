# GoChat API contracts

The canonical HTTP contract is [openapi.json](openapi.json), OpenAPI 3.0.3. It is embedded in the server and available at `GET /openapi.json`; no CDN or external documentation service is required. Import the file into an OpenAPI viewer or client generator. The document includes both builtin and external authentication APIs. The server registers only the configured mode, so requests to the other mode return 404.

All protected HTTP APIs require `Authorization: Bearer <GoChat JWT>`. External authentication uses `POST /api/auth` with the external application's token in `Authorization`; its upstream format is passed unchanged. Successful API JSON contains `code: 0`, `message`, and typed `data`. Errors contain a nonzero code, a safe message and `data: null`. File downloads, range errors, metrics and the OpenAPI document retain their native formats. Health responses retain additional top-level status fields. See [runtime operations](../operations/runtime.md) for error codes, readiness and metrics.

Pagination uses `limit` (1–100, default 50) and a nonnegative item `offset`. Group administration/member/search APIs cap offset at 1,000,000. User/group IDs are JSON integers. History enrichment also returns numeric message IDs; conversation `last_message.id`, unread `latest_message_id`, are decimal strings. Use an int64-capable decoder for numeric IDs; JavaScript Number cannot represent every valid ID exactly. WS ACKs retain numeric `message_id` for compatibility and add `message_id_string` for exact decoding in JavaScript.

The route contract test constructs routers in both authentication modes and checks every registered method/path, path parameters, reference resolution, authentication declarations and standard error-envelope schemas. It also fetches the public document over HTTP. The checked-in document was validated against the [official OpenAPI 3.0 schema](https://spec.openapis.org/oas/3.0/schema/2024-10-18); runtime tests are offline.

## WebSocket contract

The UI separates messages (private and group conversations), friends (directed relationships and notices), and groups (membership/applications). Adding a friend or joining an empty group does not create a conversation; receiving/sending messages or `POST /api/conversation/:scope/open` does. Private `send_policy` values are `unavailable`, `text_once`, `unrestricted`. Single-direction friends may send exactly one text until mutual; removing/readding/revoking does not reset it. Friend notices are distinct from message unread counts.

`POST /api/group/:groupId/join` returns a pending application, never immediate membership. Owner/admin invites still add members directly. Application states are `pending`, `approved`, `rejected`, `closed`; the current owner reviews with `{decision:"approve"|"reject", reason?:string}`. A rejected applicant may reapply; duplicate pending submissions return the existing application. Full groups reject approval with409 without consuming the pending request. Group transfer moves review authority and dissolution closes pending requests.

Server invalidation frames `friend_changed` and `group_changed` tell clients to reload relationships, notices, applications and conversations. Delivery is best-effort; durable HTTP state recovers notifications after reconnect. Delivered `msg` data additionally includes the persisted decimal-string `id` and `created_at`; `conversation_id` uses the recipient's viewer scope and `sender_id` is the authenticated author's decimal string. History includes exact `id_string` alongside its legacy numeric `id`.

WebSocket traffic uses the `server.connect_addr` listener, `GET /ws`. Its standalone liveness endpoint is `GET /health`. Authenticate the upgrade with `Authorization: Bearer <GoChat JWT>`; a `token` query parameter is supported for compatible clients. Prefer the header to keep tokens out of proxy URL logs. An invalid/missing token returns HTTP 401 with the error envelope before upgrading. A new connection for the same user replaces the old connection. No in-band `auth` command is required.

Frames are JSON objects with `cmd`, optional client `seq`, and command-specific `data`. The server registers `msg` and `ping`; other command constants do not imply supported handlers. The maximum incoming frame is 4096 bytes, and excess traffic above 30 frames/second per connection is dropped.

| Direction / cmd | Payload and behavior |
| --- | --- |
| Client `ping` | No data required. Server returns `pong`. |
| Client `msg` | `{conversation_id, type, content}`. Scope is `private:<peer>` or `group:<group>`; sender comes from the authenticated connection. Type is text/image/file/voice/video/mention, content is nonempty text or JSON-encoded FileMeta. Group sends require current, unmuted membership. |
| Server `msg_reply` | Same `seq`; `{status:"success", conversation_id:<sender scope>, message_id:<JSON integer>, message_id_string:"<exact decimal ID>", duplicate:<boolean>}`. Sent after the message and unread summaries persist atomically. |
| Server `msg` | `{id:<decimal string>,conversation_id:<recipient scope>,sender_id:<decimal string>,type,content,created_at}`. Private scope changes from `private:B` for sender A to `private:A` for recipient B; group scope stays `group:G`. Sender's client seq is preserved. |
| Server `error` | Same `seq` when available; `{code:<business code>, message:<safe message>}`. No HTTP envelope is added inside WS data. |

Connections do not push offline notifications. Clients actively query `/api/conversation`, `/api/conversation/unread`, friend notices and group applications on startup/reconnect and as needed. The frontend reloads active history before marking missed messages read. See the [WS protocol](../websocket.md).

For message retries, preserve the original nonempty `seq` (maximum 128 characters). The sender/seq pair is unique: a retry returns the existing message ID with `duplicate:true` without rewriting unread counts. Empty seq opts out of idempotency. SDK `Send` does not replay failed writes. The SDK background reader reconnects on unexpected disconnects and refreshes the token through the configured callback. `Receive()` exposes a continuous message channel; `Errors()` reports receive/reconnect diagnostics. Both close on client close or terminal reader failure. Reconnect does not resend unacknowledged messages.

Transport ping/pong runs independently of JSON `ping`: the server sends a transport ping every 54 seconds and uses a 90-second read deadline renewed by transport pong. Write deadlines are 10 seconds. A malformed JSON frame is logged without raw content and ignored; not every malformed frame receives an error reply.

## Conversation ownership

The conversation table's `from_id` is the authenticated owner; private views are `(A,B,0)` and `(B,A,0)`, and a group view is `(A,0,G)`. Public IDs are `private:<peer>` / `group:<group>` and are only meaningful with the authenticated owner.

- `GET /api/conversation` lists that owner's summaries, preferences and unread counts.
- `POST /api/conversation/{scope}/open` explicitly creates an empty view; adding a friend or joining an empty group does not.
- `PATCH /api/conversation/{scope}` updates personal preferences.
- `GET /api/conversation/{scope}/messages` returns authorized history using limit/offset without creating a view.
- `GET /api/conversation/unread` returns totals and entries carrying canonical `id`.
- `POST /api/conversation/{scope}/read` needs no body, clears the whole current conversation and sets `read_at` under a row lock. Messages committed afterwards remain unread.

The `/api/message/read`, `/api/message/unread`, private/group message-history routes and group message receipts are removed. No WS read command is supported. Messages and recipient conversation views commit atomically before an ACK or live delivery; idempotent retries do not increment counts again. Leaving/dissolving a group removes history access, list visibility and unread contributions, while retaining personal preferences.

## Maintaining the contract

When adding a route, update the matching OpenAPI operation and schemas, then run:

```powershell
go test ./internal/api -run TestOpenAPI -count=1
```

Document actual wire types rather than inferring them from SDK names. Keep binary/range responses separate from JSON API responses. New request fields that use byte limits should describe those limits explicitly; `x-max-utf8-bytes` records byte-based bounds that OpenAPI's character-count `maxLength` alone cannot express.

Media operations require ffprobe/ffmpeg for audio/video metadata, WebP thumbnails and video conversion. Images are limited to 20 million pixels; audio/video duration is at most 3600 seconds. Uploaded videos become MP4; output at or above 1 GiB is rejected. Upload sessions expire after 24 hours; completed assets expire only when `expires_hours` is set. Run `im-maintenance` periodically with the same config and upload volume. Its optional `-lifecycle-days` applies staging/incomplete-multipart cleanup rules; zero leaves bucket lifecycle configuration unchanged. Lifecycle configuration is an explicit maintenance action rather than an automatic startup requirement.
