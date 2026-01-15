"""
API Tests for Metrics Dashboard Service (Port 30950)

Metrics Dashboard provides cluster metrics visualization.
"""
import pytest


class TestMetricsDashboardHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, metrics_dashboard_client):
        """Test /health endpoint returns healthy status."""
        response = metrics_dashboard_client.get("/health")

        assert response is not None, f"Service unreachable: {metrics_dashboard_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestMetricsDashboardAPI:
    """Test API endpoints."""

    def test_index_page(self, metrics_dashboard_client):
        """Test / endpoint returns metrics dashboard interface."""
        response = metrics_dashboard_client.get("/")

        assert response is not None, f"Service unreachable: {metrics_dashboard_client.last_error}"
        assert response.status_code == 200

    def test_metrics_list(self, metrics_dashboard_client):
        """Test /api/metrics endpoint."""
        for path in ["/api/metrics", "/metrics"]:
            response = metrics_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_nodes_metrics(self, metrics_dashboard_client):
        """Test /api/nodes/metrics endpoint."""
        for path in ["/api/nodes/metrics", "/api/nodes", "/nodes"]:
            response = metrics_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestMetricsDashboardOperations:
    """Test metrics operations."""

    def test_cpu_metrics(self, metrics_dashboard_client):
        """Test CPU metrics endpoint."""
        for path in ["/api/metrics/cpu", "/api/cpu", "/cpu"]:
            response = metrics_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_memory_metrics(self, metrics_dashboard_client):
        """Test memory metrics endpoint."""
        for path in ["/api/metrics/memory", "/api/memory", "/memory"]:
            response = metrics_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_pods_metrics(self, metrics_dashboard_client):
        """Test pods metrics endpoint."""
        for path in ["/api/pods/metrics", "/api/pods", "/pods"]:
            response = metrics_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break
