# frozen_string_literal: true

module Relay
  class Channel
    attr_reader :name, :type, :subscriber_count, :occupied, :app_id

    def initialize(attrs = {})
      @name = attrs['name'] || attrs[:name]
      @type = attrs['type'] || attrs[:type]
      @subscriber_count = attrs['subscriber_count'] || attrs[:subscriber_count] || 0
      @occupied = attrs['occupied'] || attrs[:occupied] || false
      @app_id = attrs['app_id'] || attrs[:app_id]
    end

    def private?
      name&.start_with?('private-')
    end

    def presence?
      name&.start_with?('presence-')
    end

    def public?
      !private? && !presence?
    end
  end
end
