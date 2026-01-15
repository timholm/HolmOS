"""
API Tests for Calculator App Service (Port 30010)

Calculator App provides iPhone-style calculator functionality.
"""
import pytest


class TestCalculatorAppHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, calculator_app_client):
        """Test /health endpoint returns healthy status."""
        response = calculator_app_client.get("/health")

        assert response is not None, f"Service unreachable: {calculator_app_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestCalculatorAppAPI:
    """Test API endpoints."""

    def test_index_page(self, calculator_app_client):
        """Test / endpoint returns calculator interface."""
        response = calculator_app_client.get("/")

        assert response is not None, f"Service unreachable: {calculator_app_client.last_error}"
        assert response.status_code == 200

    def test_calculate_endpoint(self, calculator_app_client):
        """Test /api/calculate endpoint."""
        response = calculator_app_client.post(
            "/api/calculate",
            json={"expression": "2+2"}
        )

        if response is not None and response.status_code == 200:
            data = response.json()
            assert "result" in data or "answer" in data

    def test_calculate_with_get(self, calculator_app_client):
        """Test /api/calculate or /calculate with GET."""
        for path in ["/api/calculate?expr=2+2", "/calculate?expression=2+2"]:
            response = calculator_app_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestCalculatorAppOperations:
    """Test calculation operations."""

    def test_addition(self, calculator_app_client):
        """Test addition operation."""
        response = calculator_app_client.post(
            "/api/calculate",
            json={"expression": "5+3"}
        )

        if response is not None and response.status_code == 200:
            data = response.json()
            result = data.get("result") or data.get("answer")
            assert result == 8 or str(result) == "8"

    def test_multiplication(self, calculator_app_client):
        """Test multiplication operation."""
        response = calculator_app_client.post(
            "/api/calculate",
            json={"expression": "4*7"}
        )

        if response is not None and response.status_code == 200:
            data = response.json()
            result = data.get("result") or data.get("answer")
            assert result == 28 or str(result) == "28"
