"""
HolmOS API Test Configuration and Fixtures
"""
import pytest
import requests
import yaml
import os
from datetime import datetime
from typing import Dict, Any, Optional


# Load services configuration
SERVICES_CONFIG_PATH = "/Users/tim/HolmOS/services.yaml"

def load_services_config() -> Dict[str, Any]:
    """Load the services.yaml configuration file."""
    with open(SERVICES_CONFIG_PATH, 'r') as f:
        return yaml.safe_load(f)


@pytest.fixture(scope="session")
def services_config():
    """Provide services configuration to tests."""
    return load_services_config()


@pytest.fixture(scope="session")
def base_url():
    """Base URL for the cluster. Override with HOLMOS_HOST env var."""
    host = os.environ.get("HOLMOS_HOST", "192.168.8.197")
    return f"http://{host}"


@pytest.fixture(scope="session")
def request_timeout():
    """Default request timeout in seconds."""
    return int(os.environ.get("HOLMOS_TIMEOUT", "10"))


class ServiceTestClient:
    """HTTP client for testing HolmOS services."""

    def __init__(self, base_url: str, port: int, timeout: int = 10):
        self.base_url = f"{base_url}:{port}"
        self.timeout = timeout
        self.session = requests.Session()
        self.last_response = None
        self.last_error = None

    def get(self, path: str, **kwargs) -> Optional[requests.Response]:
        """Make a GET request."""
        kwargs.setdefault("timeout", self.timeout)
        try:
            self.last_response = self.session.get(f"{self.base_url}{path}", **kwargs)
            self.last_error = None
            return self.last_response
        except Exception as e:
            self.last_error = e
            self.last_response = None
            return None

    def post(self, path: str, **kwargs) -> Optional[requests.Response]:
        """Make a POST request."""
        kwargs.setdefault("timeout", self.timeout)
        try:
            self.last_response = self.session.post(f"{self.base_url}{path}", **kwargs)
            self.last_error = None
            return self.last_response
        except Exception as e:
            self.last_error = e
            self.last_response = None
            return None

    def put(self, path: str, **kwargs) -> Optional[requests.Response]:
        """Make a PUT request."""
        kwargs.setdefault("timeout", self.timeout)
        try:
            self.last_response = self.session.put(f"{self.base_url}{path}", **kwargs)
            self.last_error = None
            return self.last_response
        except Exception as e:
            self.last_error = e
            self.last_response = None
            return None

    def delete(self, path: str, **kwargs) -> Optional[requests.Response]:
        """Make a DELETE request."""
        kwargs.setdefault("timeout", self.timeout)
        try:
            self.last_response = self.session.delete(f"{self.base_url}{path}", **kwargs)
            self.last_error = None
            return self.last_response
        except Exception as e:
            self.last_error = e
            self.last_response = None
            return None

    def is_healthy(self, health_path: str = "/health") -> bool:
        """Check if the service is healthy."""
        response = self.get(health_path)
        return response is not None and response.status_code == 200

    def close(self):
        """Close the session."""
        self.session.close()


@pytest.fixture
def client_factory(base_url, request_timeout):
    """Factory for creating service test clients."""
    clients = []

    def create_client(port: int) -> ServiceTestClient:
        client = ServiceTestClient(base_url, port, request_timeout)
        clients.append(client)
        return client

    yield create_client

    # Cleanup
    for client in clients:
        client.close()


# Service-specific fixtures
@pytest.fixture
def holmos_shell_client(client_factory):
    """Client for holmos-shell service (port 30000)."""
    return client_factory(30000)


@pytest.fixture
def claude_pod_client(client_factory):
    """Client for claude-pod service (port 30001)."""
    return client_factory(30001)


@pytest.fixture
def app_store_client(client_factory):
    """Client for app-store service (port 30002)."""
    return client_factory(30002)


@pytest.fixture
def chat_hub_client(client_factory):
    """Client for chat-hub service (port 30003)."""
    return client_factory(30003)


@pytest.fixture
def nova_client(client_factory):
    """Client for nova service (port 30004)."""
    return client_factory(30004)


@pytest.fixture
def merchant_client(client_factory):
    """Client for merchant service (port 30005)."""
    return client_factory(30005)


@pytest.fixture
def pulse_client(client_factory):
    """Client for pulse service (port 30006)."""
    return client_factory(30006)


@pytest.fixture
def clock_app_client(client_factory):
    """Client for clock-app service (port 30007)."""
    return client_factory(30007)


@pytest.fixture
def gateway_client(client_factory):
    """Client for gateway service (port 30008)."""
    return client_factory(30008)


@pytest.fixture
def holm_git_client(client_factory):
    """Client for holm-git service (port 30009)."""
    return client_factory(30009)


@pytest.fixture
def calculator_app_client(client_factory):
    """Client for calculator-app service (port 30010)."""
    return client_factory(30010)


@pytest.fixture
def cicd_controller_client(client_factory):
    """Client for cicd-controller service (port 30020)."""
    return client_factory(30020)


@pytest.fixture
def deploy_controller_client(client_factory):
    """Client for deploy-controller service (port 30021)."""
    return client_factory(30021)


@pytest.fixture
def file_web_nautilus_client(client_factory):
    """Client for file-web-nautilus service (port 30088)."""
    return client_factory(30088)


@pytest.fixture
def cluster_manager_client(client_factory):
    """Client for cluster-manager service (port 30502)."""
    return client_factory(30502)


@pytest.fixture
def settings_web_client(client_factory):
    """Client for settings-web service (port 30600)."""
    return client_factory(30600)


@pytest.fixture
def audiobook_web_client(client_factory):
    """Client for audiobook-web service (port 30700)."""
    return client_factory(30700)


@pytest.fixture
def terminal_web_client(client_factory):
    """Client for terminal-web service (port 30800)."""
    return client_factory(30800)


@pytest.fixture
def backup_dashboard_client(client_factory):
    """Client for backup-dashboard service (port 30850)."""
    return client_factory(30850)


@pytest.fixture
def scribe_client(client_factory):
    """Client for scribe service (port 30860)."""
    return client_factory(30860)


@pytest.fixture
def vault_client(client_factory):
    """Client for vault service (port 30870)."""
    return client_factory(30870)


@pytest.fixture
def test_dashboard_client(client_factory):
    """Client for test-dashboard service (port 30900)."""
    return client_factory(30900)


@pytest.fixture
def metrics_dashboard_client(client_factory):
    """Client for metrics-dashboard service (port 30950)."""
    return client_factory(30950)


@pytest.fixture
def registry_ui_client(client_factory):
    """Client for registry-ui service (port 31750)."""
    return client_factory(31750)
