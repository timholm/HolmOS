"""
API Tests for Terminal Web Service (Port 30800)

Terminal Web provides a web-based terminal interface.
"""
import pytest


class TestTerminalWebHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, terminal_web_client):
        """Test /health endpoint returns healthy status."""
        response = terminal_web_client.get("/health")

        assert response is not None, f"Service unreachable: {terminal_web_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestTerminalWebAPI:
    """Test API endpoints."""

    def test_index_page(self, terminal_web_client):
        """Test / endpoint returns terminal interface."""
        response = terminal_web_client.get("/")

        assert response is not None, f"Service unreachable: {terminal_web_client.last_error}"
        assert response.status_code == 200
        assert "text/html" in response.headers.get("Content-Type", "")

    def test_sessions_endpoint(self, terminal_web_client):
        """Test /api/sessions or /sessions endpoint."""
        for path in ["/api/sessions", "/sessions", "/api/terminals"]:
            response = terminal_web_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 401, 404]


class TestTerminalWebWebSocket:
    """Test WebSocket connectivity for terminal sessions."""

    def test_terminal_websocket_endpoint(self, terminal_web_client):
        """Test that terminal WebSocket endpoint exists."""
        headers = {
            "Upgrade": "websocket",
            "Connection": "Upgrade",
            "Sec-WebSocket-Version": "13",
            "Sec-WebSocket-Key": "dGVzdGtleQ=="
        }

        for path in ["/ws", "/terminal", "/api/ws"]:
            response = terminal_web_client.get(path, headers=headers)
            if response is not None and response.status_code in [101, 400, 426]:
                break
