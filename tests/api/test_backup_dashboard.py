"""
API Tests for Backup Dashboard Service (Port 30850)

Backup Dashboard provides backup management functionality.
"""
import pytest


class TestBackupDashboardHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, backup_dashboard_client):
        """Test /health endpoint returns healthy status."""
        response = backup_dashboard_client.get("/health")

        assert response is not None, f"Service unreachable: {backup_dashboard_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestBackupDashboardAPI:
    """Test API endpoints."""

    def test_index_page(self, backup_dashboard_client):
        """Test / endpoint returns backup dashboard interface."""
        response = backup_dashboard_client.get("/")

        assert response is not None, f"Service unreachable: {backup_dashboard_client.last_error}"
        assert response.status_code == 200

    def test_backups_list(self, backup_dashboard_client):
        """Test /api/backups endpoint."""
        for path in ["/api/backups", "/backups"]:
            response = backup_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_schedules_endpoint(self, backup_dashboard_client):
        """Test /api/schedules endpoint."""
        for path in ["/api/schedules", "/schedules"]:
            response = backup_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestBackupDashboardOperations:
    """Test backup operations (read-only)."""

    def test_status_endpoint(self, backup_dashboard_client):
        """Test /api/status endpoint."""
        for path in ["/api/status", "/status"]:
            response = backup_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_history_endpoint(self, backup_dashboard_client):
        """Test /api/history endpoint."""
        for path in ["/api/history", "/history"]:
            response = backup_dashboard_client.get(path)
            if response is not None and response.status_code == 200:
                break
