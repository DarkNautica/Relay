# frozen_string_literal: true

require 'openssl'

module Relay
  module Auth
    # Generate an HMAC-SHA256 auth signature for private/presence channels.
    #
    # Private channels:  sign "{socket_id}:{channel_name}"
    # Presence channels: sign "{socket_id}:{channel_name}:{channel_data_json}"
    #
    # Returns { "auth" => "key:hex_signature", "channel_data" => "..." }
    def self.sign(key:, secret:, socket_id:, channel_name:, channel_data: nil)
      string_to_sign = "#{socket_id}:#{channel_name}"

      channel_data_json = nil
      if channel_data
        channel_data_json = channel_data.is_a?(String) ? channel_data : channel_data.to_json
        string_to_sign = "#{string_to_sign}:#{channel_data_json}"
      end

      digest = OpenSSL::Digest.new('sha256')
      signature = OpenSSL::HMAC.hexdigest(digest, secret, string_to_sign)

      result = { 'auth' => "#{key}:#{signature}" }
      result['channel_data'] = channel_data_json if channel_data_json

      result
    end
  end
end
