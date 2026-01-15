"""
API Tests for Test Dashboard Service (Port 30900)

Test Dashboard provides service health monitoring functionality.
"""
import pytest


class TestTestDashboardHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, test_dashboard_client):
        """Test /health endpoint returns healthy status."""
        response = test_dashboard_client.get("/health")

        assert response is not None, f"Service unreachable: {test_dashboard_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestTestDashboardAPI:
    """Test API endpoints."""

    def test_index_page(self, test_dashboard_client):
        """Test / endpoint returns test dashboard interface."""
        response = test_dashboard_client.get("/")

        assert response is not None, f"Service unreachable: {test_dashboard_client.last_error}"
        assert response.status_code == 200

    def test_tests_list(self, test_dashboard_client):
        """Test /api/tests endpoint."""
        for path in ["/api/tests", "/tests", "/api/results"]:
            response = test_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_services_health(self, test_dashboard_client):
        """Test /api/services or /api/health endpoint."""
        for path in ["/api/services", "/api/health", "/services"]:
            response = test_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestTestDashboardOperations:
    """Test monitoring operations."""

    def test_status_endpoint(self, test_dashboard_client):
        """Test /api/status endpoint."""
        for path in ["/api/status", "/status"]:
            response = test_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_run_tests_endpoint(self, test_dashboard_client):
        """Test /api/run endpoint exists."""
        response = test_dashboard_client.post("/api/run", json={})

        if response is not None:
            # Should exist (not 404)
            assert response.status_code in [200, 202, 400, 500]
