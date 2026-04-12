# @relayhq/relay-node

Official Node.js server SDK for [Relay](https://github.com/relayhq/relay-server) — the open source, self-hostable real-time WebSocket server.

Publish events, query channels, and authenticate private/presence channel subscriptions from your Node.js backend.

## Install

```bash
npm install @relayhq/relay-node
```

## Quick Start

```typescript
import { RelayClient } from '@relayhq/relay-node';

const relay = new RelayClient({
  host: '127.0.0.1',
  port: 6001,
  appId: 'my-app',
  key: 'my-key',
  secret: 'my-secret',
});

// Publish an event
await relay.publish('chat', 'new-message', { text: 'Hello!' });

// Publish a batch of events
await relay.publishBatch([
  { channel: 'chat', event: 'new-message', data: { text: 'Hello' } },
  { channel: 'alerts', event: 'notify', data: { level: 'info' } },
]);

// Query channels
const channels = await relay.getChannels();
const channel = await relay.getChannel('chat');
const users = await relay.getChannelUsers('presence-room');
```

## Authentication

For private and presence channels, clients must authenticate their subscription server-side.

### Manual authentication

```typescript
const auth = relay.authenticate(socketId, 'private-orders');

// For presence channels, include user data
const auth = relay.authenticate(socketId, 'presence-room', {
  user_id: 42,
  user_info: { name: 'Alice' },
});
```

### Express middleware

```typescript
import express from 'express';
import { RelayClient, relayAuthMiddleware } from '@relayhq/relay-node';

const app = express();
app.use(express.json());

const relay = new RelayClient({ /* config */ });

app.post('/broadcasting/auth', relayAuthMiddleware(relay, (req) => {
  return req.user; // return user object or null to deny
}));
```

## API Reference

### `new RelayClient(config)`

| Option   | Type    | Default | Description                |
|----------|---------|---------|----------------------------|
| `host`   | string  | —       | Relay server hostname      |
| `port`   | number  | —       | Relay server port          |
| `appId`  | string  | —       | Your Relay app ID          |
| `key`    | string  | —       | Your Relay app key         |
| `secret` | string  | —       | Your Relay app secret      |
| `tls`    | boolean | false   | Use HTTPS                  |

### Methods

| Method | Description |
|--------|-------------|
| `publish(channel, event, data, excludeSocketId?)` | Publish an event to a channel |
| `publishBatch(items[])` | Publish multiple events at once |
| `getChannels()` | List all active channels |
| `getChannel(name)` | Get info for one channel |
| `getChannelUsers(channel)` | Get presence channel members |
| `authenticate(socketId, channelName, channelData?)` | Generate auth token |

## Requirements

- Node.js 18+
- Zero runtime dependencies

## License

MIT
