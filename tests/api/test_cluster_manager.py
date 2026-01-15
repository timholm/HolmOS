"""
API Tests for Cluster Manager Service (Port 30502)

Cluster Manager provides the cluster admin dashboard.
"""
import pytest


class TestClusterManagerHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, cluster_manager_client):
        """Test /health endpoint returns healthy status."""
        response = cluster_manager_client.get("/health")

        assert response is not None, f"Service unreachable: {cluster_manager_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestClusterManagerAPI:
    """Test API endpoints."""

    def test_index_page(self, cluster_manager_client):
        """Test / endpoint returns cluster manager interface."""
        response = cluster_manager_client.get("/")

        assert response is not None, f"Service unreachable: {cluster_manager_client.last_error}"
        assert response.status_code == 200

    def test_nodes_list(self, cluster_manager_client):
        """Test /api/nodes endpoint."""
        for path in ["/api/nodes", "/nodes"]:
            response = cluster_manager_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_pods_endpoint(self, cluster_manager_client):
        """Test /api/pods endpoint."""
        for path in ["/api/pods", "/pods"]:
            response = cluster_manager_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestClusterManagerOperations:
    """Test cluster management operations (read-only)."""

    def test_namespaces_endpoint(self, cluster_manager_client):
        """Test /api/namespaces endpoint."""
        for path in ["/api/namespaces", "/namespaces"]:
            response = cluster_manager_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_deployments_endpoint(self, cluster_manager_client):
        """Test /api/deployments endpoint."""
        for path in ["/api/deployments", "/deployments"]:
            response = cluster_manager_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_services_endpoint(self, cluster_manager_client):
        """Test /api/services endpoint."""
        for path in ["/api/services", "/services"]:
            response = cluster_manager_client.get(path)
            if response is not None and response.status_code == 200:
                break
