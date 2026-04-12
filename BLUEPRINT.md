# Relay — Universal Real-Time Server
## Master Blueprint v1.0

---

## What Is Relay

Relay is a self-hostable, framework-agnostic real-time WebSocket server.
It replaces paid services like Pusher and Ably with a free, open source,
single-binary alternative that any developer can run on their own infrastructure.

Every modern web app needs real-time functionality — live notifications,
chat, presence indicators, collaborative features. Relay provides that
infrastructure in a way that is free, beautiful to use, and easy to deploy.

---

## Repository Ecosystem

| Repo             | Language       | Purpose                              |
|------------------|----------------|--------------------------------------|
| relay-server     | Go             | Core WebSocket server + HTTP API     |
| relay-js         | TypeScript     | Browser/Node client SDK              |
| relay-php        | PHP            | Laravel Broadcasting driver          |
| relay-docs       | MDX/Astro      | Documentation site                   |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                     Your Application                     │
│                                                         │
│   Backend (Laravel/Rails/Node)   Frontend (JS/React)    │
│          │                              │               │
│          │ HTTP Publish API             │ WebSocket      │
│          │                              │               │
└──────────┼──────────────────────────────┼───────────────┘
           │                              │
           ▼                              ▼
┌─────────────────────────────────────────────────────────┐
│                    Relay Server (Go)                     │
│                                                         │
│  ┌─────────┐  ┌─────────┐  ┌──────────┐  ┌─────────┐  │
│  │   Hub   │  │Channels │  │Presence  │  │  Auth   │  │
│  └─────────┘  └─────────┘  └──────────┘  └─────────┘  │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │              WebSocket Handler                   │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │              HTTP API + Dashboard                │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

---

## Channel Types

### Public Channels
- Any client can subscribe without authentication
- Prefix: none (e.g. `chat`, `news`, `scores`)
- Use case: live feeds, public scoreboards, public notifications

### Private Channels
- Subscription requires server-side authentication
- Prefix: `private-` (e.g. `private-user.123`)
- Use case: user-specific notifications, private chat

### Presence Channels
- Like private channels but also track who is subscribed
- Prefix: `presence-` (e.g. `presence-room.456`)
- Use case: online indicators, collaborative cursors, member lists

---

## Protocol

Relay is fully compatible with the Pusher protocol v7.
This means any app currently using Pusher can switch to Relay
by changing a single environment variable — no code changes.

Relay also supports its own extended protocol for features
beyond what Pusher offers (binary messages, channel history, etc.)

### Message Format (JSON)
```json
{
  "event": "relay:subscribe",
  "channel": "presence-room.1",
  "data": {
    "auth": "app-key:signature",
    "channel_data": "{\"user_id\":1,\"user_info\":{\"name\":\"Jayden\"}}"
  }
}
```

---

## Server Events (Relay → Client)

| Event                    | Description                          |
|--------------------------|--------------------------------------|
| relay:connection_established | Sent on connect with socket_id   |
| relay:subscription_succeeded | Channel subscribe confirmed      |
| relay:member_added       | Presence: new member joined          |
| relay:member_removed     | Presence: member left                |
| relay:error              | Error with code and message          |
| relay:pong               | Response to client ping              |

---

## Client Events (Client → Relay)

| Event                    | Description                          |
|--------------------------|--------------------------------------|
| relay:subscribe          | Subscribe to a channel               |
| relay:unsubscribe        | Unsubscribe from a channel           |
| relay:ping               | Keep-alive ping                      |
| client-*                 | Client events (forwarded to channel) |

---

## HTTP API

### Publish Event
```
POST /apps/{appId}/events
Authorization: Bearer {appSecret}

{
  "channel": "chat",
  "event": "new-message",
  "data": { "text": "Hello world" }
}
```

### Publish to Multiple Channels
```
POST /apps/{appId}/events/batch
Authorization: Bearer {appSecret}

{
  "batch": [
    { "channel": "chat", "event": "new-message", "data": {} },
    { "channel": "notifications", "event": "alert", "data": {} }
  ]
}
```

### Get Channel Info
```
GET /apps/{appId}/channels/{channelName}
Authorization: Bearer {appSecret}
```

### Get All Channels
```
GET /apps/{appId}/channels
Authorization: Bearer {appSecret}
```

### Get Presence Members
```
GET /apps/{appId}/channels/{channelName}/users
Authorization: Bearer {appSecret}
```

### Authentication Endpoint (for private/presence)
```
POST /apps/{appId}/auth
```

---

## Configuration (.env)

```env
RELAY_HOST=0.0.0.0
RELAY_PORT=6001
RELAY_APP_ID=my-app
RELAY_APP_KEY=my-key
RELAY_APP_SECRET=my-secret
RELAY_DEBUG=false
RELAY_MAX_CONNECTIONS=10000
RELAY_MAX_CHANNEL_NAME_LENGTH=200
RELAY_MAX_EVENT_PAYLOAD_KB=100
RELAY_PING_INTERVAL=120
RELAY_PING_TIMEOUT=30
RELAY_DASHBOARD_ENABLED=true
RELAY_DASHBOARD_PATH=/dashboard
```

---

## Deployment

### Docker (Recommended)
```bash
docker run -d \
  -p 6001:6001 \
  -e RELAY_APP_KEY=your-key \
  -e RELAY_APP_SECRET=your-secret \
  relayhq/relay:latest
```

### Docker Compose
```bash
docker compose up -d
```

### Binary
```bash
./relay serve
```

---

## Roadmap

### Phase 1 — Core (Current)
- [x] Project scaffold and architecture
- [ ] WebSocket server with public channels
- [ ] Private channel authentication
- [ ] Presence channels
- [ ] HTTP publish API
- [ ] JS client SDK
- [ ] Laravel PHP driver

### Phase 2 — Polish
- [ ] Built-in web dashboard
- [ ] Docker image + Docker Compose
- [ ] Channel history / replay
- [ ] Webhook support
- [ ] Rate limiting

### Phase 3 — Scale
- [ ] Horizontal scaling via Redis
- [ ] Native protocol extensions
- [ ] Binary message support
- [ ] Rails driver
- [ ] Django driver

### Phase 4 — Cloud
- [ ] Relay Cloud (managed hosting)
- [ ] Multi-app support
- [ ] Usage analytics
- [ ] SLA monitoring

---

## Tech Stack Decisions

| Decision          | Choice          | Reason                                        |
|-------------------|-----------------|-----------------------------------------------|
| Server language   | Go              | Native concurrency, single binary, performance|
| WebSocket lib     | gorilla/websocket | Battle-tested, widely used                  |
| Router            | gorilla/mux     | Flexible, well documented                     |
| JS SDK language   | TypeScript      | Type safety, better DX                        |
| PHP driver        | PHP 8.1+        | Laravel compatibility                          |
| Dashboard         | React + Vite    | Fast dev, small bundle                        |
| Docs              | Astro + MDX     | Fast static site, great DX                   |

---

## Naming Conventions

- Go packages: lowercase, single word (`hub`, `auth`, `protocol`)
- Go types: PascalCase (`HubClient`, `ChannelType`, `RelayServer`)
- Events: colon-namespaced (`relay:subscribe`, `client-message`)
- Channels: prefix-based (`private-`, `presence-`)
- ENV vars: `RELAY_` prefix
- HTTP routes: RESTful, `/apps/{appId}/...`
