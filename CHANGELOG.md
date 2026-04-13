# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `GET /apps/{appId}/channels` now returns a map format with `subscription_count` and `user_count` (presence channels) for the Channel Inspector UI
- `GET /apps/{appId}/channels/{channelName}/events` now supports cursor-based pagination (`?limit=N&cursor=OPAQUE`), returns `socket_id` per event and `next_cursor` for paging
- `GET /apps/{appId}/stats` — new authenticated endpoint returning per-app `connections`, `peak_connections`, and `messages_count`
- Per-app peak connection tracking (high-water mark)
- Per-app message counter
- Integration tests for channels map format, channel events pagination, and per-app stats

## [0.2.0] - 2026-04-12

### Added
- Per-app connection limit enforcement via `max_connections` in apps.json
- Structured JSON logging for all server events (migrated from `log.Printf` to `log/slog`)
- Version variable injected at build time via `-ldflags`
- Version reported in startup log line
- Connection limit test suite (limit enforcement, per-app isolation, unlimited mode)

### Changed
- All log output is now structured JSON (one JSON object per line) for compatibility with log aggregators (Loki, Datadog, etc.)
- Connection limit rejections use WebSocket close code 4100 with "Over connection limit" message
- Webhook logging now includes `app_id` context on all log lines

## [0.1.0] - 2026-04-12

### Added
- Initial release
- Pusher-compatible WebSocket server (protocol v7)
- Public, private, and presence channel support
- HMAC-SHA256 authentication for private and presence channels
- HTTP API for publishing events (single and batch)
- Multi-app support via apps.json
- Hot reload via SIGHUP
- Built-in web dashboard
- Channel event history with replay
- Webhook support (channel.occupied, channel.vacated, member.added, member.removed)
- Rate limiting (per-IP for WebSocket connections and API requests)
- Integration test suite
- Cross-platform binary builds (Linux, macOS, Windows; amd64, arm64)
- Docker multi-platform images (linux/amd64, linux/arm64)
- GitHub Actions CI/CD (vet, test, release, Docker)
- Documentation site with GitHub Pages deployment
- Official SDKs: JavaScript/TypeScript, PHP/Laravel, Node.js, Ruby/Rails, Python/Django
