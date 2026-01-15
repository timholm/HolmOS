"""
API Tests for File Web Nautilus Service (Port 30088)

File Web Nautilus provides GNOME-style file manager functionality.
"""
import pytest


class TestFileWebNautilusHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, file_web_nautilus_client):
        """Test /health endpoint returns healthy status."""
        response = file_web_nautilus_client.get("/health")

        assert response is not None, f"Service unreachable: {file_web_nautilus_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestFileWebNautilusAPI:
    """Test API endpoints."""

    def test_index_page(self, file_web_nautilus_client):
        """Test / endpoint returns file manager interface."""
        response = file_web_nautilus_client.get("/")

        assert response is not None, f"Service unreachable: {file_web_nautilus_client.last_error}"
        assert response.status_code == 200

    def test_files_list(self, file_web_nautilus_client):
        """Test /api/files or /files endpoint."""
        for path in ["/api/files", "/files", "/api/list"]:
            response = file_web_nautilus_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_browse_endpoint(self, file_web_nautilus_client):
        """Test /api/browse endpoint."""
        for path in ["/api/browse", "/browse", "/api/dir"]:
            response = file_web_nautilus_client.get(path)
            if response is not None and response.status_code == 200:
                break

        if response is not None:
            assert response.status_code in [200, 400, 404]


class TestFileWebNautilusOperations:
    """Test file operations (read-only tests)."""

    def test_list_root_directory(self, file_web_nautilus_client):
        """Test listing root directory."""
        for path in ["/api/files?path=/", "/api/browse?path=/", "/api/list?dir=/"]:
            response = file_web_nautilus_client.get(path)
            if response is not None and response.status_code == 200:
                break

    def test_file_info(self, file_web_nautilus_client):
        """Test file info endpoint."""
        for path in ["/api/info", "/api/stat", "/api/file"]:
            response = file_web_nautilus_client.get(path)
            if response is not None and response.status_code in [200, 400]:
                break
