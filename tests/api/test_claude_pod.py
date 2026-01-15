"""
API Tests for Claude Pod Service (Port 30001)

Claude Pod provides the AI chat interface.
"""
import pytest


class TestClaudePodHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, claude_pod_client):
        """Test /health endpoint returns healthy status."""
        response = claude_pod_client.get("/health")

        assert response is not None, f"Service unreachable: {claude_pod_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestClaudePodAPI:
    """Test API endpoints."""

    def test_index_page(self, claude_pod_client):
        """Test / endpoint returns chat interface."""
        response = claude_pod_client.get("/")

        assert response is not None, f"Service unreachable: {claude_pod_client.last_error}"
        assert response.status_code == 200

    def test_chat_endpoint_exists(self, claude_pod_client):
        """Test /chat endpoint exists and accepts POST."""
        # Just verify the endpoint exists - don't send actual messages
        response = claude_pod_client.post("/chat", json={"message": "test"})

        # Should get some response (not 404)
        if response is not None:
            assert response.status_code != 404, "Chat endpoint not found"
