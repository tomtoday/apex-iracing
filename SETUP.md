# APEX Web Interface - Setup Instructions

## Prerequisites

- Python 3.8 or higher
- pip (Python package manager, usually included with Python)
- macOS (these instructions assume macOS, but Linux/Windows steps are similar)

## Quick Start

### 1. Clone or Navigate to the Repository

```bash
cd /Users/tom/development/apex-iracing
```

### 2. Create a Python Virtual Environment

A virtual environment keeps your project dependencies isolated from your system Python.

```bash
python3 -m venv .venv
```

This creates a `.venv/` folder (already in `.gitignore`).

### 3. Activate the Virtual Environment

```bash
source .venv/bin/activate
```

You should see `(.venv)` in your terminal prompt. When you're done working, deactivate with:

```bash
deactivate
```

### 4. Install Dependencies

```bash
pip install -r requirements.txt
```

This installs Flask and requests from the `requirements.txt` file.

### 5. Run the Application

```bash
python app.py
```

You should see output like:

```
================================================================================
🔐 APEX Web Interface
================================================================================

Starting APEX on http://127.0.0.1:3000

First run? Click 'Login' on the page to authenticate with iRacing.
You'll be taken through the OAuth flow - just log in with your
iRacing credentials. Your token will be saved locally.

================================================================================
```

### 6. Open in Browser

Open your browser and navigate to:

```
http://127.0.0.1:3000
```

### 7. Authenticate

Click the "Login with iRacing" button. Your browser will open iRacing's official login page. After you log in:

1. You'll be redirected back to the app
2. Your token is saved in `.iracing_token` (kept secret, not committed to git)
3. You can now use the endpoints

## Files Created

### Backend
- **`app.py`** - Flask server with OAuth and API proxy routes
- **`oauth_client.py`** - OAuth2 flow handler with PKCE and token management
- **`requirements.txt`** - Python dependencies
- **`.iracing_token`** (auto-created) - Your OAuth token (keep secret, gitignored)

### Frontend
- **`templates/index.html`** - Main HTML page
- **`static/app.js`** - Frontend JavaScript (fetch calls, UI logic)
- **`static/style.css`** - Page styling

## Available Endpoints

The web interface currently exposes three endpoints:

### 1. Member Info
- Get your iRacing member profile data
- Optionally specify a `member_id` for other users (if permissions allow)
- Response: Auto-fetches data from S3
- Example: Member name, rating, license class, division, etc.

### 2. Car List
- Get all cars available in iRacing
- No parameters
- Response: Auto-fetches full car list from S3
- Example: Car ID, car name, car class, etc.

### 3. Search Series
- Search for racing series (example of chunked data)
- No parameters (searches all series)
- Response: Auto-fetches first chunk, shows navigation
- Click "Next Chunk" to load more series

## How It Works

### OAuth Flow

1. Click "Login" on the home page
2. Browser opens iRacing's login page (`https://oauth.iracing.com/...`)
3. You log in with your iRacing credentials
4. Redirected back to `http://127.0.0.1:3000/callback`
5. Token is extracted and saved to `.iracing_token`
6. App loads all endpoints

### API Calls

1. You select an endpoint and click "Fetch"
2. Frontend calls `/api/member-info` (or other endpoint) on the backend
3. Backend:
   - Adds your access token to the request
   - Calls iRacing API (`https://members-ng.iracing.com/data/...`)
   - Detects response pattern (simple S3, chunked, direct)
   - **Auto-fetches S3 data** if needed (this is the key advantage over Bruno)
   - Returns combined result to frontend
4. Frontend displays the data (formatted JSON or table)

### Chunked Data Navigation

For endpoints that return chunked data (like Search Series):

1. Initial fetch returns chunk 1 of N
2. UI shows "Chunk 1 of 5" with "Next" button
3. Click "Next" → fetches chunk 2
4. Click "Previous" → goes back

## Troubleshooting

### "Command not found: python3"
If Python isn't installed, download from https://www.python.org

### "No such file or directory: .venv"
Forgot to create venv? Run: `python3 -m venv .venv`

### "ModuleNotFoundError: No module named 'flask'"
Forgot to install dependencies? Run: `pip install -r requirements.txt`

### "Address already in use"
Port 3000 is in use. Either:
- Kill the process: `lsof -ti:3000 | xargs kill -9`
- Modify `app.py` to use a different port (search for `port=3000`)

### "Not authenticated" after login
- Token may have expired → click Login again
- Check that `.iracing_token` file was created in the project root
- Check browser console for errors (F12 → Console)

### Search Series stuck on "Fetching..."
- Check browser console (F12 → Console) for error messages
- Try refreshing and logging in again
- S3 links expire after ~2 minutes, so timing out is possible

## Development Notes

### Virtual Environment Management

Always activate before working:
```bash
source .venv/bin/activate
```

Always deactivate when done:
```bash
deactivate
```

To delete the virtual environment (to start fresh):
```bash
rm -rf .venv
```

### Adding New Endpoints

1. Add endpoint function to `app.py`
2. Add button and form to `templates/index.html`
3. Add handler function to `static/app.js`
4. Call your backend endpoint in the handler

Example in `app.py`:
```python
@app.route("/api/new-endpoint")
def new_endpoint():
    """Description"""
    headers = get_api_headers()
    if not headers:
        return jsonify({"error": "Not authenticated"}), 401
    
    try:
        url = f"{IRACING_API_BASE}/path/to/endpoint"
        response = requests.get(url, headers=headers)
        response.raise_for_status()
        api_response = response.json()
        
        processed = process_api_response(api_response)
        return jsonify(processed)
    except Exception as e:
        return jsonify({"error": str(e)}), 500
```

### Security Notes

- **`.iracing_token` is secret** - Never commit it or share it
- **PKCE is enabled** - Prevents authorization code interception
- **All traffic over HTTPS** (for iRacing OAuth)
- **Localhost only** - Server runs on `127.0.0.1:3000` (not exposed to network)

## Next Steps

Once the web interface is working:

1. Test each endpoint and verify data is correct
2. Try navigating chunks in Search Series
3. Consider adding more endpoints (Track List, Results, etc.)
4. Add data validation and error handling as needed

## Documentation

- iRacing API Docs: https://members-ng.iracing.com/data/doc
- Flask Docs: https://flask.palletsprojects.com
- OAuth 2.1 Spec: https://oauth.net/
