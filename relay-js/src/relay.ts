/**
 * Relay JS — Official client SDK for the Relay real-time server.
 * Zero dependencies. TypeScript native.
 *
 * @packageDocumentation
 */

// ─────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────

/** Configuration options for the Relay client. */
export type RelayConfig = {
  /** Relay server hostname, e.g. "localhost" or "relay.example.com" */
  host: string
  /** Your app key (set in Relay server config) */
  key: string
  /** Override the WebSocket port (default: 6001) */
  port?: number
  /** Enable TLS (wss://) — recommended in production */
  tls?: boolean
  /** Activity timeout in seconds before sending a ping (default: 120) */
  activityTimeout?: number
  /** Seconds to wait for a pong before reconnecting (default: 30) */
  pongTimeout?: number
  /** Maximum reconnection attempts before giving up (default: 6) */
  maxReconnectAttempts?: number
  /** Auth endpoint URL for private/presence channels (default: "/broadcasting/auth") */
  authEndpoint?: string
  /** Extra headers to send with auth requests */
  authHeaders?: Record<string, string>
  /** Custom auth transport — override the default fetch-based auth */
  auth?: (socketId: string, channel: string) => Promise<{ auth: string; channel_data?: string }>
  /** Log connection state changes and received events to the console (default: false) */
  logToConsole?: boolean
}

/** Callback invoked when a channel or connection event fires. */
export type EventCallback = (data: any) => void

/** All possible connection states. */
export type ConnectionState =
  | 'initialized'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'failed'
  | 'unavailable'

/** A member in a presence channel. */
export type PresenceMember = {
  /** Unique user identifier */
  id: string | number
  /** Arbitrary user metadata */
  info?: any
}

/** Connection info object returned by `relay.connection`. */
export type ConnectionInfo = {
  /** Current connection state */
  state: ConnectionState
  /** The socket ID assigned by the server, or null if not connected */
  socket_id: string | null
  /** Bind a listener to a connection state: 'connecting', 'connected', 'disconnected', 'failed' */
  bind: (state: ConnectionState, callback: () => void) => void
}

// ─────────────────────────────────────────────
// Channel
// ─────────────────────────────────────────────

/**
 * Represents a subscription to a Relay channel.
 * Use `bind()` to listen for events and `trigger()` to send client events.
 */
export class Channel {
  /** The channel name, e.g. "chat" or "private-user.1" */
  readonly name: string
  /** @internal */
  private callbacks: Map<string, EventCallback[]> = new Map()
  /** @internal */
  private allCallbacks: EventCallback[] = []
  /** @internal */
  private relay: Relay

  /** @internal */
  constructor(name: string, relay: Relay) {
    this.name = name
    this.relay = relay
  }

  /**
   * Listen for a specific event on this channel.
   * @param event - Event name to listen for
   * @param callback - Handler invoked with the event data
   * @returns this (for chaining)
   */
  bind(event: string, callback: EventCallback): this {
    if (!this.callbacks.has(event)) {
      this.callbacks.set(event, [])
    }
    this.callbacks.get(event)!.push(callback)
    return this
  }

  /**
   * Listen for every event on this channel. Useful for debugging.
   * The callback receives `{ event, data }`.
   * @param callback - Handler invoked for all events
   * @returns this (for chaining)
   */
  bindAll(callback: EventCallback): this {
    this.allCallbacks.push(callback)
    return this
  }

  /**
   * Remove a specific listener, or all listeners for an event.
   * @param event - Event name
   * @param callback - Specific callback to remove; omit to remove all for this event
   * @returns this (for chaining)
   */
  unbind(event: string, callback?: EventCallback): this {
    if (!callback) {
      this.callbacks.delete(event)
      return this
    }
    const cbs = this.callbacks.get(event)
    if (cbs) {
      const filtered = cbs.filter(cb => cb !== callback)
      this.callbacks.set(event, filtered)
    }
    return this
  }

  /**
   * Remove all listeners from this channel, including bindAll listeners.
   * @returns this (for chaining)
   */
  unbindAll(): this {
    this.callbacks.clear()
    this.allCallbacks = []
    return this
  }

  /** Unsubscribe from this channel entirely. */
  unsubscribe(): void {
    this.relay.unsubscribe(this.name)
  }

  /**
   * Trigger a client event on a private or presence channel.
   * Event names must start with "client-".
   * @param event - Event name (must be prefixed with "client-")
   * @param data - Payload to send
   */
  trigger(event: string, data: any): void {
    if (!event.startsWith('client-')) {
      console.warn('[Relay] Client events must be prefixed with "client-"')
      return
    }
    this.relay.sendRaw({
      event,
      channel: this.name,
      data: JSON.stringify(data),
    })
  }

  /** @internal Called by Relay when an event arrives for this channel */
  _emit(event: string, data: any): void {
    const cbs = this.callbacks.get(event) ?? []
    for (const cb of cbs) {
      try {
        cb(data)
      } catch (err) {
        console.error(`[Relay] Error in "${event}" handler on channel "${this.name}":`, err)
      }
    }
    // Fire bindAll listeners
    for (const cb of this.allCallbacks) {
      try {
        cb({ event, data })
      } catch (err) {
        console.error(`[Relay] Error in bindAll handler on channel "${this.name}":`, err)
      }
    }
  }
}

// ─────────────────────────────────────────────
// Presence Channel
// ─────────────────────────────────────────────

/**
 * A presence channel that tracks online members.
 * Extends Channel with member management.
 */
export class PresenceChannel extends Channel {
  /** Map of all currently subscribed members, keyed by user ID. */
  members: Map<string | number, PresenceMember> = new Map()
  /** The current user's presence member data, set after subscription succeeds. */
  me: PresenceMember | null = null

  /** @internal */
  _handleSubscriptionSucceeded(data: any): void {
    if (data?.presence) {
      const { ids, hash } = data.presence
      for (const id of ids) {
        this.members.set(id, hash[id])
      }
    }
  }

  /** @internal */
  _handleMemberAdded(member: PresenceMember): void {
    this.members.set(member.id, member)
    this._emit('relay:member_added', member)
  }

  /** @internal */
  _handleMemberRemoved(member: PresenceMember): void {
    this.members.delete(member.id)
    this._emit('relay:member_removed', member)
  }
}

// ─────────────────────────────────────────────
// Relay Client
// ─────────────────────────────────────────────

/**
 * The main Relay client. Creates a WebSocket connection, manages
 * channel subscriptions, handles authentication and reconnection.
 *
 * ```ts
 * const relay = new Relay('my-key', { host: 'localhost', port: 6001 })
 * const channel = relay.subscribe('chat')
 * channel.bind('message', (data) => console.log(data))
 * ```
 */
export class Relay {
  private config: Required<Omit<RelayConfig, 'auth'>> & { auth: RelayConfig['auth'] }
  private ws: WebSocket | null = null
  private channels: Map<string, Channel> = new Map()
  private socketId: string | null = null
  private state: ConnectionState = 'initialized'
  private reconnectAttempts = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private pingTimer: ReturnType<typeof setTimeout> | null = null
  private pongTimer: ReturnType<typeof setTimeout> | null = null
  private globalCallbacks: Map<string, EventCallback[]> = new Map()
  private connectionStateCallbacks: Map<ConnectionState, (() => void)[]> = new Map()

  /**
   * Create a new Relay client and connect to the server.
   * @param key - Your Relay app key
   * @param config - Connection and auth configuration
   */
  constructor(key: string, config: Omit<RelayConfig, 'key'>) {
    this.config = {
      key,
      host: config.host ?? 'localhost',
      port: config.port ?? 6001,
      tls: config.tls ?? false,
      activityTimeout: config.activityTimeout ?? 120,
      pongTimeout: config.pongTimeout ?? 30,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 6,
      authEndpoint: config.authEndpoint ?? '/broadcasting/auth',
      authHeaders: config.authHeaders ?? {},
      auth: config.auth,
      logToConsole: config.logToConsole ?? false,
    }
    this.connect()
  }

  // ─── Connection ───────────────────────────────────

  private connect(): void {
    this.setState('connecting')

    const scheme = this.config.tls ? 'wss' : 'ws'
    const url = `${scheme}://${this.config.host}:${this.config.port}/app/${this.config.key}`

    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempts = 0
    }

    this.ws.onmessage = (event) => {
      this.handleMessage(event.data)
    }

    this.ws.onclose = () => {
      this.clearTimers()
      this.setState('disconnected')
      this.scheduleReconnect()
    }

    this.ws.onerror = () => {
      // onerror is always followed by onclose — handle reconnection there
    }
  }

  /** Close the connection and stop all reconnection attempts. */
  disconnect(): void {
    this.clearTimers()
    this.ws?.close()
    this.ws = null
    this.setState('disconnected')
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      const reason = `Failed to connect after ${this.config.maxReconnectAttempts} attempts`
      this.log(`connection failed: ${reason}`)
      this.setState('failed')
      this.emitGlobal('failed', { reason })
      return
    }

    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000)
    this.reconnectAttempts++
    this.log(`reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.config.maxReconnectAttempts})`)

    this.reconnectTimer = setTimeout(() => {
      this.connect()
      // Resubscribe to all channels after reconnect
      this.channels.forEach((_, name) => this.sendSubscribe(name))
    }, delay)
  }

  // ─── Subscribe / Unsubscribe ──────────────────────

  /**
   * Subscribe to a channel. Returns a Channel or PresenceChannel instance.
   * If already subscribed, returns the existing instance.
   * @param channelName - Channel name (prefix with "private-" or "presence-" for auth channels)
   * @returns The channel instance
   */
  subscribe(channelName: string): Channel {
    if (this.channels.has(channelName)) {
      return this.channels.get(channelName)!
    }

    let channel: Channel
    if (channelName.startsWith('presence-')) {
      channel = new PresenceChannel(channelName, this)
    } else {
      channel = new Channel(channelName, this)
    }

    this.channels.set(channelName, channel)

    if (this.state === 'connected') {
      this.sendSubscribe(channelName)
    }

    return channel
  }

  /**
   * Unsubscribe from a channel and remove all its listeners.
   * @param channelName - Channel to unsubscribe from
   */
  unsubscribe(channelName: string): void {
    this.channels.delete(channelName)
    this.sendRaw({ event: 'relay:unsubscribe', data: JSON.stringify({ channel: channelName }) })
  }

  /**
   * Get an already-subscribed channel by name.
   * @param name - Channel name
   * @returns The channel, or undefined if not subscribed
   */
  channel(name: string): Channel | undefined {
    return this.channels.get(name)
  }

  private async sendSubscribe(channelName: string): Promise<void> {
    const isPrivate = channelName.startsWith('private-')
    const isPresence = channelName.startsWith('presence-')

    let auth: string | undefined
    let channelData: string | undefined

    if ((isPrivate || isPresence) && this.socketId) {
      try {
        const result = await this.getAuth(channelName)
        auth = result.auth
        channelData = result.channel_data
      } catch (err) {
        console.error(`[Relay] Auth failed for channel "${channelName}":`, err)
        return
      }
    }

    this.sendRaw({
      event: 'relay:subscribe',
      data: JSON.stringify({ channel: channelName, auth, channel_data: channelData }),
    })
  }

  private async getAuth(channelName: string): Promise<{ auth: string; channel_data?: string }> {
    if (this.config.auth) {
      return this.config.auth(this.socketId!, channelName)
    }

    const res = await fetch(this.config.authEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...this.config.authHeaders,
      },
      body: JSON.stringify({
        socket_id: this.socketId,
        channel_name: channelName,
      }),
    })

    if (!res.ok) {
      throw new Error(`Auth failed for "${channelName}": HTTP ${res.status} ${res.statusText}`)
    }

    return res.json()
  }

  // ─── Message Handling ─────────────────────────────

  private handleMessage(raw: string): void {
    let msg: { event: string; channel?: string; data?: string }
    try {
      msg = JSON.parse(raw)
    } catch {
      return
    }

    const data = msg.data ? this.parseData(msg.data) : undefined

    switch (msg.event) {
      case 'relay:connection_established':
        this.socketId = data?.socket_id ?? null
        this.setState('connected')
        this.startPingCycle()
        // Subscribe to any channels that were queued before connection
        this.channels.forEach((_, name) => this.sendSubscribe(name))
        break

      case 'relay:subscription_succeeded':
        if (msg.channel) {
          const ch = this.channels.get(msg.channel)
          if (ch instanceof PresenceChannel) {
            ch._handleSubscriptionSucceeded(data)
          }
          ch?._emit('relay:subscription_succeeded', data)
          this.log(`subscribed to ${msg.channel}`)
        }
        break

      case 'relay:member_added':
        if (msg.channel) {
          const ch = this.channels.get(msg.channel)
          if (ch instanceof PresenceChannel) {
            ch._handleMemberAdded(data)
          }
        }
        break

      case 'relay:member_removed':
        if (msg.channel) {
          const ch = this.channels.get(msg.channel)
          if (ch instanceof PresenceChannel) {
            ch._handleMemberRemoved(data)
          }
        }
        break

      case 'relay:pong':
        this.clearPongTimer()
        break

      case 'relay:error':
        console.error('[Relay] Server error:', data)
        break

      default:
        // Forward to the appropriate channel
        if (msg.channel) {
          const ch = this.channels.get(msg.channel)
          ch?._emit(msg.event, data)
          this.log(`event: ${msg.event} on ${msg.channel}`)
        }
        // Also fire global listeners
        this.emitGlobal(msg.event, { channel: msg.channel, data })
    }
  }

  private parseData(data: string): any {
    try {
      return JSON.parse(data)
    } catch {
      return data
    }
  }

  // ─── Keep-Alive ───────────────────────────────────

  private startPingCycle(): void {
    this.pingTimer = setTimeout(() => {
      this.sendRaw({ event: 'relay:ping', data: '{}' })

      this.pongTimer = setTimeout(() => {
        // No pong received — assume connection is dead
        this.ws?.close()
      }, this.config.pongTimeout * 1000)
    }, this.config.activityTimeout * 1000)
  }

  private clearPongTimer(): void {
    if (this.pongTimer) {
      clearTimeout(this.pongTimer)
      this.pongTimer = null
    }
    // Schedule next ping
    this.startPingCycle()
  }

  private clearTimers(): void {
    if (this.pingTimer) clearTimeout(this.pingTimer)
    if (this.pongTimer) clearTimeout(this.pongTimer)
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
  }

  // ─── Utilities ────────────────────────────────────

  /** @internal Send a raw JSON message over the WebSocket. */
  sendRaw(msg: object): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  private setState(state: ConnectionState): void {
    const prev = this.state
    this.state = state
    if (prev !== state) {
      this.log(state)
    }
    this.emitGlobal('state_change', { previous: prev, current: state })
    // Fire connection state callbacks
    const cbs = this.connectionStateCallbacks.get(state) ?? []
    for (const cb of cbs) cb()
  }

  /**
   * Bind a global connection-level event listener.
   * Fires for all events across all channels plus internal events like 'state_change' and 'failed'.
   * @param event - Event name
   * @param callback - Handler
   * @returns this (for chaining)
   */
  bind(event: string, callback: EventCallback): this {
    if (!this.globalCallbacks.has(event)) {
      this.globalCallbacks.set(event, [])
    }
    this.globalCallbacks.get(event)!.push(callback)
    return this
  }

  private emitGlobal(event: string, data: any): void {
    const cbs = this.globalCallbacks.get(event) ?? []
    for (const cb of cbs) cb(data)
  }

  /** Log a message when `logToConsole` is enabled. */
  private log(message: string): void {
    if (this.config.logToConsole) {
      console.log(`[Relay] ${message}`)
    }
  }

  /**
   * Connection info and state binding.
   *
   * Use `relay.connection.state` to read the current state, or
   * `relay.connection.bind('connected', () => {})` to listen for state changes.
   */
  get connection(): ConnectionInfo {
    return {
      state: this.state,
      socket_id: this.socketId,
      bind: (state: ConnectionState, callback: () => void) => {
        if (!this.connectionStateCallbacks.has(state)) {
          this.connectionStateCallbacks.set(state, [])
        }
        this.connectionStateCallbacks.get(state)!.push(callback)
      },
    }
  }
}

export default Relay
