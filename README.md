# ⚡ Relay

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

---

## Channel Types

| Type     | Prefix      | Auth Required | Members Tracked |
|----------|-------------|---------------|-----------------|
| Public   | (none)      | No            | No              |
| Private  | `private-`  | Yes           | No              |
| Presence | `presence-` | Yes           | Yes             |

---

## Repositories

| Repo            | Description                     |
|-----------------|---------------------------------|
| relay-server    | Go server — the core            |
| relay-js        | JavaScript/TypeScript client SDK|
| relay-php       | Laravel Broadcasting driver     |
| relay-docs      | Documentation site              |

---

## Roadmap

- [x] Core WebSocket server
- [x] Public, private, presence channels
- [x] HTTP publish API
- [x] JS client SDK
- [x] Laravel driver
- [ ] Web dashboard
- [ ] Channel history / replay
- [ ] Webhook support
- [ ] Horizontal scaling (Redis)
- [ ] Rails driver
- [ ] Django driver
- [ ] Relay Cloud (managed hosting)

---

## Contributing

Relay is MIT licensed and welcomes contributions.
See CONTRIBUTING.md for guidelines.

---

## License

MIT © Relay HQ
