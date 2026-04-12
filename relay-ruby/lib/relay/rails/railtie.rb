# frozen_string_literal: true

require 'rails/railtie'

module Relay
  module Rails
    class Railtie < ::Rails::Railtie
      initializer 'relay.action_cable' do
        ActiveSupport.on_load(:action_cable) do
          require_relative 'broadcaster'

          # Register the Relay adapter so cable.yml can use `adapter: relay`
          ::ActionCable::Server::Configuration.class_eval do
            def relay_adapter
              Relay::Rails::Broadcaster
            end
          end
        end
      end

      # Provide a global Relay client accessible via Relay::Rails.client
      initializer 'relay.client' do |app|
        config = app.config_for(:cable) rescue {}
        if config['adapter'] == 'relay' || config[:adapter] == 'relay'
          Relay::Rails.instance_variable_set(:@client, Relay::Client.new(
            host: config['host'] || config[:host] || '127.0.0.1',
            port: (config['port'] || config[:port] || 6001).to_i,
            app_id: config['app_id'] || config[:app_id],
            key: config['key'] || config[:key],
            secret: config['secret'] || config[:secret],
            tls: config['tls'] || config[:tls] || false
          ))
        end
      end

      def self.client
        @client
      end
    end
  end
end
