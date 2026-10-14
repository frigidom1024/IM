# Changelog

## Unreleased

- Change `Receive()` to a continuous `<-chan WSMessage` stream, driven by a single background reader started at connection time.
- Add `Errors()` for receive/decode/reconnect diagnostics; close both streams and stop I/O on `Close()` or terminal receive failure.
- Preserve read error causes, avoid reconnecting on malformed JSON frames, and bound repeated immediate disconnects with backoff.
- Rename the internal connection accessor to `currentConnection()` and keep dialing in explicit connection/reconnect paths.

- Replace the WebSocket three-ID chat payload with `conversation_id`; change `SendChat` to `(conversationID, content, messageType)`.
- Add recipient-scoped conversation IDs and authenticated string sender IDs to deliveries, and sender-scoped IDs to ACKs.
- Remove server offline notification pushes; clients query notification and conversation HTTP endpoints.
- Decode ACK message IDs from exact string IDs and reject conflicting idempotency retries on the server.

- Move history and unread/read APIs to viewer-owned conversation endpoints; `MarkConversationRead(ctx, scope)` clears the whole conversation without a cursor.
- Remove group message receipts and read cursor fields; add canonical scope IDs to unread summaries.
- Keep ID-based private/group history and read helpers forwarding to conversation HTTP APIs.

- Send automatic WebSocket protocol heartbeats every 30 seconds, with a configurable interval and opt-out.
- Continue heartbeats across reconnects, stop them on close, and coalesce concurrent reconnects for the same failed connection.

## 0.2.0 — 2026-10-11

- Add configurable HTTP timeout, custom transport, opt-in bounded GET retries and sanitized request logging.
- Add per-user serialized token refresh before authenticated HTTP calls and WebSocket handshakes/reconnects.
- Serialize WebSocket sends/receives and cancel pending reconnects on close.
- Stream multipart upload bodies with file progress callbacks and safely encoded filenames.
- Add typed conversation preferences, group management, receipts and paginated group-file APIs.
- Add resumable seekable-file upload with persisted progress and session-preserving errors, plus authenticated streaming downloads/thumbnails.
- Decode both legacy numeric and lossless string message IDs.
- Add net/http and Gin external-auth adapters and a standalone SDK integration consumer.
- Preserve APIError, existing client methods and no automatic replay of HTTP writes or WebSocket messages.

## 0.1.0

Initial typed HTTP/WebSocket client and external-user authentication adapter.
