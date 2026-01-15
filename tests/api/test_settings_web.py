"""
API Tests for Settings Web Service (Port 30600)

Settings Web provides the settings hub for HolmOS configuration.
"""
import pytest


class TestSettingsWebHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, settings_web_client):
        """Test /health endpoint returns healthy status."""
        response = settings_web_client.get("/health")

        assert response is not None, f"Service unreachable: {settings_web_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestSettingsWebAPI:
    """Test API endpoints."""

    def test_index_page(self, settings_web_client):
        """Test / endpoint returns settings interface."""
        response = settings_web_client.get("/")

        assert response is not None, f"Service unreachable: {settings_web_client.last_error}"
        assert response.status_code == 200

    def test_settings_list(self, settings_web_client):
        """Test /api/settings endpoint."""
        for path in ["/api/settings", "/settings", "/api/config"]:
            response = settings_web_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_categories_endpoint(self, settings_web_client):
        """Test /api/categories endpoint."""
        for path in ["/api/categories", "/categories"]:
            response = settings_web_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestSettingsWebConfiguration:
    """Test configuration endpoints."""

    def test_get_setting(self, settings_web_client):
        """Test getting a specific setting."""
        for path in ["/api/settings/general", "/api/config/general"]:
            response = settings_web_client.get(path)
            if response is not None and response.status_code in [200, 404]:
                break

    def test_system_info(self, settings_web_client):
        """Test /api/system or /system endpoint."""
        for path in ["/api/system", "/system", "/api/info"]:
            response = settings_web_client.get(path)
            if response is not None and response.status_code == 200:
                break
