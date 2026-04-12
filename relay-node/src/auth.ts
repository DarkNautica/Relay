import { createHmac } from 'node:crypto';
import type { AuthResponse } from './types';

/**
 * Generate an HMAC-SHA256 auth signature for private/presence channels.
 *
 * Private channels:  sign "{socketId}:{channelName}"
 * Presence channels: sign "{socketId}:{channelName}:{channelData}"
 *
 * Returns { auth: "key:hex_signature", channel_data?: "..." }
 */
export function generateAuth(
  key: string,
  secret: string,
  socketId: string,
  channelName: string,
  channelData?: Record<string, unknown>
): AuthResponse {
  let stringToSign = `${socketId}:${channelName}`;
  let channelDataJson: string | undefined;

  if (channelData !== undefined) {
    channelDataJson = JSON.stringify(channelData);
    stringToSign += `:${channelDataJson}`;
  }

  const signature = createHmac('sha256', secret)
    .update(stringToSign)
    .digest('hex');

  const result: AuthResponse = { auth: `${key}:${signature}` };

  if (channelDataJson !== undefined) {
    result.channel_data = channelDataJson;
  }

  return result;
}
