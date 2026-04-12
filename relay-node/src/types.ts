export interface RelayConfig {
  host: string;
  port: number;
  appId: string;
  key: string;
  secret: string;
  tls?: boolean;
}

export interface PublishData {
  channel: string;
  event: string;
  data: string | Record<string, unknown>;
  socket_id?: string;
}

export interface BatchItem {
  channel: string;
  event: string;
  data: string | Record<string, unknown>;
  socket_id?: string;
}

export interface ChannelInfo {
  name: string;
  type: string;
  subscriber_count: number;
  occupied: boolean;
  app_id: string;
}

export interface ChannelsResponse {
  channels: ChannelInfo[];
}

export interface ChannelUsersResponse {
  users: Array<{ id: string | number; user_info: Record<string, unknown> }>;
}

export interface AuthResponse {
  auth: string;
  channel_data?: string;
}

export interface RelayResponse {
  ok?: boolean;
  count?: number;
  error?: string;
}
