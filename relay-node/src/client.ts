import http from 'node:http';
import https from 'node:https';
import type {
  RelayConfig,
  BatchItem,
  ChannelInfo,
  ChannelsResponse,
  ChannelUsersResponse,
  AuthResponse,
  RelayResponse,
} from './types';
import { generateAuth } from './auth';

export class RelayClient {
  private config: RelayConfig;
  private baseUrl: string;

  constructor(config: RelayConfig) {
    this.config = config;
    const scheme = config.tls ? 'https' : 'http';
    this.baseUrl = `${scheme}://${config.host}:${config.port}`;
  }

  /**
   * Publish an event to a single channel.
   */
  async publish(
    channel: string,
    event: string,
    data: string | Record<string, unknown>,
    excludeSocketId?: string
  ): Promise<RelayResponse> {
    const payload: Record<string, unknown> = {
      channel,
      event,
      data: typeof data === 'string' ? data : JSON.stringify(data),
    };

    if (excludeSocketId) {
      payload.socket_id = excludeSocketId;
    }

    return this.post<RelayResponse>('/events', payload);
  }

  /**
   * Publish multiple events in a single request.
   */
  async publishBatch(items: BatchItem[]): Promise<RelayResponse> {
    const batch = items.map((item) => ({
      channel: item.channel,
      event: item.event,
      data: typeof item.data === 'string' ? item.data : JSON.stringify(item.data),
      ...(item.socket_id ? { socket_id: item.socket_id } : {}),
    }));

    return this.post<RelayResponse>('/events/batch', { batch });
  }

  /**
   * Get all active channels for this app.
   */
  async getChannels(): Promise<ChannelInfo[]> {
    const res = await this.get<ChannelsResponse>('/channels');
    return res.channels;
  }

  /**
   * Get info for a single channel.
   */
  async getChannel(name: string): Promise<ChannelInfo> {
    return this.get<ChannelInfo>(`/channels/${encodeURIComponent(name)}`);
  }

  /**
   * Get the list of users in a presence channel.
   */
  async getChannelUsers(channel: string): Promise<ChannelUsersResponse> {
    return this.get<ChannelUsersResponse>(
      `/channels/${encodeURIComponent(channel)}/users`
    );
  }

  /**
   * Generate an auth token for a private or presence channel.
   * This is used server-side to authorize client subscriptions.
   */
  authenticate(
    socketId: string,
    channelName: string,
    channelData?: Record<string, unknown>
  ): AuthResponse {
    return generateAuth(
      this.config.key,
      this.config.secret,
      socketId,
      channelName,
      channelData
    );
  }

  // ─── HTTP helpers ────────────────────────────────────────────────

  private appPath(path: string): string {
    return `/apps/${this.config.appId}${path}`;
  }

  private request<T>(method: string, path: string, body?: unknown): Promise<T> {
    return new Promise((resolve, reject) => {
      const url = new URL(this.appPath(path), this.baseUrl);
      const transport = this.config.tls ? https : http;
      const bodyStr = body !== undefined ? JSON.stringify(body) : undefined;

      const req = transport.request(
        url,
        {
          method,
          headers: {
            Authorization: `Bearer ${this.config.secret}`,
            'Content-Type': 'application/json',
            Accept: 'application/json',
            ...(bodyStr ? { 'Content-Length': Buffer.byteLength(bodyStr).toString() } : {}),
          },
        },
        (res) => {
          let data = '';
          res.on('data', (chunk: Buffer) => {
            data += chunk.toString();
          });
          res.on('end', () => {
            if (res.statusCode && res.statusCode >= 400) {
              let message = `Relay API error: ${res.statusCode}`;
              try {
                const parsed = JSON.parse(data);
                if (parsed.error) message = parsed.error;
              } catch {
                // ignore parse error
              }
              reject(new Error(message));
              return;
            }
            try {
              resolve(JSON.parse(data) as T);
            } catch {
              resolve({} as T);
            }
          });
        }
      );

      req.on('error', (err) => reject(new Error(`Relay request failed: ${err.message}`)));

      if (bodyStr) {
        req.write(bodyStr);
      }
      req.end();
    });
  }

  private post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>('POST', path, body);
  }

  private get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }
}
