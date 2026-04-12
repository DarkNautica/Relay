# relay-python

Official Python server SDK for [Relay](https://github.com/relayhq/relay-server) — the open source, self-hostable real-time WebSocket server.

Publish events, query channels, and authenticate private/presence channel subscriptions from your Python or Django backend.

## Install

```bash
pip install relay-python
```

## Quick Start

```python
from relay import RelayClient

client = RelayClient(
    host='127.0.0.1',
    port=6001,
    app_id='my-app',
    key='my-key',
    secret='my-secret',
)

# Publish an event
client.publish('chat', 'new-message', {'text': 'Hello!'})

# Publish a batch
client.publish_batch([
    {'channel': 'chat', 'event': 'new-message', 'data': {'text': 'Hello'}},
    {'channel': 'alerts', 'event': 'notify', 'data': {'level': 'info'}},
])

# Query channels
channels = client.get_channels()
channel = client.get_channel('chat')
users = client.get_channel_users('presence-room')
```

## Async Support

```python
# All methods have async variants
await client.async_publish('chat', 'new-message', {'text': 'Hello!'})
await client.async_publish_batch(items)
```

## Authentication

```python
# Private channel
auth = client.authenticate(socket_id, 'private-orders')

# Presence channel
auth = client.authenticate(socket_id, 'presence-room',
    channel_data={'user_id': 42, 'user_info': {'name': 'Alice'}})
```

## Django Integration

### Middleware (automatic auth endpoint)

```python
# settings.py
from relay import RelayClient

RELAY_CLIENT = RelayClient(
    host='127.0.0.1',
    port=6001,
    app_id='my-app',
    key='my-key',
    secret='my-secret',
)

RELAY_AUTH_PATH = '/broadcasting/auth'  # default

MIDDLEWARE = [
    ...
    'relay.django.middleware.RelayAuthMiddleware',
]
```

The middleware intercepts POST requests to `/broadcasting/auth`, checks `request.user.is_authenticated`, and returns a signed auth token.

### Decorator (custom auth view)

```python
from relay.django import relay_auth_required

@relay_auth_required
def auth_view(request):
    # Return channel_data for presence channels, or None for private
    return {
        'user_id': request.user.id,
        'user_info': {'name': request.user.username},
    }
```

### Django Channels Layer

```python
# settings.py
CHANNEL_LAYERS = {
    "default": {
        "BACKEND": "relay.django.channels.RelayChannelLayer",
        "CONFIG": {
            "host": "127.0.0.1",
            "port": 6001,
            "app_id": "my-app",
            "key": "my-key",
            "secret": "my-secret",
        },
    },
}
```

Then use standard Django Channels group_send:

```python
from channels.layers import get_channel_layer

channel_layer = get_channel_layer()
await channel_layer.group_send('chat', {
    'type': 'new_message',
    'text': 'Hello!',
})
```

## API Reference

| Method | Description |
|--------|-------------|
| `publish(channel, event, data, exclude_socket_id=)` | Publish an event |
| `publish_batch(items)` | Publish multiple events |
| `async_publish(...)` | Async variant of publish |
| `async_publish_batch(items)` | Async variant of publish_batch |
| `get_channels()` | List all active channels |
| `get_channel(name)` | Get info for one channel |
| `get_channel_users(channel)` | Get presence members |
| `authenticate(socket_id, channel_name, channel_data=)` | Generate auth token |

## Requirements

- Python 3.10+
- Zero runtime dependencies

## License

MIT
