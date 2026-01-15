"""
API Tests for App Store Service (Port 30002)

App Store provides the AI-powered app generator.
"""
import pytest


class TestAppStoreHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, app_store_client):
        """Test /health endpoint returns healthy status."""
        response = app_store_client.get("/health")

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "healthy"
        assert data.get("service") == "app-store-ai"


class TestAppStoreAPI:
    """Test API endpoints."""

    def test_list_apps(self, app_store_client):
        """Test /apps endpoint returns list of apps from registry."""
        response = app_store_client.get("/apps")

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        # May return 200 or 500 depending on registry connectivity
        if response.status_code == 200:
            data = response.json()
            assert "apps" in data
            assert "count" in data

    def test_merchant_catalog(self, app_store_client):
        """Test /merchant/catalog endpoint."""
        response = app_store_client.get("/merchant/catalog")

        # May return error if Merchant service is not available
        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        # Status 200 if Merchant available, 500 if not
        assert response.status_code in [200, 500]

    def test_forge_builds(self, app_store_client):
        """Test /forge/builds endpoint."""
        response = app_store_client.get("/forge/builds")

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        # May return error if Forge service is not available
        assert response.status_code in [200, 500]

    def test_kaniko_jobs(self, app_store_client):
        """Test /kaniko/jobs endpoint returns Kaniko build jobs."""
        response = app_store_client.get("/kaniko/jobs")

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        if response.status_code == 200:
            data = response.json()
            assert "items" in data or "error" in data


class TestAppStoreUI:
    """Test UI endpoints."""

    def test_index_page(self, app_store_client):
        """Test / endpoint returns HTML page."""
        response = app_store_client.get("/")

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        assert response.status_code == 200
        assert "text/html" in response.headers.get("Content-Type", "")
        assert "AI App Store" in response.text


class TestAppStoreMerchantIntegration:
    """Test Merchant AI integration."""

    def test_merchant_chat_endpoint(self, app_store_client):
        """Test /merchant/chat endpoint accepts messages."""
        response = app_store_client.post(
            "/merchant/chat",
            json={"message": "hello", "session_id": "test-session"}
        )

        assert response is not None, f"Service unreachable: {app_store_client.last_error}"
        # May return error if Merchant service is not available
        if response.status_code == 200:
            data = response.json()
            assert "session_id" in data
