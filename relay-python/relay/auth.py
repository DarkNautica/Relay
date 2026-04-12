"""HMAC-SHA256 authentication for Relay private/presence channels."""

import hmac
import hashlib
import json
from typing import Any


def generate_auth(
    key: str,
    secret: str,
    socket_id: str,
    channel_name: str,
    channel_data: dict[str, Any] | None = None,
) -> dict[str, str]:
    """Generate an auth signature for a private or presence channel.

    Private channels:  sign "{socket_id}:{channel_name}"
    Presence channels: sign "{socket_id}:{channel_name}:{channel_data_json}"

    Returns {"auth": "key:hex_signature", "channel_data": "..."}.
    """
    string_to_sign = f"{socket_id}:{channel_name}"

    channel_data_json = None
    if channel_data is not None:
        channel_data_json = json.dumps(channel_data, separators=(",", ":"))
        string_to_sign = f"{string_to_sign}:{channel_data_json}"

    signature = hmac.new(
        secret.encode("utf-8"),
        string_to_sign.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()

    result: dict[str, str] = {"auth": f"{key}:{signature}"}

    if channel_data_json is not None:
        result["channel_data"] = channel_data_json

    return result
