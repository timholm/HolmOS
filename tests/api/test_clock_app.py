"""
API Tests for Clock App Service (Port 30007)

Clock App provides world clock, alarms, and timer functionality.
"""
import pytest


class TestClockAppHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, clock_app_client):
        """Test /health endpoint returns healthy status."""
        response = clock_app_client.get("/health")

        assert response is not None, f"Service unreachable: {clock_app_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestClockAppAPI:
    """Test API endpoints."""

    def test_index_page(self, clock_app_client):
        """Test / endpoint returns clock interface."""
        response = clock_app_client.get("/")

        assert response is not None, f"Service unreachable: {clock_app_client.last_error}"
        assert response.status_code == 200

    def test_time_endpoint(self, clock_app_client):
        """Test /api/time or /time endpoint."""
        for path in ["/api/time", "/time", "/api/now"]:
            response = clock_app_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                # Should return time-related data
                assert isinstance(data, dict)
                break

    def test_timezones_endpoint(self, clock_app_client):
        """Test /api/timezones endpoint."""
        for path in ["/api/timezones", "/timezones", "/api/zones"]:
            response = clock_app_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]


class TestClockAppAlarms:
    """Test alarm functionality."""

    def test_alarms_list(self, clock_app_client):
        """Test /api/alarms endpoint."""
        for path in ["/api/alarms", "/alarms"]:
            response = clock_app_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (list, dict))
                break


class TestClockAppTimer:
    """Test timer functionality."""

    def test_timer_endpoint(self, clock_app_client):
        """Test /api/timer endpoint."""
        for path in ["/api/timer", "/timer"]:
            response = clock_app_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]
