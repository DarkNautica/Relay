"""Django middleware and decorator for Relay channel authentication.

Handles POST /broadcasting/auth using Django's authentication system.

Usage as middleware (add to MIDDLEWARE):

    # settings.py
    RELAY_CLIENT = RelayClient(host='127.0.0.1', port=6001, ...)

    MIDDLEWARE = [
        ...
        'relay.django.middleware.RelayAuthMiddleware',
    ]

    RELAY_AUTH_PATH = '/broadcasting/auth'  # default

Usage as a view decorator:

    from relay.django.middleware import relay_auth_required

    @relay_auth_required
    def my_auth_view(request):
        # Only reached if user is authenticated
        # Return channel_data dict for presence channels, or None
        return {'user_id': request.user.id, 'user_info': {'name': str(request.user)}}
"""

from __future__ import annotations

import json
import functools
from typing import Any, Callable

from relay.client import RelayClient


class RelayAuthMiddleware:
    """Django middleware that intercepts POST requests to the auth endpoint.

    Requires Django settings:
        RELAY_CLIENT: a RelayClient instance
        RELAY_AUTH_PATH: the URL path to intercept (default: '/broadcasting/auth')
    """

    def __init__(self, get_response: Callable) -> None:
        self.get_response = get_response

    def __call__(self, request: Any) -> Any:
        from django.conf import settings
        from django.http import JsonResponse

        auth_path = getattr(settings, "RELAY_AUTH_PATH", "/broadcasting/auth")

        if request.method == "POST" and request.path == auth_path:
            return self._handle_auth(request, settings)

        return self.get_response(request)

    def _handle_auth(self, request: Any, settings: Any) -> Any:
        from django.http import JsonResponse

        client: RelayClient | None = getattr(settings, "RELAY_CLIENT", None)
        if client is None:
            return JsonResponse(
                {"error": "RELAY_CLIENT not configured"}, status=500
            )

        if not request.user or not request.user.is_authenticated:
            return JsonResponse({"error": "Forbidden"}, status=403)

        try:
            body = json.loads(request.body)
        except (json.JSONDecodeError, ValueError):
            return JsonResponse(
                {"error": "Invalid JSON body"}, status=400
            )

        socket_id = body.get("socket_id")
        channel_name = body.get("channel_name")

        if not socket_id or not channel_name:
            return JsonResponse(
                {"error": "socket_id and channel_name are required"}, status=400
            )

        channel_data = None
        if channel_name.startswith("presence-"):
            channel_data = {
                "user_id": request.user.pk,
                "user_info": {"name": str(request.user)},
            }

        auth = client.authenticate(socket_id, channel_name, channel_data=channel_data)
        return JsonResponse(auth)


def relay_auth_required(view_func: Callable) -> Callable:
    """Decorator for Django views that handle Relay channel authentication.

    The decorated view should return a dict of channel_data for presence
    channels, or None for private channels. Return None to use default
    user info, or raise PermissionDenied to deny access.

    Example:
        @relay_auth_required
        def auth_view(request):
            return {'user_id': request.user.id, 'user_info': {'name': 'Alice'}}
    """

    @functools.wraps(view_func)
    def wrapper(request: Any, *args: Any, **kwargs: Any) -> Any:
        from django.conf import settings
        from django.http import JsonResponse

        client: RelayClient | None = getattr(settings, "RELAY_CLIENT", None)
        if client is None:
            return JsonResponse(
                {"error": "RELAY_CLIENT not configured"}, status=500
            )

        if not request.user or not request.user.is_authenticated:
            return JsonResponse({"error": "Forbidden"}, status=403)

        try:
            body = json.loads(request.body)
        except (json.JSONDecodeError, ValueError):
            return JsonResponse({"error": "Invalid JSON body"}, status=400)

        socket_id = body.get("socket_id")
        channel_name = body.get("channel_name")

        if not socket_id or not channel_name:
            return JsonResponse(
                {"error": "socket_id and channel_name are required"}, status=400
            )

        # Call the view to get channel_data
        channel_data = view_func(request, *args, **kwargs)

        # For presence channels, ensure we have channel_data
        if channel_name.startswith("presence-") and channel_data is None:
            channel_data = {
                "user_id": request.user.pk,
                "user_info": {"name": str(request.user)},
            }

        auth = client.authenticate(socket_id, channel_name, channel_data=channel_data)
        return JsonResponse(auth)

    return wrapper
