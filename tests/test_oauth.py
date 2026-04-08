"""Tests for IracingOAuthClient"""
import base64
import hashlib
import json
import os
import pytest
from unittest.mock import patch, MagicMock
from urllib.parse import urlparse, parse_qs

from oauth_client import IracingOAuthClient


@pytest.fixture
def client(tmp_path, monkeypatch):
    """Fresh OAuth client with token file isolated to a temp directory."""
    monkeypatch.chdir(tmp_path)
    return IracingOAuthClient()


# ── PKCE ──────────────────────────────────────────────────────────────────

def test_pkce_verifier_is_url_safe():
    verifier = IracingOAuthClient._generate_pkce_verifier()
    # URL-safe base64 chars only (no +, /, =)
    assert all(c in "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_" for c in verifier)

def test_pkce_challenge_matches_verifier():
    verifier = IracingOAuthClient._generate_pkce_verifier()
    challenge = IracingOAuthClient._generate_pkce_challenge(verifier)
    expected = base64.urlsafe_b64encode(
        hashlib.sha256(verifier.encode()).digest()
    ).decode().rstrip("=")
    assert challenge == expected

def test_pkce_challenge_has_no_padding():
    verifier = IracingOAuthClient._generate_pkce_verifier()
    challenge = IracingOAuthClient._generate_pkce_challenge(verifier)
    assert "=" not in challenge


# ── Auth URL ──────────────────────────────────────────────────────────────

def test_generate_auth_url_required_params(client):
    url = client.generate_auth_url()
    parsed = urlparse(url)
    params = parse_qs(parsed.query)

    assert params["client_id"] == ["apex-api-explorer"]
    assert params["response_type"] == ["code"]
    assert params["code_challenge_method"] == ["S256"]
    assert "code_challenge" in params
    assert "state" in params
    assert "redirect_uri" in params

def test_generate_auth_url_sets_state(client):
    client.generate_auth_url()
    assert client.state is not None
    assert len(client.state) > 0

def test_generate_auth_url_sets_code_verifier(client):
    client.generate_auth_url()
    assert client.code_verifier is not None


# ── Token management ──────────────────────────────────────────────────────

def test_not_authenticated_with_no_token(client):
    assert client.is_authenticated() is False

def test_get_access_token_returns_none_when_no_token(client):
    assert client.get_access_token() is None

def test_save_and_load_token(client, tmp_path):
    token = {"access_token": "abc123", "refresh_token": "refresh456"}
    client._save_token(token)
    assert client.get_access_token() == "abc123"
    assert client.is_authenticated() is True

def test_clear_token(client, tmp_path):
    client._save_token({"access_token": "abc123"})
    client.clear_token()
    assert client.is_authenticated() is False
    assert not os.path.exists(".iracing_token")


# ── Callback ──────────────────────────────────────────────────────────────

def test_handle_callback_raises_on_state_mismatch(client):
    client.generate_auth_url()
    with pytest.raises(ValueError, match="State mismatch"):
        client.handle_callback(code="somecode", state="wrongstate")
