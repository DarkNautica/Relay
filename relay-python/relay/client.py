"""Relay HTTP API client for Python."""

from __future__ import annotations

import json
import urllib.request
import urllib.error
import urllib.parse
from typing import Any

from relay.auth import generate_auth


class RelayError(Exception):
    """Raised when a Relay API request fails."""


class RelayClient:
    """Server-side client for the Relay real-time WebSocket server.

    Publish events, query channels, and authenticate private/presence
    channel subscriptions from your Python backend.
    """

    def __init__(
        self,
        host: str,
        port: int = 6001,
        app_id: str = "",
        key: str = "",
        secret: str = "",
        tls: bool = False,
    ) -> None:
        self.host = host
        self.port = port
        self.app_id = app_id
        self.key = key
        self.secret = secret
        self.tls = tls

        scheme = "https" if tls else "http"
        self._base_url = f"{scheme}://{host}:{port}"

    # ─── Publishing ─────────────────────────────────────────────────

    def publish(
        self,
        channel: str,
        event: str,
        data: str | dict[str, Any],
        exclude_socket_id: str | None = None,
    ) -> dict[str, Any]:
        """Publish an event to a single channel."""
        payload: dict[str, Any] = {
            "channel": channel,
            "event": event,
            "data": data if isinstance(data, str) else json.dumps(data),
        }
        if exclude_socket_id:
            payload["socket_id"] = exclude_socket_id

        return self._post("/events", payload)

    def publish_batch(self, items: list[dict[str, Any]]) -> dict[str, Any]:
        """Publish multiple events in a single request."""
        batch = []
        for item in items:
            entry: dict[str, Any] = {
                "channel": item["channel"],
                "event": item["event"],
                "data": item["data"] if isinstance(item["data"], str) else json.dumps(item["data"]),
            }
            if "socket_id" in item:
                entry["socket_id"] = item["socket_id"]
            batch.append(entry)

        return self._post("/events/batch", {"batch": batch})

    # ─── Async variants ─────────────────────────────────────────────

    async def async_publish(
        self,
        channel: str,
        event: str,
        data: str | dict[str, Any],
        exclude_socket_id: str | None = None,
    ) -> dict[str, Any]:
        """Async variant of publish. Runs the HTTP call in a thread."""
        import asyncio
        return await asyncio.to_thread(
            self.publish, channel, event, data, exclude_socket_id
        )

    async def async_publish_batch(self, items: list[dict[str, Any]]) -> dict[str, Any]:
        """Async variant of publish_batch."""
        import asyncio
        return await asyncio.to_thread(self.publish_batch, items)

    # ─── Channel queries ────────────────────────────────────────────

    def get_channels(self) -> list[dict[str, Any]]:
        """Get all active channels for this app."""
        response = self._get("/channels")
        return response.get("channels", [])

    def get_channel(self, name: str) -> dict[str, Any]:
        """Get info for a single channel."""
        return self._get(f"/channels/{urllib.parse.quote(name, safe='')}")

    def get_channel_users(self, channel: str) -> list[dict[str, Any]]:
        """Get the members of a presence channel."""
        response = self._get(f"/channels/{urllib.parse.quote(channel, safe='')}/users")
        return response.get("users", [])

    # ─── Authentication ─────────────────────────────────────────────

    def authenticate(
        self,
        socket_id: str,
        channel_name: str,
        channel_data: dict[str, Any] | None = None,
    ) -> dict[str, str]:
        """Generate an auth token for a private or presence channel."""
        return generate_auth(
            key=self.key,
            secret=self.secret,
            socket_id=socket_id,
            channel_name=channel_name,
            channel_data=channel_data,
        )

    # ─── HTTP helpers ───────────────────────────────────────────────

    def _app_path(self, path: str) -> str:
        return f"/apps/{self.app_id}{path}"

    def _headers(self) -> dict[str, str]:
        return {
            "Authorization": f"Bearer {self.secret}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        }

    def _post(self, path: str, body: Any) -> dict[str, Any]:
        url = f"{self._base_url}{self._app_path(path)}"
        data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(url, data=data, headers=self._headers(), method="POST")
        return self._do_request(req)

    def _get(self, path: str) -> dict[str, Any]:
        url = f"{self._base_url}{self._app_path(path)}"
        req = urllib.request.Request(url, headers=self._headers(), method="GET")
        return self._do_request(req)

    def _do_request(self, req: urllib.request.Request) -> dict[str, Any]:
        try:
            with urllib.request.urlopen(req, timeout=5) as response:
                body = response.read().decode("utf-8")
                return json.loads(body) if body else {}
        except urllib.error.HTTPError as e:
            message = f"Relay API error: HTTP {e.code}"
            try:
                parsed = json.loads(e.read().decode("utf-8"))
                if "error" in parsed:
                    message = parsed["error"]
            except (json.JSONDecodeError, UnicodeDecodeError):
                pass
            raise RelayError(message) from e
        except urllib.error.URLError as e:
            raise RelayError(f"Relay request failed: {e.reason}") from e
