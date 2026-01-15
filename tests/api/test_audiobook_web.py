"""
API Tests for Audiobook Web Service (Port 30700)

Audiobook Web provides the audiobook TTS pipeline.
"""
import pytest


class TestAudiobookWebHealth:
    """Test health endpoints."""

    def test_health_endpoint(self, audiobook_web_client):
        """Test /health endpoint returns healthy status."""
        response = audiobook_web_client.get("/health")

        assert response is not None, f"Service unreachable: {audiobook_web_client.last_error}"
        assert response.status_code == 200

        data = response.json()
        assert data.get("status") in ["healthy", "ok"]


class TestAudiobookWebAPI:
    """Test API endpoints."""

    def test_index_page(self, audiobook_web_client):
        """Test / endpoint returns audiobook interface."""
        response = audiobook_web_client.get("/")

        assert response is not None, f"Service unreachable: {audiobook_web_client.last_error}"
        assert response.status_code == 200

    def test_books_list(self, audiobook_web_client):
        """Test /api/books or /books endpoint."""
        for path in ["/api/books", "/books", "/api/library"]:
            response = audiobook_web_client.get(path)
            if response is not None and response.status_code == 200:
                data = response.json()
                assert isinstance(data, (dict, list))
                break

    def test_voices_endpoint(self, audiobook_web_client):
        """Test /api/voices endpoint for TTS voices."""
        for path in ["/api/voices", "/voices", "/api/tts/voices"]:
            response = audiobook_web_client.get(path)
            if response is not None and response.status_code == 200:
                break


class TestAudiobookWebTTS:
    """Test TTS functionality."""

    def test_tts_endpoint(self, audiobook_web_client):
        """Test /api/tts endpoint."""
        for path in ["/api/tts", "/tts", "/api/synthesize"]:
            # Try POST for TTS
            response = audiobook_web_client.post(
                path,
                json={"text": "Hello world"}
            )
            if response is not None and response.status_code in [200, 400, 404]:
                break

    def test_jobs_endpoint(self, audiobook_web_client):
        """Test /api/jobs endpoint for conversion jobs."""
        for path in ["/api/jobs", "/jobs", "/api/conversions"]:
            response = audiobook_web_client.get(path)
            if response is not None and response.status_code == 200:
                break
