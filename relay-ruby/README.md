# relay-ruby

Official Ruby server SDK for [Relay](https://github.com/relayhq/relay-server) — the open source, self-hostable real-time WebSocket server.

Publish events, query channels, and authenticate private/presence channel subscriptions from your Ruby or Rails backend.

## Install

```ruby
# Gemfile
gem 'relay-ruby'
```

## Quick Start

```ruby
require 'relay'

client = Relay::Client.new(
  host: '127.0.0.1',
  port: 6001,
  app_id: 'my-app',
  key: 'my-key',
  secret: 'my-secret'
)

# Publish an event
client.publish('chat', 'new-message', { text: 'Hello!' })

# Publish a batch
client.publish_batch([
  { channel: 'chat', event: 'new-message', data: { text: 'Hello' } },
  { channel: 'alerts', event: 'notify', data: { level: 'info' } }
])

# Query channels
channels = client.get_channels
channel  = client.get_channel('chat')
users    = client.get_channel_users('presence-room')
```

## Authentication

```ruby
# Private channel
auth = client.authenticate(socket_id, 'private-orders')

# Presence channel
auth = client.authenticate(socket_id, 'presence-room',
  channel_data: { user_id: 42, user_info: { name: 'Alice' } }
)
```

## Rails Integration

### 1. Add to Gemfile

```ruby
gem 'relay-ruby'
```

### 2. Configure cable.yml

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

### 3. Broadcast as usual

```ruby
ActionCable.server.broadcast('chat', { message: 'Hello' })
```

The Relay adapter replaces Redis/Postgres as the ActionCable backend. All existing `broadcast` calls work unchanged.

### Auth endpoint

Add a route for client-side channel authentication:

```ruby
# config/routes.rb
post '/broadcasting/auth', to: 'relay#auth'

# app/controllers/relay_controller.rb
class RelayController < ApplicationController
  def auth
    client = Relay::Rails.client
    unless current_user
      head :forbidden
      return
    end

    channel_data = nil
    if params[:channel_name].start_with?('presence-')
      channel_data = { user_id: current_user.id, user_info: { name: current_user.name } }
    end

    render json: client.authenticate(
      params[:socket_id],
      params[:channel_name],
      channel_data: channel_data
    )
  end
end
```

## API Reference

| Method | Description |
|--------|-------------|
| `publish(channel, event, data, exclude_socket_id:)` | Publish an event |
| `publish_batch(items)` | Publish multiple events |
| `get_channels` | List all active channels |
| `get_channel(name)` | Get info for one channel |
| `get_channel_users(channel)` | Get presence members |
| `authenticate(socket_id, channel_name, channel_data:)` | Generate auth token |

## Requirements

- Ruby 3.0+
- Zero runtime dependencies

## License

MIT
