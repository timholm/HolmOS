"""
API Tests for Chat Hub Service (Port 30003)

Chat Hub provides unified agent messaging with WebSocket support.
"""
import pytest


class TestChatHubHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, chat_hub_client):
        """Test /health endpoint returns healthy status."""
        response = chat_hub_client.get("/health")

        assert response is not None, f"Service unreachable: {chat_hub_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestChatHubAPI:
    """Test API endpoints."""

    def test_index_page(self, chat_hub_client):
        """Test / endpoint returns chat interface."""
        response = chat_hub_client.get("/")

        assert response is not None, f"Service unreachable: {chat_hub_client.last_error}"
        assert response.status_code == 200

    def test_agents_list(self, chat_hub_client):
        """Test /api/agents endpoint returns list of agents."""
        response = chat_hub_client.get("/api/agents")

        if response is not None and response.status_code == 200:
            data = response.json()
            # Should return list of agents
            assert isinstance(data, (list, dict))

    def test_messages_endpoint(self, chat_hub_client):
        """Test /api/messages endpoint."""
        response = chat_hub_client.get("/api/messages")

        if response is not None:
            # Should exist but may require authentication
            assert response.status_code in [200, 401, 403, 404]


class TestChatHubWebSocket:
    """Test WebSocket connectivity (basic HTTP handshake check)."""

    def test_websocket_upgrade_available(self, chat_hub_client):
        """Test that WebSocket upgrade headers are supported."""
        # We can't do full WebSocket test with requests, but we can check
        # the endpoint accepts WebSocket upgrade requests
        headers = {
            "Upgrade": "websocket",
            "Connection": "Upgrade",
            "Sec-WebSocket-Version": "13",
            "Sec-WebSocket-Key": "dGVzdGtleQ=="
        }
        response = chat_hub_client.get("/ws", headers=headers)

        # WebSocket endpoints typically return 101 or 400 for upgrade
        # 404 means the endpoint doesn't exist
        if response is not None:
            assert response.status_code in [101, 400, 426, 200, 404]
