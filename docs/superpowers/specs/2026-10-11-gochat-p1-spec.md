# GoChat P1

Source: referenced chat 重构项目目录结构 (2), requirements 7–12. The user confirmed all six areas, implemented and verified in stages. Preserve existing HTTP and WebSocket contracts and the single-instance deployment model.

## Acceptance and work graph

1. SDK productization (independent): semantic version, changelog, timeout and custom transport, bounded retries for safe HTTP reads only, sanitized logger injection, token refresh callback for HTTP and WebSocket reconnect, standard library and Gin adapter examples, independently runnable integration tests. Existing APIError and reconnect behavior remain compatible.
2. Operations (independent): dependency-aware readiness with bounded MySQL/MinIO checks and explicit optional Redis/etcd; Prometheus text metrics for HTTP count/latency, authentication results, upload failures, WS connections/users, message counts/persistence failures and database pool; unified JSON envelopes and error codes including 404/405/panic. No credentials or unbounded request labels in metrics or logs. Document metrics scraping.
3. Conversations (independent): authenticated paginated list identified by private:<user_id> or group:<group_id>, last message/time, unread count, name/avatar, persisted per-user pin/mute/draft preferences. Include sent-only conversations and groups; prevent users accessing other users' settings or groups they have left. Message creation must make both sender/recipient conversations discoverable. Additive SQL migration.
4. Group management (independent): search, invitation, owner transfer (atomic owner and role changes), announcement, per-member mute/nickname, paginated members, dissolution and group read receipts. Owner/admin/member roles match model constants. Enforce membership, capacity, ownership and role hierarchy. Muted/nonmembers cannot send; only participants view history and receipts. Additive migration.
5. Media (independent, group integration depends on 4): thumbnails for supported raster images, ffprobe audio/video metadata and ffmpeg video conversion, persistent resumable chunk upload sessions with ownership/offset/size checks and expiry, streaming downloads authorized by owner or shared message/group membership, group file listing, expiry cleanup, MinIO lifecycle setup, SDK progress callback. Large file handling uses bounded memory/temp files and streaming. Never execute user filenames as shell commands. Document ffmpeg prerequisites and limits; tests must verify uploads/download access and resume behavior.
6. API documentation (depends on 1–5): checked-in OpenAPI covering all HTTP routes, request/response schemas and error envelope, accessible from HTTP; separate WS contract documentation and SDK methods for new APIs. Contract test ensures every registered API route appears in OpenAPI.

## Verification

User-approved boundaries: public HTTP, Go SDK, real WebSocket server/client, Docker E2E. Build vertical failing test -> implementation slices. Run targeted tests per stage and full go test/vet/race and extended Docker E2E at integration. Record actual results, never count unavailable tools as passes.

## Ownership

SDK ticket owns sdk and examples/sdk-*; operations ticket owns middleware/telemetry, readiness/metrics APIs and application wiring; groups ticket owns group models/services/handlers, group migration and group WS authorization; main integrator owns conversations/media/OpenAPI, integration SQL and E2E. Integrator resolves shared router/client integration after merges.
