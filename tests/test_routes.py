"""Tests for Flask routes"""
import json
import pytest
from unittest.mock import patch, MagicMock

import app as flask_app

SAMPLE_DOCS = {
    "member": {
        "get": {
            "link": "https://members-ng.iracing.com/data/member/get",
            "parameters": {"cust_ids": {"type": "number", "required": True}},
        }
    }
}


@pytest.fixture(autouse=True)
def patch_docs():
    """Use a fixed docs dict for all route tests."""
    flask_app.API_DOCS = SAMPLE_DOCS


@pytest.fixture
def client():
    flask_app.app.config["TESTING"] = True
    with flask_app.app.test_client() as c:
        yield c


@pytest.fixture
def authed_client(client, monkeypatch):
    """Client with a mocked authenticated oauth_client."""
    monkeypatch.setattr(flask_app.oauth_client, "is_authenticated", lambda: True)
    monkeypatch.setattr(flask_app.oauth_client, "get_access_token", lambda: "fake-token")
    return client


# ── Auth status ───────────────────────────────────────────────────────────

def test_auth_status_unauthenticated(client, monkeypatch):
    monkeypatch.setattr(flask_app.oauth_client, "is_authenticated", lambda: False)
    res = client.get("/api/auth-status")
    assert res.status_code == 200
    assert res.get_json()["authenticated"] is False

def test_auth_status_authenticated(authed_client):
    res = authed_client.get("/api/auth-status")
    assert res.status_code == 200
    assert res.get_json()["authenticated"] is True


# ── Docs ──────────────────────────────────────────────────────────────────

def test_docs_returns_api_docs(client):
    res = client.get("/api/docs")
    assert res.status_code == 200
    assert res.get_json() == SAMPLE_DOCS


# ── Proxy — auth guard ────────────────────────────────────────────────────

def test_proxy_requires_auth(client, monkeypatch):
    monkeypatch.setattr(flask_app.oauth_client, "is_authenticated", lambda: False)
    res = client.get("/api/proxy?_endpoint=member/get")
    assert res.status_code == 401

def test_proxy_requires_endpoint_param(authed_client):
    res = authed_client.get("/api/proxy")
    assert res.status_code == 400
    assert "Missing _endpoint" in res.get_json()["error"]

def test_proxy_rejects_unknown_endpoint(authed_client):
    res = authed_client.get("/api/proxy?_endpoint=fake/endpoint")
    assert res.status_code == 404

def test_proxy_rejects_malformed_endpoint(authed_client):
    res = authed_client.get("/api/proxy?_endpoint=nodslash")
    assert res.status_code == 400


# ── Proxy — successful call ───────────────────────────────────────────────

def test_proxy_calls_iracing_and_returns_data(authed_client):
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.json.return_value = {"some": "data"}
    mock_response.raise_for_status = MagicMock()

    with patch("app.requests.get", return_value=mock_response):
        res = authed_client.get("/api/proxy?_endpoint=member/get&cust_ids=12345")

    assert res.status_code == 200
    body = res.get_json()
    assert body["type"] == "direct"
    assert body["data"] == {"some": "data"}


# ── Index ─────────────────────────────────────────────────────────────────

def test_index_returns_200(client):
    res = client.get("/")
    assert res.status_code == 200
