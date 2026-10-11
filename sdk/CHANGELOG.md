# Changelog

## 0.2.0 ¡ª 2026-10-11

- Add configurable HTTP timeout, custom transport, opt-in bounded GET retries and sanitized request logging.
- Add per-user serialized token refresh before authenticated HTTP calls and WebSocket handshakes/reconnects.
- Serialize WebSocket sends/receives and cancel pending reconnects on close.
- Stream multipart upload bodies with file progress callbacks and safely encoded filenames.
- Add net/http and Gin external-auth adapters and a standalone SDK integration consumer.
- Preserve APIError, existing client methods and no automatic replay of HTTP writes or WebSocket messages.

## 0.1.0

Initial typed HTTP/WebSocket client and external-user authentication adapter.
