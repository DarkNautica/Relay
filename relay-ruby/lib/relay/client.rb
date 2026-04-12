# frozen_string_literal: true

require 'net/http'
require 'json'
require 'uri'

module Relay
  class Client
    attr_reader :host, :port, :app_id, :key, :secret, :tls

    def initialize(host:, port: 6001, app_id:, key:, secret:, tls: false)
      @host = host
      @port = port
      @app_id = app_id
      @key = key
      @secret = secret
      @tls = tls
    end

    # Publish an event to a single channel.
    def publish(channel, event, data, exclude_socket_id: nil)
      payload = {
        channel: channel,
        event: event,
        data: data.is_a?(String) ? data : data.to_json
      }
      payload[:socket_id] = exclude_socket_id if exclude_socket_id

      post('/events', payload)
    end

    # Publish multiple events in a single request.
    def publish_batch(items)
      batch = items.map do |item|
        entry = {
          channel: item[:channel],
          event: item[:event],
          data: item[:data].is_a?(String) ? item[:data] : item[:data].to_json
        }
        entry[:socket_id] = item[:socket_id] if item[:socket_id]
        entry
      end

      post('/events/batch', { batch: batch })
    end

    # Get all active channels for this app.
    def get_channels
      response = get('/channels')
      (response['channels'] || []).map { |attrs| Channel.new(attrs) }
    end

    # Get info for a single channel.
    def get_channel(name)
      attrs = get("/channels/#{URI.encode_www_form_component(name)}")
      Channel.new(attrs)
    end

    # Get the members of a presence channel.
    def get_channel_users(channel)
      response = get("/channels/#{URI.encode_www_form_component(channel)}/users")
      response['users'] || []
    end

    # Generate an auth token for a private or presence channel.
    def authenticate(socket_id, channel_name, channel_data: nil)
      Auth.sign(
        key: @key,
        secret: @secret,
        socket_id: socket_id,
        channel_name: channel_name,
        channel_data: channel_data
      )
    end

    private

    def app_path(path)
      "/apps/#{@app_id}#{path}"
    end

    def base_uri
      scheme = @tls ? 'https' : 'http'
      URI("#{scheme}://#{@host}:#{@port}")
    end

    def connection
      uri = base_uri
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = @tls
      http.open_timeout = 5
      http.read_timeout = 5
      http
    end

    def default_headers
      {
        'Authorization' => "Bearer #{@secret}",
        'Content-Type' => 'application/json',
        'Accept' => 'application/json'
      }
    end

    def post(path, body)
      uri = URI(app_path(path))
      request = Net::HTTP::Post.new(uri, default_headers)
      request.body = body.to_json

      response = connection.request(request)
      handle_response(response)
    end

    def get(path)
      uri = URI(app_path(path))
      request = Net::HTTP::Get.new(uri, default_headers)

      response = connection.request(request)
      handle_response(response)
    end

    def handle_response(response)
      case response
      when Net::HTTPSuccess
        JSON.parse(response.body || '{}')
      else
        message = begin
          parsed = JSON.parse(response.body || '{}')
          parsed['error'] || "HTTP #{response.code}"
        rescue JSON::ParserError
          "HTTP #{response.code}"
        end
        raise Relay::Error, "Relay API error: #{message}"
      end
    end
  end
end
