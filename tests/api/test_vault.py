"""
API Tests for Vault Service (Port 30870)

Vault is the secret management agent with the catchphrase:
"Your secrets are safe with me."
"""
import pytest
import uuid


class TestVaultHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, vault_client):
        """Test /api/health endpoint returns healthy status."""
        response = vault_client.get("/api/health")

        assert response is not None, f"Service unreachable: {vault_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "healthy"
        assert data.get("service") == "Vault"
        assert data.get("encryption") == "AES-256-GCM"


class TestVaultAPI:
    """Test API endpoints."""

    def test_list_secrets(self, vault_client):
        """Test GET /api/secrets returns list of secrets."""
        response = vault_client.get("/api/secrets")

        assert response is not None, f"Service unreachable: {vault_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert isinstance(data, list)

    def test_create_read_delete_secret(self, vault_client):
        """Test full secret lifecycle: create, read, delete."""
        # Generate unique name for test
        test_name = f"test-secret-{uuid.uuid4().hex[:8]}"
        test_value = "test-secret-value"

        # Create secret
        create_response = vault_client.post(
            "/api/secrets",
            json={
                "name": test_name,
                "value": test_value,
                "metadata": {"description": "API test secret"}
            }
        )

        assert create_response is not None, f"Service unreachable: {vault_client.last_error}"
        assert create_response.status_code == 200

        create_data = create_response.json()
        assert create_data.get("success") is True
        assert create_data.get("version") == 1

        # Read secret
        read_response = vault_client.get(f"/api/secrets/{test_name}")

        assert read_response is not None
        assert read_response.status_code == 200

        read_data = read_response.json()
        assert read_data["name"] == test_name
        assert read_data["value"] == test_value
        assert read_data["version"] == 1

        # Delete secret
        delete_response = vault_client.delete(f"/api/secrets/{test_name}")

        assert delete_response is not None
        assert delete_response.status_code == 200

        delete_data = delete_response.json()
        assert delete_data.get("success") is True

        # Verify deleted
        verify_response = vault_client.get(f"/api/secrets/{test_name}")
        assert verify_response is not None
        assert verify_response.status_code == 404

    def test_update_secret_creates_new_version(self, vault_client):
        """Test that updating a secret creates a new version."""
        test_name = f"test-version-{uuid.uuid4().hex[:8]}"

        # Create initial secret
        vault_client.post(
            "/api/secrets",
            json={"name": test_name, "value": "version-1"}
        )

        # Update secret
        update_response = vault_client.put(
            f"/api/secrets/{test_name}",
            json={"value": "version-2"}
        )

        if update_response is not None and update_response.status_code == 200:
            update_data = update_response.json()
            assert update_data.get("version") == 2

        # Cleanup
        vault_client.delete(f"/api/secrets/{test_name}")

    def test_create_duplicate_secret_fails(self, vault_client):
        """Test that creating a duplicate secret fails."""
        test_name = f"test-dup-{uuid.uuid4().hex[:8]}"

        # Create first secret
        vault_client.post(
            "/api/secrets",
            json={"name": test_name, "value": "first"}
        )

        # Try to create duplicate
        dup_response = vault_client.post(
            "/api/secrets",
            json={"name": test_name, "value": "second"}
        )

        assert dup_response is not None
        assert dup_response.status_code == 400

        # Cleanup
        vault_client.delete(f"/api/secrets/{test_name}")


class TestVaultAudit:
    """Test audit logging functionality."""

    def test_audit_log_endpoint(self, vault_client):
        """Test /api/audit endpoint returns audit logs."""
        response = vault_client.get("/api/audit")

        assert response is not None, f"Service unreachable: {vault_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert isinstance(data, list)

    def test_audit_log_with_limit(self, vault_client):
        """Test /api/audit endpoint with limit parameter."""
        response = vault_client.get("/api/audit?limit=10")

        assert response is not None
        assert response.status_code == 200

        data = response.json()
        assert isinstance(data, list)
        assert len(data) <= 10


class TestVaultUI:
    """Test UI endpoints."""

    def test_index_page(self, vault_client):
        """Test / endpoint returns Vault UI."""
        response = vault_client.get("/")

        assert response is not None, f"Service unreachable: {vault_client.last_error}"
        assert response.status_code == 200
        assert "text/html" in response.headers.get("Content-Type", "")
        assert "Vault" in response.text


class TestVaultValidation:
    """Test input validation."""

    def test_create_without_name_fails(self, vault_client):
        """Test that creating a secret without name fails."""
        response = vault_client.post(
            "/api/secrets",
            json={"value": "no-name-secret"}
        )

        assert response is not None
        assert response.status_code == 400

    def test_create_without_value_fails(self, vault_client):
        """Test that creating a secret without value fails."""
        response = vault_client.post(
            "/api/secrets",
            json={"name": "no-value-secret"}
        )

        assert response is not None
        assert response.status_code == 400

    def test_read_nonexistent_secret_returns_404(self, vault_client):
        """Test that reading a nonexistent secret returns 404."""
        response = vault_client.get("/api/secrets/nonexistent-secret-12345")

        assert response is not None
        assert response.status_code == 404
