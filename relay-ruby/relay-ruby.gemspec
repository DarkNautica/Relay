# frozen_string_literal: true

Gem::Specification.new do |spec|
  spec.name          = 'relay-ruby'
  spec.version       = '1.0.0'
  spec.authors       = ['Relay HQ']
  spec.email         = ['hello@relayhq.com']

  spec.summary       = 'Official Ruby server SDK for Relay'
  spec.description   = 'Publish events, query channels, and authenticate private/presence channel subscriptions from your Ruby or Rails backend.'
  spec.homepage      = 'https://github.com/relayhq/relay-ruby'
  spec.license       = 'MIT'

  spec.required_ruby_version = '>= 3.0.0'

  spec.files         = Dir['lib/**/*.rb'] + ['relay-ruby.gemspec', 'README.md', 'LICENSE']
  spec.require_paths = ['lib']

  # Zero runtime dependencies — uses Ruby built-in net/http and openssl
end
