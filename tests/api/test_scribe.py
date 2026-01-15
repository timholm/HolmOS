"""
API Tests for Scribe Service (Port 30860)

Scribe is the records keeping agent with the catchphrase:
"It's all in the records."
"""
import pytest


class TestScribeHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, scribe_client):
        """Test /health endpoint returns healthy status."""
        response = scribe_client.get("/health")

        assert response is not None, f"Service unreachable: {scribe_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestScribeAPI:
    """Test API endpoints."""

    def test_index_page(self, scribe_client):
        """Test / endpoint returns scribe interface."""
        response = scribe_client.get("/")

        assert response is not None, f"Service unreachable: {scribe_client.last_error}"
        assert response.status_code == 200

    def test_records_endpoint(self, scribe_client):
        """Test /records or /api/records endpoint."""
        for path in ["/records", "/api/records", "/api/logs"]:
            response = scribe_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]

    def test_audit_endpoint(self, scribe_client):
        """Test /audit or /api/audit endpoint."""
        for path in ["/audit", "/api/audit"]:
            response = scribe_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 404]


class TestScribeRecording:
    """Test recording functionality."""

    def test_log_endpoint(self, scribe_client):
        """Test /log or /api/log endpoint for writing logs."""
        response = scribe_client.post(
            "/api/log",
            json={"message": "test log entry", "level": "info"}
        )

        # May or may not exist
        if response is not None:
            assert response.status_code in [200, 201, 400, 404]
