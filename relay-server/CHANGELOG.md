# Changelog

## [0.3.0] - 2026-04-13

### Added
- Event store: tracks last 1000 events per app in memory
- Delivery tracking: records which socket IDs received each event
- Latency metrics: p50/p95/p99 delivery latency per app
- New API: GET /apps/{appId}/events — event timeline with pagination
- New API: GET /apps/{appId}/events/{eventId} — single event detail with delivery list
- New API: POST /apps/{appId}/events/{eventId}/replay — re-publish any historical event
- New API: GET /apps/{appId}/metrics — aggregate metrics

## [0.2.0] - 2026-04-12

### Added
- Per-app connection limits with atomic enforcement
- Structured JSON logging (slog)
- Channel inspector API (channels map, events pagination, per-app stats)

## [0.1.0] - 2026-04-11

### Added
- Initial release
- Pusher-compatible WebSocket server
- Public, private, and presence channels
- HTTP API for publishing events
- HMAC-SHA256 authentication
- Event history with ring buffers
- Webhook dispatcher with retries
- Embedded web dashboard
- Rate limiting per IP
- Multi-app support via apps.json
