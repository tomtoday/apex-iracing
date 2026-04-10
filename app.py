"""APEX Web Interface - Flask app for iRacing API exploration"""
import json
import jwt
import requests
from flask import Flask, render_template, redirect, request, jsonify
from urllib.parse import urlparse, parse_qs
from oauth_client import IracingOAuthClient

class AuthError(Exception):
    """Raised when authentication fails and cannot be refreshed"""
    pass

app = Flask(__name__)
oauth_client = IracingOAuthClient()

# iRacing API configuration
IRACING_API_BASE = "https://members-ng.iracing.com/data"


def get_api_headers():
    """Get headers for API requests including authorization"""
    token = oauth_client.get_access_token()
    if not token:
        return None
    return {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
    }


def make_api_request(url, params=None):
    """Make API request with automatic token refresh on 401"""
    headers = get_api_headers()
    if not headers:
        raise AuthError("Not authenticated")
    
    response = requests.get(url, headers=headers, params=params)

    # If we get 401, try to refresh token and retry once
    if response.status_code == 401:
        if oauth_client.refresh_access_token():
            # Retry with refreshed token
            headers = get_api_headers()
            response = requests.get(url, headers=headers, params=params)
        else:
            # Refresh failed, clear token and raise auth error
            oauth_client.clear_token()
            raise AuthError("Token expired and refresh failed")
    elif response.status_code == 400:
        raise ValueError(f"Bad request: {response.text}")
    else:
        response.raise_for_status()

    return response


def get_current_user_id():
    """Extract current user's cust_id from JWT token"""
    try:
        token = oauth_client.get_access_token()
        if not token:
            return None
        
        # Decode JWT without verification (we trust the server that issued it)
        decoded = jwt.decode(token, options={"verify_signature": False})
        return decoded.get('iracing_cust_id')
    except Exception as e:
        print(f"Failed to extract user ID from token: {str(e)}")
        return None


def fetch_s3_data(url):
    """Fetch data from an S3 URL"""
    try:
        response = requests.get(url, timeout=30)
        response.raise_for_status()
        return response.json()
    except Exception as e:
        return {"error": f"Failed to fetch S3 data: {str(e)}"}


def process_api_response(response_data):
    """
    Process API response and auto-fetch S3 data if needed.
    Handles three patterns:
    1. Simple S3 link: {link: "...", expires: "..."}
    2. Chunked data: {data: {chunk_info: {...}}}
    3. Direct data: raw JSON (no S3 fetch)
    """
    if not response_data:
        return {"error": "Empty response"}

    # Pattern 1: Simple S3 link
    if isinstance(response_data, dict) and "link" in response_data:
        s3_url = response_data.get("link")
        s3_data = fetch_s3_data(s3_url)
        return {
            "type": "simple_s3",
            "expires": response_data.get("expires"),
            "data": s3_data,
        }

    # Pattern 2: Chunked data
    if (
        isinstance(response_data, dict)
        and "data" in response_data
        and isinstance(response_data.get("data"), dict)
        and "chunk_info" in response_data["data"]
    ):
        chunk_info = response_data["data"]["chunk_info"]
        base_url = chunk_info.get("base_download_url", "")
        chunk_files = chunk_info.get("chunk_file_names", [])

        # Auto-fetch first chunk
        first_chunk_url = base_url + chunk_files[0] if chunk_files else None
        chunk_data = fetch_s3_data(first_chunk_url) if first_chunk_url else None

        return {
            "type": "chunked",
            "chunk_info": {
                "total_chunks": chunk_info.get("num_chunks"),
                "total_rows": chunk_info.get("rows"),
                "chunk_size": chunk_info.get("chunk_size"),
                "chunk_files": chunk_files,
                "base_url": base_url,
            },
            "current_chunk": 0,
            "data": chunk_data,
        }

    # Pattern 3: Direct data (or fallback)
    return {
        "type": "direct",
        "data": response_data,
    }


@app.route("/")
def index():
    """Main page"""
    return render_template("index.html")


@app.route("/api/auth-status")
def auth_status():
    """Check authentication status"""
    return jsonify({
        "authenticated": oauth_client.is_authenticated(),
        "client_id": oauth_client.client_id,
    })


@app.route("/login")
def login():
    """Initiate OAuth login"""
    auth_url = oauth_client.generate_auth_url()
    oauth_client.open_browser(auth_url)
    return redirect(auth_url)


@app.route("/callback")
def callback():
    """OAuth callback handler"""
    code = request.args.get("code")
    state = request.args.get("state")

    if not code or not state:
        return jsonify({"error": "Missing code or state"}), 400

    try:
        oauth_client.handle_callback(code, state)
        global API_DOCS
        API_DOCS = load_api_docs()
        return """
        <html>
        <head><title>Authentication Successful</title></head>
        <body>
            <h1>✅ Authentication Successful!</h1>
            <p>You can now close this window and return to the web interface.</p>
            <p><a href="/">Back to APEX</a></p>
        </body>
        </html>
        """
    except Exception as e:
        return jsonify({"error": str(e)}), 400


@app.route("/api/member-info")
def member_info():
    """Get member information"""
    if not oauth_client.is_authenticated():
        return jsonify({"error": "Not authenticated"}), 401

    try:
        member_id = request.args.get("member_id")
        
        # If no member_id provided, use current user's ID from token
        if not member_id:
            member_id = get_current_user_id()
            if not member_id:
                return jsonify({"error": "Could not determine member ID. Please provide member_id parameter or re-authenticate."}), 400
        
        url = f"{IRACING_API_BASE}/member/get"
        params = {"include_licenses": "true", "cust_ids": member_id}

        response = make_api_request(url, params=params)
        api_response = response.json()

        # Process response (auto-fetch S3 if needed)
        processed = process_api_response(api_response)
        return jsonify(processed)

    except AuthError as e:
        return jsonify({"error": str(e)}), 401
    except requests.exceptions.RequestException as e:
        return jsonify({"error": f"API request failed: {str(e)}"}), 500
    except Exception as e:
        return jsonify({"error": str(e)}), 500


@app.route("/api/cars")
def cars():
    """Get car list"""
    if not oauth_client.is_authenticated():
        return jsonify({"error": "Not authenticated"}), 401

    try:
        url = f"{IRACING_API_BASE}/car/get"
        response = make_api_request(url)
        api_response = response.json()

        # Process response (auto-fetch S3 if needed)
        processed = process_api_response(api_response)
        return jsonify(processed)

    except AuthError as e:
        return jsonify({"error": str(e)}), 401
    except requests.exceptions.RequestException as e:
        return jsonify({"error": f"API request failed: {str(e)}"}), 500
    except Exception as e:
        return jsonify({"error": str(e)}), 500


@app.route("/api/search-series")
def search_series():
    """Search for series (chunked data example)"""
    if not oauth_client.is_authenticated():
        return jsonify({"error": "Not authenticated"}), 401

    try:
        url = f"{IRACING_API_BASE}/results/search_series"

        season_year = request.args.get("season_year")
        season_quarter = request.args.get("season_quarter")
        start_range_begin = request.args.get("start_range_begin")
        finish_range_begin = request.args.get("finish_range_begin")

        # API requires at least one of: (season_year + season_quarter), start_range_begin, finish_range_begin
        if season_year and not season_quarter:
            return jsonify({"error": "season_quarter is required when using season_year"}), 400
        if season_quarter and not season_year:
            return jsonify({"error": "season_year is required when using season_quarter"}), 400
        if not season_year and not start_range_begin and not finish_range_begin:
            return jsonify({"error": "At least one filter is required: season_year + season_quarter, start_range_begin, or finish_range_begin"}), 400

        params = {}
        if season_year:
            params["season_year"] = season_year
        if season_quarter:
            params["season_quarter"] = season_quarter
        if start_range_begin:
            params["start_range_begin"] = start_range_begin
        if finish_range_begin:
            params["finish_range_begin"] = finish_range_begin
        for key in ("cust_id", "team_id", "series_id", "race_week_num"):
            if request.args.get(key):
                params[key] = request.args.get(key)

        response = make_api_request(url, params=params)
        api_response = response.json()

        # Process response (auto-fetch S3 if needed)
        processed = process_api_response(api_response)
        return jsonify(processed)

    except AuthError as e:
        return jsonify({"error": str(e)}), 401
    except ValueError as e:
        return jsonify({"error": str(e)}), 400
    except requests.exceptions.RequestException as e:
        return jsonify({"error": f"API request failed: {str(e)}"}), 500
    except Exception as e:
        import traceback
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500


@app.route("/api/search-chunk/<int:chunk_index>")
def search_chunk(chunk_index):
    """Fetch a specific chunk from a chunked response"""
    chunk_files = request.args.get("files")
    base_url = request.args.get("base_url")

    if not chunk_files or not base_url:
        return jsonify({"error": "Missing chunk metadata"}), 400

    try:
        files = json.loads(chunk_files)
        if chunk_index < 0 or chunk_index >= len(files):
            return jsonify({"error": "Chunk index out of range"}), 400

        chunk_url = base_url + files[chunk_index]
        chunk_data = fetch_s3_data(chunk_url)

        return jsonify({
            "type": "chunked_data",
            "current_chunk": chunk_index,
            "total_chunks": len(files),
            "data": chunk_data,
        })

    except Exception as e:
        return jsonify({"error": str(e)}), 500


IRACING_DOCS_URL = "https://members-ng.iracing.com/data/doc"
DOCS_CACHE_PATH = "data/data-docs.json"


def load_api_docs():
    """Fetch API docs from iRacing, falling back to local cache."""
    headers = get_api_headers()
    if headers:
        try:
            response = requests.get(IRACING_DOCS_URL, headers=headers, timeout=10)
            if response.status_code == 401:
                if oauth_client.refresh_access_token():
                    headers = get_api_headers()
                    response = requests.get(IRACING_DOCS_URL, headers=headers, timeout=10)
                else:
                    raise Exception("Token expired and refresh failed")
            response.raise_for_status()
            docs = response.json()
            with open(DOCS_CACHE_PATH, "w") as f:
                json.dump(docs, f)
            print("✅ API docs fetched from iRacing")
            return docs
        except Exception as e:
            print(f"⚠️  Could not fetch API docs ({e}), falling back to cache")
    else:
        print("⚠️  Not authenticated — loading API docs from cache")

    try:
        with open(DOCS_CACHE_PATH) as f:
            print("📄 API docs loaded from cache")
            return json.load(f)
    except FileNotFoundError:
        print("❌ No API docs cache found — authenticate and restart")
        return {}


API_DOCS = load_api_docs()


@app.route("/api/docs")
def api_docs():
    """Serve the iRacing API documentation"""
    return jsonify(API_DOCS)


@app.route("/api/proxy")
def proxy():
    """Generic proxy for any iRacing API endpoint"""
    if not oauth_client.is_authenticated():
        return jsonify({"error": "Not authenticated"}), 401

    endpoint = request.args.get("_endpoint")
    if not endpoint:
        return jsonify({"error": "Missing _endpoint parameter"}), 400

    parts = endpoint.split("/")
    if len(parts) != 2:
        return jsonify({"error": "Invalid endpoint format, expected category/name"}), 400

    category, name = parts
    if category not in API_DOCS or name not in API_DOCS[category]:
        return jsonify({"error": f"Unknown endpoint: {endpoint}"}), 404

    url = API_DOCS[category][name]["link"]
    params = {k: v for k, v in request.args.items() if k != "_endpoint"}

    try:
        response = make_api_request(url, params=params or None)
        processed = process_api_response(response.json())
        return jsonify(processed)
    except AuthError as e:
        return jsonify({"error": str(e)}), 401
    except ValueError as e:
        return jsonify({"error": str(e)}), 400
    except requests.exceptions.RequestException as e:
        return jsonify({"error": f"API request failed: {str(e)}"}), 500
    except Exception as e:
        import traceback
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500


@app.errorhandler(404)
def not_found(error):
    return jsonify({"error": "Endpoint not found"}), 404


if __name__ == "__main__":
    if not oauth_client.is_authenticated():
        print("=" * 80)
        print("🔐 APEX Web Interface")
        print("=" * 80)
        print("")
        print("Starting APEX on http://127.0.0.1:3000")
        print("")
        print("First run? Click 'Login' on the page to authenticate with iRacing.")
        print("You'll be taken through the OAuth flow - just log in with your")
        print("iRacing credentials. Your token will be saved locally.")
        print("")
        print("=" * 80)
    else:
        print("✅ Already authenticated - token loaded from .iracing_token")

    app.run(host="127.0.0.1", port=3000, debug=True)
