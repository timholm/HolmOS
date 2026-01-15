"""
API Tests for CI/CD Controller Service (Port 30020)

CI/CD Controller manages the CI/CD pipeline.
"""
import pytest


class TestCICDControllerHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, cicd_controller_client):
        """Test /health endpoint returns healthy status."""
        response = cicd_controller_client.get("/health")

        assert response is not None, f"Service unreachable: {cicd_controller_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestCICDControllerAPI:
    """Test API endpoints."""

    def test_index_page(self, cicd_controller_client):
        """Test / endpoint returns CI/CD interface."""
        response = cicd_controller_client.get("/")

        assert response is not None, f"Service unreachable: {cicd_controller_client.last_error}"
        assert response.status_code == 200

    def test_pipelines_list(self, cicd_controller_client):
        """Test /api/pipelines or /pipelines endpoint."""
        for path in ["/api/pipelines", "/pipelines", "/api/builds"]:
            response = cicd_controller_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_jobs_endpoint(self, cicd_controller_client):
        """Test /api/jobs endpoint."""
        for path in ["/api/jobs", "/jobs"]:
            response = cicd_controller_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestCICDControllerOperations:
    """Test CI/CD operations (read-only)."""

    def test_status_endpoint(self, cicd_controller_client):
        """Test /api/status endpoint."""
        for path in ["/api/status", "/status"]:
            response = cicd_controller_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_history_endpoint(self, cicd_controller_client):
        """Test /api/history endpoint."""
        for path in ["/api/history", "/history", "/api/runs"]:
            response = cicd_controller_client.get(path)
            if response is not None and response.status_code == 200:
                break
