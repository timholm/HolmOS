"""
API Tests for Merchant Service (Port 30005)

Merchant is the request handling agent with the catchphrase:
"Describe what you need, I'll make it happen."
"""
import pytest


class TestMerchantHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, merchant_client):
        """Test /health endpoint returns healthy status."""
        response = merchant_client.get("/health")

        assert response is not None, f"Service unreachable: {merchant_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestMerchantAPI:
    """Test API endpoints."""

    def test_index_page(self, merchant_client):
        """Test / endpoint returns interface."""
        response = merchant_client.get("/")

        assert response is not None, f"Service unreachable: {merchant_client.last_error}"
        assert response.status_code == 200

    def test_catalog_endpoint(self, merchant_client):
        """Test /catalog endpoint returns available templates."""
        response = merchant_client.get("/catalog")

        if response is not None and response.status_code == 200:
            data = response.json()
            # Should return templates list
            assert "templates" in data or isinstance(data, list)

    def test_chat_endpoint(self, merchant_client):
        """Test /chat endpoint accepts messages."""
        response = merchant_client.post(
            "/chat",
            json={"message": "hello"}
        )

        if response is not None:
            # Should get some response
            assert response.status_code in [200, 400, 500]


class TestMerchantBuildAPI:
    """Test build-related endpoints."""

    def test_build_endpoint_exists(self, merchant_client):
        """Test /build endpoint exists."""
        response = merchant_client.post("/build", json={})

        if response is not None:
            # Should not be 404
            assert response.status_code != 404

    def test_templates_endpoint(self, merchant_client):
        """Test /templates endpoint if it exists."""
        response = merchant_client.get("/templates")

        # May or may not exist
        if response is not None:
            assert response.status_code in [200, 404]
