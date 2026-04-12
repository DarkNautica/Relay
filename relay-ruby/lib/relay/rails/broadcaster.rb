# frozen_string_literal: true

require 'json'

module Relay
  module Rails
    # ActionCable broadcaster adapter for Relay.
    #
    # Integrates with Rails' built-in broadcasting system so you can switch
    # from Pusher/Redis to Relay by changing one line in cable.yml:
    #
    #   production:
    #     adapter: relay
    #     host: 127.0.0.1
    #     port: 6001
    #     app_id: my-app
    #     key: my-key
    #     secret: my-secret
    #
    # Then broadcast as usual:
    #
    #   ActionCable.server.broadcast("chat", { message: "Hello" })
    #
    class Broadcaster
      attr_reader :client

      def initialize(options = {})
        @client = Relay::Client.new(
          host: options[:host] || options['host'] || '127.0.0.1',
          port: (options[:port] || options['port'] || 6001).to_i,
          app_id: options[:app_id] || options['app_id'],
          key: options[:key] || options['key'],
          secret: options[:secret] || options['secret'],
          tls: options[:tls] || options['tls'] || false
        )
      end

      # Called by ActionCable when broadcasting to a channel.
      def broadcast(channel, payload)
        data = payload.is_a?(String) ? payload : payload.to_json
        @client.publish(channel, 'broadcast', data)
      end

      # Called by ActionCable to subscribe to incoming messages.
      # Relay is push-only from the server side, so this is a no-op.
      def subscribe(channel, callback, success_callback = nil)
        success_callback&.call
      end

      # Called by ActionCable to unsubscribe.
      def unsubscribe(channel, callback)
        # No-op for push-only adapter
      end

      # Shuts down the adapter. No persistent connections to clean up.
      def shutdown
        # No-op
      end
    end
  end
end
