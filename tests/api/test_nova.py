"""
API Tests for Nova Service (Port 30004)

Nova is the cluster guardian agent with the catchphrase:
"I see all 13 stars in our constellation."
"""
import pytest


class TestNovaHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, nova_client):
        """Test /health endpoint returns healthy status."""
        response = nova_client.get("/health")

        assert response is not None, f"Service unreachable: {nova_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "healthy"
        assert data.get("agent") == "Nova"


class TestNovaAPI:
    """Test API endpoints."""

    def test_capabilities_endpoint(self, nova_client):
        """Test /capabilities endpoint returns Nova's capabilities."""
        response = nova_client.get("/capabilities")

        assert response is not None, f"Service unreachable: {nova_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["agent"] == "Nova"
        assert "catchphrase" in data
        assert "I see all 13 stars" in data["catchphrase"]
        assert "features" in data
        assert isinstance(data["features"], list)

    def test_dashboard_data(self, nova_client):
        """Test /api/dashboard endpoint returns cluster data."""
        response = nova_client.get("/api/dashboard")

        assert response is not None, f"Service unreachable: {nova_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert "nodes" in data
        assert "pods" in data
        assert "deployments" in data
        assert "metrics" in data
        assert "timestamp" in data

    def test_chat_endpoint(self, nova_client):
        """Test /chat endpoint accepts messages."""
        response = nova_client.post(
            "/chat",
            json={"message": "status"}
        )

        assert response is not None, f"Service unreachable: {nova_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert "response" in data
        assert data.get("agent") == "Nova"


class TestNovaClusterOperations:
    """Test cluster management operations (read-only tests)."""

    def test_scale_endpoint_exists(self, nova_client):
        """Test /api/scale endpoint exists."""
        # We use OPTIONS or send empty payload to verify endpoint exists
        response = nova_client.post("/api/scale", json={})

        if response is not None:
            # Should not be 404
            assert response.status_code != 404

    def test_restart_endpoint_exists(self, nova_client):
        """Test /api/restart endpoint exists."""
        response = nova_client.post("/api/restart", json={})

        if response is not None:
            # Should not be 404
            assert response.status_code != 404

    def test_logs_endpoint_exists(self, nova_client):
        """Test /api/logs endpoint exists."""
        response = nova_client.post("/api/logs", json={})

        if response is not None:
            # Should not be 404
            assert response.status_code != 404


class TestNovaUI:
    """Test UI endpoints."""

    def test_index_page(self, nova_client):
        """Test / endpoint returns dashboard HTML."""
        response = nova_client.get("/")

        assert response is not None, f"Service unreachable: {nova_client.last_error}"
        assert response.status_code == 200
        assert "text/html" in response.headers.get("Content-Type", "")
        assert "Nova" in response.text
