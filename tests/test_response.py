"""Tests for process_api_response and fetch_s3_data"""
import pytest
from unittest.mock import patch, MagicMock

from app import process_api_response


# ── Direct response ───────────────────────────────────────────────────────

def test_direct_response_passthrough():
    data = {"some_key": "some_value"}
    result = process_api_response(data)
    assert result["type"] == "direct"
    assert result["data"] == data

def test_direct_response_list():
    data = [{"id": 1}, {"id": 2}]
    result = process_api_response(data)
    assert result["type"] == "direct"
    assert result["data"] == data

def test_empty_response_returns_error():
    assert process_api_response(None)["error"] == "Empty response"
    assert process_api_response({})["error"] == "Empty response"


# ── Simple S3 response ────────────────────────────────────────────────────

def test_simple_s3_fetches_link():
    s3_payload = {"link": "https://s3.example.com/data.json", "expires": "2026-01-01T00:00:00Z"}
    s3_data = [{"car_id": 1, "name": "Mazda MX-5"}]

    with patch("app.fetch_s3_data", return_value=s3_data) as mock_fetch:
        result = process_api_response(s3_payload)

    mock_fetch.assert_called_once_with("https://s3.example.com/data.json")
    assert result["type"] == "simple_s3"
    assert result["data"] == s3_data
    assert result["expires"] == "2026-01-01T00:00:00Z"


# ── Chunked response ──────────────────────────────────────────────────────

CHUNKED_PAYLOAD = {
    "data": {
        "chunk_info": {
            "base_download_url": "https://s3.example.com/chunks/",
            "chunk_file_names": ["chunk_0.json", "chunk_1.json"],
            "num_chunks": 2,
            "rows": 500,
            "chunk_size": 250,
        }
    }
}

def test_chunked_fetches_first_chunk():
    chunk_data = [{"subsession_id": 1}]

    with patch("app.fetch_s3_data", return_value=chunk_data) as mock_fetch:
        result = process_api_response(CHUNKED_PAYLOAD)

    mock_fetch.assert_called_once_with("https://s3.example.com/chunks/chunk_0.json")
    assert result["type"] == "chunked"
    assert result["data"] == chunk_data
    assert result["current_chunk"] == 0

def test_chunked_includes_base_url():
    with patch("app.fetch_s3_data", return_value=[]):
        result = process_api_response(CHUNKED_PAYLOAD)
    assert result["chunk_info"]["base_url"] == "https://s3.example.com/chunks/"

def test_chunked_includes_chunk_files():
    with patch("app.fetch_s3_data", return_value=[]):
        result = process_api_response(CHUNKED_PAYLOAD)
    assert result["chunk_info"]["chunk_files"] == ["chunk_0.json", "chunk_1.json"]
    assert result["chunk_info"]["total_chunks"] == 2
