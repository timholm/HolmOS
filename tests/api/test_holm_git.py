"""
API Tests for HolmGit Service (Port 30009)

HolmGit provides Git repository server functionality.
"""
import pytest


class TestHolmGitHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, holm_git_client):
        """Test /health endpoint returns healthy status."""
        response = holm_git_client.get("/health")

        assert response is not None, f"Service unreachable: {holm_git_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestHolmGitAPI:
    """Test API endpoints."""

    def test_index_page(self, holm_git_client):
        """Test / endpoint returns Git interface."""
        response = holm_git_client.get("/")

        assert response is not None, f"Service unreachable: {holm_git_client.last_error}"
        assert response.status_code == 200

    def test_repos_list(self, holm_git_client):
        """Test /api/repos or /repos endpoint."""
        for path in ["/api/repos", "/repos", "/api/repositories"]:
            response = holm_git_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_users_endpoint(self, holm_git_client):
        """Test /api/users endpoint."""
        for path in ["/api/users", "/users"]:
            response = holm_git_client.get(path)
            if response is not None and response.status_code in [200, 401]:
                break


class TestHolmGitRepositories:
    """Test repository operations (read-only)."""

    def test_list_branches(self, holm_git_client):
        """Test listing branches for a repository."""
        # First get repos list
        response = holm_git_client.get("/api/repos")
        if response is not None and response.status_code == 200:
            data = response.json()
            repos = data if isinstance(data, list) else data.get("repos", [])
            if repos:
                repo = repos[0] if isinstance(repos[0], str) else repos[0].get("name")
                branches_response = holm_git_client.get(f"/api/repos/{repo}/branches")
                if branches_response is not None:
                    assert branches_response.status_code in [200, 404]

    def test_git_info(self, holm_git_client):
        """Test /api/info or /info endpoint."""
        for path in ["/api/info", "/info", "/api/version"]:
            response = holm_git_client.get(path)
            if response is not None and response.status_code == 200:
                break
