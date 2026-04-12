# relay-python — Claude Code Context

## What This Is
Official Python server SDK for Relay. Backend publishing client for Python
and Django apps to publish events, query channels, and authenticate subscriptions.

## Architecture
- `relay/__init__.py` — Exports RelayClient and generate_auth
- `relay/client.py` — RelayClient class with sync + async methods
- `relay/auth.py` — HMAC-SHA256 signature generation
- `relay/django/__init__.py` — Django integration exports
- `relay/django/channels.py` — Django Channels layer backend
- `relay/django/middleware.py` — Auth middleware + decorator

## Key Design Decisions
- Zero runtime dependencies — uses Python built-in urllib, hmac, hashlib
- Sync by default, async variants use asyncio.to_thread
- Django middleware auto-handles /broadcasting/auth using request.user
- Django Channels layer is send-only (Relay handles WebSocket delivery)
- Auth signature: HMAC-SHA256 of `{socketId}:{channelName}` or with channel_data
