"""
API Tests for HolmOS Shell Service (Port 30000)

HolmOS Shell provides the iPhone-style home screen interface.
"""
import pytest


class TestHolmosShellHealth:
    """Test health and readiness endpoints."""

    def test_health_endpoint(self, holmos_shell_client):
        """Test /health endpoint returns healthy status."""
        response = holmos_shell_client.get("/health")

        assert response is not None, f"Service unreachable: {holmos_shell_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "healthy"

    def test_ready_endpoint(self, holmos_shell_client):
        """Test /ready endpoint returns ready status."""
        response = holmos_shell_client.get("/ready")

        assert response is not None, f"Service unreachable: {holmos_shell_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "ready"


class TestHolmosShellAPI:
    """Test API endpoints."""

    def test_list_apps(self, holmos_shell_client):
        """Test /api/apps endpoint returns list of apps."""
        response = holmos_shell_client.get("/api/apps")

        assert response is not None, f"Service unreachable: {holmos_shell_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "ok"
        assert "host" in data
        assert "apps" in data
        assert isinstance(data["apps"], list)
        assert len(data["apps"]) > 0

    def test_status_endpoint(self, holmos_shell_client):
        """Test /api/status endpoint returns system status."""
        response = holmos_shell_client.get("/api/status")

        assert response is not None, f"Service unreachable: {holmos_shell_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "running"
        assert "version" in data
        assert "host" in data


class TestHolmosShellUI:
    """Test UI endpoints."""

    def test_index_page(self, holmos_shell_client):
        """Test / endpoint returns HTML page."""
        response = holmos_shell_client.get("/")

        assert response is not None, f"Service unreachable: {holmos_shell_client.last_error}"
        assert response.status_code == 200
        assert "text/html" in response.headers.get("Content-Type", "")
        assert "HolmOS" in response.text
