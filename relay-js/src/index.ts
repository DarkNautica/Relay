/**
 * @relayhq/relay-js — Official client SDK for the Relay real-time server.
 *
 * @example
 * ```ts
 * import Relay, { RelayConfig, Channel, PresenceChannel } from '@relayhq/relay-js'
 *
 * const relay = new Relay('my-key', { host: 'localhost', port: 6001 })
 * const channel = relay.subscribe('chat')
 * channel.bind('message', (data) => console.log(data))
 * ```
 *
 * @packageDocumentation
 */

export {
  Relay,
  Relay as default,
  Channel,
  PresenceChannel,
} from './relay'

export type {
  RelayConfig,
  EventCallback,
  ConnectionState,
  PresenceMember,
  ConnectionInfo,
} from './relay'
