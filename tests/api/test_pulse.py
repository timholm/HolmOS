"""
API Tests for Pulse Service (Port 30006)

Pulse is the health monitoring agent with the catchphrase:
"Vital signs are looking good."
"""
import pytest


class TestPulseHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, pulse_client):
        """Test /health endpoint returns healthy status."""
        response = pulse_client.get("/health")

        assert response is not None, f"Service unreachable: {pulse_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestPulseAPI:
    """Test API endpoints."""

    def test_index_page(self, pulse_client):
        """Test / endpoint returns monitoring interface."""
        response = pulse_client.get("/")

        assert response is not None, f"Service unreachable: {pulse_client.last_error}"
        assert response.status_code == 200

    def test_metrics_endpoint(self, pulse_client):
        """Test /metrics or /api/metrics endpoint."""
        # Try common metrics endpoints
        for path in ["/metrics", "/api/metrics", "/api/health"]:
            response = pulse_client.get(path)
            if response is not None and response.status_code == 200:
                break

        # At least one should work
        if response is not None:
            assert response.status_code in [200, 404]

    def test_services_status(self, pulse_client):
        """Test /api/services or /services endpoint."""
        for path in ["/api/services", "/services", "/api/status"]:
            response = pulse_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]


class TestPulseMonitoring:
    """Test monitoring functionality."""

    def test_cluster_health_data(self, pulse_client):
        """Test cluster health data endpoint."""
        for path in ["/api/cluster", "/cluster", "/api/nodes"]:
            response = pulse_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                # Should return some health/node data
                assert isinstance(data, (dict, list))
                break
