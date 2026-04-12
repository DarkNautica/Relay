import type { IncomingMessage, ServerResponse } from 'node:http';
import type { RelayClient } from './client';

interface ExpressRequest extends IncomingMessage {
  body?: Record<string, unknown>;
  user?: unknown;
}

type AuthCallback = (req: ExpressRequest) => unknown | null;

/**
 * Express middleware that handles POST /broadcasting/auth requests.
 *
 * Usage:
 *   app.post('/broadcasting/auth', relayAuthMiddleware(relay, (req) => {
 *     return req.user; // return user object or null to deny
 *   }));
 */
export function relayAuthMiddleware(client: RelayClient, authCallback: AuthCallback) {
  return (req: ExpressRequest, res: ServerResponse): void => {
    // Parse body if not already parsed (requires body-parser or express.json())
    const body = req.body;

    if (!body || typeof body.socket_id !== 'string' || typeof body.channel_name !== 'string') {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'socket_id and channel_name are required' }));
      return;
    }

    const user = authCallback(req);

    if (!user) {
      res.writeHead(403, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Forbidden' }));
      return;
    }

    const socketId = body.socket_id as string;
    const channelName = body.channel_name as string;

    // For presence channels, include user data in the signature
    let channelData: Record<string, unknown> | undefined;
    if (channelName.startsWith('presence-')) {
      channelData = typeof user === 'object' ? (user as Record<string, unknown>) : { id: user };
    }

    const auth = client.authenticate(socketId, channelName, channelData);

    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(auth));
  };
}
