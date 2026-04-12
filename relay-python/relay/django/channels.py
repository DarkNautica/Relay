"""Django Channels layer backend for Relay.

This allows Django Channels to use Relay as the backing transport,
similar to how you'd use Redis with channels_redis.

Add to your Django settings:

    CHANNEL_LAYERS = {
        "default": {
            "BACKEND": "relay.django.channels.RelayChannelLayer",
            "CONFIG": {
                "host": "127.0.0.1",
                "port": 6001,
                "app_id": "my-app",
                "key": "my-key",
                "secret": "my-secret",
            },
        },
    }
"""

from __future__ import annotations

import json
from typing import Any

from relay.client import RelayClient


class RelayChannelLayer:
    """Django Channels layer that publishes messages via the Relay HTTP API.

    This is a send-only layer — Relay handles the WebSocket connections
    directly, so the receive side is managed by Relay's own protocol.
    """

    extensions = ["groups"]

    def __init__(self, **config: Any) -> None:
        self.client = RelayClient(
            host=config.get("host", "127.0.0.1"),
            port=config.get("port", 6001),
            app_id=config.get("app_id", ""),
            key=config.get("key", ""),
            secret=config.get("secret", ""),
            tls=config.get("tls", False),
        )
        # In-memory group tracking for the layer interface
        self._groups: dict[str, set[str]] = {}

    # ─── Channel Layer interface ────────────────────────────────────

    async def send(self, channel: str, message: dict[str, Any]) -> None:
        """Send a message to a specific channel."""
        event = message.get("type", "message")
        await self.client.async_publish(channel, event, message)

    async def receive(self, channel: str) -> dict[str, Any]:
        """Receive is not supported — Relay handles WebSocket delivery."""
        raise NotImplementedError(
            "RelayChannelLayer is send-only. "
            "Relay delivers messages directly via WebSocket."
        )

    async def group_add(self, group: str, channel: str) -> None:
        """Track a channel's membership in a group."""
        if group not in self._groups:
            self._groups[group] = set()
        self._groups[group].add(channel)

    async def group_discard(self, group: str, channel: str) -> None:
        """Remove a channel from a group."""
        if group in self._groups:
            self._groups[group].discard(channel)
            if not self._groups[group]:
                del self._groups[group]

    async def group_send(self, group: str, message: dict[str, Any]) -> None:
        """Send a message to all members of a group via Relay."""
        event = message.get("type", "message")
        await self.client.async_publish(group, event, message)

    async def new_channel(self, prefix: str = "specific.") -> str:
        """Generate a new channel name (for point-to-point messaging)."""
        import uuid
        return f"{prefix}{uuid.uuid4().hex}"

    # ─── Flush (for testing) ────────────────────────────────────────

    async def flush(self) -> None:
        """Clear all local state."""
        self._groups.clear()
