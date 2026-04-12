/**
 * Relay JS — Official client SDK for the Relay real-time server
 * Zero dependencies. TypeScript native.
 */

// ─────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────

export type RelayConfig = {
  /** Your Relay server URL e.g. "ws://localhost:6001" */
  host: string
  /** Your app key (set in Relay server config) */
  key: string
  /** Optional: override the WebSocket port */
  port?: number
  /** Enable TLS (wss://) — recommended in production */
  tls?: boolean
  /** Activity timeout in seconds before sending a ping */
  activityTimeout?: number
  /** Time to wait for a pong before reconnecting */
  pongTimeout?: number
  /** How many times to attempt reconnection (default: 6) */
  maxReconnectAttempts?: number
  /** Auth endpoint for private/presence channels */
  authEndpoint?: string
  /** Headers to send with auth requests */
  authHeaders?: Record<string, string>
  /** Custom auth transport function — override default fetch */
  auth?: (socketId: string, channel: string) => Promise<{ auth: string; channel_data?: string }>
}

export type EventCallback = (data: any) => void

export type ConnectionState =
  | 'initialized'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'failed'
  | 'unavailable'

// ─────────────────────────────────────────────
// Channel
// ─────────────────────────────────────────────

export class Channel {
  readonly name: string
  private callbacks: Map<string, EventCallback[]> = new Map()
  private relay: Relay

  constructor(name: string, relay: Relay) {
    this.name = name
    this.relay = relay
  }

  /** Listen for an event on this channel */
  bind(event: string, callback: EventCallback): this {
    if (!this.callbacks.has(event)) {
      this.callbacks.set(event, [])
    }
    this.callbacks.get(event)!.push(callback)
    return this
  }

  /** Remove a listener */
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

  /** Unsubscribe from this channel entirely */
  unsubscribe(): void {
    this.relay.unsubscribe(this.name)
  }

  /** Trigger a client event (only works on private/presence channels) */
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
  }
}

// ─────────────────────────────────────────────
// Presence Channel
// ─────────────────────────────────────────────

export type PresenceMember = {
  id: string | number
  info?: any
}

export class PresenceChannel extends Channel {
  members: Map<string | number, PresenceMember> = new Map()
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

export class Relay {
  private config: Required<RelayConfig>
  private ws: WebSocket | null = null
  private channels: Map<string, Channel> = new Map()
  private socketId: string | null = null
  private state: ConnectionState = 'initialized'
  private reconnectAttempts = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private pingTimer: ReturnType<typeof setTimeout> | null = null
  private pongTimer: ReturnType<typeof setTimeout> | null = null
  private globalCallbacks: Map<string, EventCallback[]> = new Map()

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
      auth: config.auth!,
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

  disconnect(): void {
    this.clearTimers()
    this.ws?.close()
    this.ws = null
    this.setState('disconnected')
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      this.setState('failed')
      return
    }

    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000)
    this.reconnectAttempts++

    this.reconnectTimer = setTimeout(() => {
      this.connect()
      // Resubscribe to all channels after reconnect
      this.channels.forEach((_, name) => this.sendSubscribe(name))
    }, delay)
  }

  // ─── Subscribe / Unsubscribe ──────────────────────

  /** Subscribe to a public channel */
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

  /** Unsubscribe from a channel */
  unsubscribe(channelName: string): void {
    this.channels.delete(channelName)
    this.sendRaw({ event: 'relay:unsubscribe', data: JSON.stringify({ channel: channelName }) })
  }

  /** Get an already-subscribed channel */
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
      throw new Error(`Auth request failed: ${res.status}`)
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

  /** @internal */
  sendRaw(msg: object): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  private setState(state: ConnectionState): void {
    this.state = state
    this.emitGlobal('state_change', { current: state })
  }

  /** Bind a global connection-level event */
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

  get connection() {
    return {
      state: this.state,
      socket_id: this.socketId,
    }
  }
}

export default Relay
