"""
API Tests for Gateway Service (Port 30008)

Gateway is the routing agent with the catchphrase:
"All roads lead through me."
"""
import pytest


class TestGatewayHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, gateway_client):
        """Test /health endpoint returns healthy status."""
        response = gateway_client.get("/health")

        assert response is not None, f"Service unreachable: {gateway_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestGatewayAPI:
    """Test API endpoints."""

    def test_index_page(self, gateway_client):
        """Test / endpoint returns gateway interface."""
        response = gateway_client.get("/")

        assert response is not None, f"Service unreachable: {gateway_client.last_error}"
        assert response.status_code == 200

    def test_routes_endpoint(self, gateway_client):
        """Test /routes or /api/routes endpoint."""
        for path in ["/routes", "/api/routes", "/api/services"]:
            response = gateway_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_status_endpoint(self, gateway_client):
        """Test /status or /api/status endpoint."""
        for path in ["/status", "/api/status"]:
            response = gateway_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]


class TestGatewayRouting:
    """Test routing functionality."""

    def test_proxy_headers(self, gateway_client):
        """Test that gateway handles proxy headers correctly."""
        headers = {
            "X-Forwarded-For": "10.0.0.1",
            "X-Forwarded-Proto": "https"
        }
        response = gateway_client.get("/", headers=headers)

        if response is not None:
            assert response.status_code == 200
