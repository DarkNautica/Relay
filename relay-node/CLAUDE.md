# relay-node — Claude Code Context

## What This Is
Official Node.js server SDK for Relay. Backend publishing client that lets
Node.js apps publish events, query channels, and authenticate private/presence
channel subscriptions via the Relay HTTP API.

## Architecture
- `src/client.ts` — RelayClient class, all HTTP methods (publish, channels, auth)
- `src/auth.ts` — HMAC-SHA256 signature generation for channel authentication
- `src/middleware.ts` — Express middleware for handling /broadcasting/auth
- `src/types.ts` — All TypeScript interfaces
- `src/index.ts` — Public exports

## Key Design Decisions
- Zero runtime dependencies — uses Node built-in `http`/`https` and `crypto`
- Auth signatures use HMAC-SHA256, matching the Pusher protocol
- String to sign: `{socketId}:{channelName}` (private) or `{socketId}:{channelName}:{channelData}` (presence)
- Auth token format: `{appKey}:{hexSignature}`
- HTTP API auth: `Authorization: Bearer {appSecret}`
- All API paths prefixed with `/apps/{appId}/`

## Building
```bash
npm install
npm run build
```

## Testing
Tests can be run against a live Relay server instance.
