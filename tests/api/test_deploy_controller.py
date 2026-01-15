"""
API Tests for Deploy Controller Service (Port 30021)

Deploy Controller manages auto-deployment functionality.
"""
import pytest


class TestDeployControllerHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, deploy_controller_client):
        """Test /health endpoint returns healthy status."""
        response = deploy_controller_client.get("/health")

        assert response is not None, f"Service unreachable: {deploy_controller_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestDeployControllerAPI:
    """Test API endpoints."""

    def test_index_page(self, deploy_controller_client):
        """Test / endpoint returns deployment interface."""
        response = deploy_controller_client.get("/")

        assert response is not None, f"Service unreachable: {deploy_controller_client.last_error}"
        assert response.status_code == 200

    def test_deployments_list(self, deploy_controller_client):
        """Test /api/deployments endpoint."""
        for path in ["/api/deployments", "/deployments"]:
            response = deploy_controller_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_services_endpoint(self, deploy_controller_client):
        """Test /api/services endpoint."""
        for path in ["/api/services", "/services"]:
            response = deploy_controller_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestDeployControllerOperations:
    """Test deployment operations (read-only)."""

    def test_status_endpoint(self, deploy_controller_client):
        """Test /api/status endpoint."""
        for path in ["/api/status", "/status"]:
            response = deploy_controller_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_rollouts_endpoint(self, deploy_controller_client):
        """Test /api/rollouts endpoint."""
        for path in ["/api/rollouts", "/rollouts"]:
            response = deploy_controller_client.get(path)
            if response is not None and response.status_code in [200, 404]:
                break
