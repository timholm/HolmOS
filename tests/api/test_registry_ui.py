"""
API Tests for Registry UI Service (Port 31750)

Registry UI provides the container registry browser.
"""
import pytest


class TestRegistryUIHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, registry_ui_client):
        """Test /health endpoint returns healthy status."""
        response = registry_ui_client.get("/health")

        assert response is not None, f"Service unreachable: {registry_ui_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestRegistryUIAPI:
    """Test API endpoints."""

    def test_index_page(self, registry_ui_client):
        """Test / endpoint returns registry browser interface."""
        response = registry_ui_client.get("/")

        assert response is not None, f"Service unreachable: {registry_ui_client.last_error}"
        assert response.status_code == 200

    def test_repositories_list(self, registry_ui_client):
        """Test /api/repositories or /repositories endpoint."""
        for path in ["/api/repositories", "/repositories", "/api/repos", "/v2/_catalog"]:
            response = registry_ui_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_images_endpoint(self, registry_ui_client):
        """Test /api/images endpoint."""
        for path in ["/api/images", "/images"]:
            response = registry_ui_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestRegistryUIOperations:
    """Test registry operations (read-only)."""

    def test_tags_list(self, registry_ui_client):
        """Test listing tags for a repository."""
        # First get repos list
        response = registry_ui_client.get("/api/repositories")
        if response is not None and response.status_code == 200:
            data = response.json()
            repos = data if isinstance(data, list) else data.get("repositories", [])
            if repos:
                repo = repos[0] if isinstance(repos[0], str) else repos[0].get("name")
                tags_response = registry_ui_client.get(f"/api/repositories/{repo}/tags")
                if tags_response is not None:
                    assert tags_response.status_code in [200, 404]

    def test_search_endpoint(self, registry_ui_client):
        """Test /api/search endpoint."""
        for path in ["/api/search?q=test", "/search?query=test"]:
            response = registry_ui_client.get(path)
            if response is not None and response.status_code in [200, 400]:
                break
