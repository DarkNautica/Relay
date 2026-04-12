# Relay Server — Claude Code Context

## What This Project Is
Relay is an open source, self-hostable, framework-agnostic real-time WebSocket
server. It replaces paid services like Pusher and Ably. Written in Go.

## Project Goals
- Full Pusher protocol v7 compatibility (existing apps switch with one env var)
- Self-hostable as a single binary or Docker container
- Support public, private, and presence channels
- HTTP API for publishing events from any backend
- Beautiful developer experience

## Architecture
The server is built around a central Hub that manages all connections and
channels using Go channels and goroutines. Each WebSocket connection is a
Client goroutine. Channels route messages between clients.

## Key Packages
- `internal/config` — all configuration loaded from env
- `internal/hub` — Hub (broker), Client (connection), Channel (room)
- `internal/protocol` — Pusher-compatible message types and parsing
- `internal/websocket` — HTTP upgrade handler, client read/write pumps
- `internal/api` — REST API handlers for publishing and channel info
- `internal/auth` — HMAC authentication for private/presence channels
- `internal/presence` — Presence channel member management

## Go Conventions Used Here
- Errors always returned, never panicked in production code
- Goroutines communicate via channels, never shared memory
- Context used for cancellation and timeouts
- All exported types have godoc comments

## Pusher Protocol Notes
- Socket ID format: `{random}.{random}` (e.g. `283491.9834748`)
- Auth signature: HMAC-SHA256 of `{socketId}:{channelName}` using appSecret
- Presence auth also includes channel_data JSON in the signature

## Port
Default: 6001 (same as Pusher/Soketi/Reverb convention)

## Environment Variables
All config via environment variables prefixed with RELAY_
See .env.example for full reference.

## Testing
- Unit tests alongside each package (`_test.go` files)
- Integration tests in `/tests/`
- Run with: `go test ./...`

## Building
```bash
go build -o relay ./main.go
```

## Running
```bash
./relay
# or
go run ./main.go
```
