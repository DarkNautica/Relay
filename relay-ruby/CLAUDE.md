# relay-ruby — Claude Code Context

## What This Is
Official Ruby server SDK for Relay. Backend publishing client for Ruby and
Rails apps to publish events, query channels, and authenticate subscriptions.

## Architecture
- `lib/relay.rb` — Main require, loads all modules
- `lib/relay/client.rb` — Relay::Client class with all HTTP methods
- `lib/relay/auth.rb` — HMAC-SHA256 signature generation
- `lib/relay/channel.rb` — Channel value object
- `lib/relay/rails/broadcaster.rb` — ActionCable adapter
- `lib/relay/rails/railtie.rb` — Rails auto-configuration

## Key Design Decisions
- Zero runtime dependencies — uses Ruby built-in net/http and openssl
- Rails integration via Railtie + ActionCable adapter pattern
- cable.yml uses `adapter: relay` to switch from Redis/Postgres
- Auth signature: HMAC-SHA256 of `{socketId}:{channelName}` or `{socketId}:{channelName}:{channelData}`
- HTTP API auth: `Authorization: Bearer {appSecret}`
