# ⚡ Relay

[![Release](https://img.shields.io/github/v/release/DarkNautica/Relay?label=latest)](https://github.com/DarkNautica/Relay/releases)

**The open source, self-hostable real-time WebSocket server.**

Replace Pusher and Ably with a single binary you control. Zero vendor lock-in.
Free forever. Framework-agnostic.

---

## Why Relay

Every modern app needs real-time functionality — live notifications, chat,
presence indicators, collaborative features. Existing solutions either cost
a fortune (Pusher, Ably) or are tied to a single framework (Laravel Reverb,
Soketi).

Relay is different:

- **Self-hosted** — runs on your own server, your own terms
- **Framework-agnostic** — works with Laravel, Rails, Django, Next.js, anything
- **Pusher-compatible** — existing apps switch with one config change
- **Single binary** — one file, no runtime, no JVM, no Node
- **Free forever** — MIT licensed, open source

---

## Quick Start

### 1. Run the server

```bash
# Docker (easiest)
docker run -d -p 6001:6001 \
  -e RELAY_APP_KEY=my-key \
  -e RELAY_APP_SECRET=my-secret \
  relayhq/relay:latest

# Or download the binary
./relay
```

### 2. Connect from JavaScript

```bash
npm install @relayhq/relay-js
```

```javascript
import Relay from '@relayhq/relay-js'

const relay = new Relay('my-key', {
  host: 'localhost',
  port: 6001,
})

const channel = relay.subscribe('chat')

channel.bind('new-message', (data) => {
  console.log('New message:', data.text)
})
```

### 3. Publish from Laravel

```bash
composer require relayhq/relay-php
```

```php
// config/broadcasting.php
'default' => 'relay',
'connections' => [
    'relay' => [
        'driver' => 'relay',
        'host'   => env('RELAY_HOST', '127.0.0.1'),
        'port'   => env('RELAY_PORT', 6001),
        'key'    => env('RELAY_APP_KEY'),
        'secret' => env('RELAY_APP_SECRET'),
        'app_id' => env('RELAY_APP_ID'),
    ],
],
```

```php
// Any Laravel event — just implement ShouldBroadcast
class MessageSent implements ShouldBroadcast
{
    public function broadcastOn(): array
    {
        return [new Channel('chat')];
    }
}

// Fire it
broadcast(new MessageSent($message));
```

### 4. Publish from Node.js

```bash
npm install @relayhq/relay-node
```

```typescript
import { RelayClient } from '@relayhq/relay-node'

const relay = new RelayClient({
  host: '127.0.0.1',
  port: 6001,
  appId: 'my-app',
  key: 'my-key',
  secret: 'my-secret',
})

await relay.publish('chat', 'new-message', { text: 'Hello!' })
```

### 5. Publish from Rails

```ruby
# Gemfile
gem 'relay-ruby'
```

```yaml
# config/cable.yml
production:
  adapter: relay
  host: 127.0.0.1
  port: 6001
  app_id: my-app
  key: my-key
  secret: my-secret
```

```ruby
ActionCable.server.broadcast('chat', { message: 'Hello!' })
```

### 6. Publish from Django / Python

```bash
pip install relay-python
```

```python
from relay import RelayClient

relay = RelayClient(
    host='127.0.0.1',
    port=6001,
    app_id='my-app',
    key='my-key',
    secret='my-secret',
)

relay.publish('chat', 'new-message', {'text': 'Hello!'})
```

---

## Connection Limit Enforcement

Each app in `apps.json` can define a `max_connections` limit. When the limit is
reached, new WebSocket connections for that app are rejected at handshake time
with close code `4100` ("Over connection limit"). Other apps are unaffected.

If `max_connections` is `0` or omitted, the app has no connection limit.

```json
[
  {
    "id": "my-app",
    "key": "my-key",
    "secret": "my-secret",
    "max_connections": 1000
  }
]
```

---

## Logging

Relay emits structured JSON logs (one JSON object per line) to stdout, ready for
log aggregators like Loki, Datadog, or CloudWatch.

Every log line includes `level`, `time`, and `msg`. Context fields like
`app_id`, `socket_id`, `channel`, `connections`, and `limit` are added where
relevant.

```jsonl
{"time":"2026-04-12T15:30:00Z","level":"INFO","msg":"server listening","addr":"0.0.0.0:6001"}
{"time":"2026-04-12T15:30:01Z","level":"WARN","msg":"connection rejected: app at connection limit","app_id":"my-app","app_key":"my-key","connections":1000,"limit":1000}
```

Set `RELAY_DEBUG=true` to include `DEBUG`-level messages (connection events,
message routing, subscription details).

---

## Channel Types

| Type     | Prefix      | Auth Required | Members Tracked |
|----------|-------------|---------------|-----------------|
| Public   | (none)      | No            | No              |
| Private  | `private-`  | Yes           | No              |
| Presence | `presence-` | Yes           | Yes             |

---

## Repositories

| Repo            | Description                          |
|-----------------|--------------------------------------|
| relay-server    | Go server — the core                 |
| relay-js        | JavaScript/TypeScript client SDK     |
| relay-php       | Laravel Broadcasting driver          |
| relay-node      | Node.js server SDK                   |
| relay-ruby      | Ruby / Rails server SDK              |
| relay-python    | Python / Django server SDK           |
| relay-docs      | Documentation site                   |

---

## Roadmap

- [x] Core WebSocket server
- [x] Public, private, presence channels
- [x] HTTP publish API
- [x] JS client SDK
- [x] Laravel driver
- [x] Web dashboard
- [x] Channel history / replay
- [x] Webhook support
- [ ] Horizontal scaling (Redis)
- [x] Node.js server SDK
- [x] Rails driver
- [x] Django driver
- [ ] Relay Cloud (managed hosting)

---

## Contributing

Relay is MIT licensed and welcomes contributions.
See CONTRIBUTING.md for guidelines.

---

## License

MIT © Relay HQ
