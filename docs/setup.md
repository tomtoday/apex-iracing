# APEX - Setup Instructions

## Prerequisites

- Python 3.8 or higher
- pip (included with Python)

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/tomtoday/apex-iracing.git ## or the SSH variant if you prefer

cd apex-iracing
```

### 2. Create and activate a virtual environment

```bash
python3 -m venv .venv
source .venv/bin/activate
```

You'll see `(.venv)` in your terminal prompt. Run `deactivate` when done.

### 3. Install dependencies

```bash
pip install -r requirements.txt
```

### 4. Run the application

```bash
python app.py
```

### 5. Open in browser and authenticate

Go to `http://127.0.0.1:3000` and click **Login with iRacing**. After logging in:

1. You're redirected back to the app
2. Your token is saved to `.iracing_token` (gitignored — never committed)
3. The app fetches the full API documentation from iRacing
4. All 70+ endpoints are immediately available in the sidebar

## How It Works

### OAuth Flow

1. Click **Login** → browser opens `https://oauth.iracing.com/...`
2. Log in with your iRacing credentials
3. Redirected to `http://127.0.0.1:3000/callback`
4. Token saved to `.iracing_token`; API docs fetched from `https://members-ng.iracing.com/data/doc`

The app uses OAuth 2.1 with PKCE — no client secret is required.

### API Explorer

The sidebar is built dynamically from the iRacing API documentation. Select any endpoint, fill in the parameters, and click **Call Endpoint**. The backend:

- Adds your Bearer token to the request
- Calls `https://members-ng.iracing.com/data/...`
- Automatically follows S3 redirect links and fetches the actual data
- Handles chunked (paginated) responses with Prev/Next navigation
- Returns the result to the frontend as JSON or a table

### API Docs caching

On startup the app attempts to fetch fresh docs from iRacing. If not yet authenticated, it falls back to a local cache (`static/data-docs.json`, gitignored). The cache is written on every successful fetch, so it stays current automatically.

## Using the Bruno Collection

The `bruno/` directory contains a complete Bruno collection covering all 70+ endpoints — useful for low-level API exploration or scripted testing.

### Setup

1. Install [Bruno](https://www.usebruno.com) (free, open source)
2. In Bruno, choose **Open Collection** and select the `bruno/` folder

### Making requests

1. Pick any endpoint from the 16 category folders in the sidebar
2. Click **Send** — Bruno handles OAuth automatically on first run (opens your browser)
3. The response will contain either an S3 link or chunked data info
4. Run **Fetch-S3-Data** to download the actual data

### S3 data handling

Most iRacing endpoints don't return data directly — they return a signed S3 link. Bruno's post-response scripts handle this automatically:

| Response type | Pattern | How to get data |
|---|---|---|
| Simple S3 | `{link: "...", expires: "..."}` | Run **Fetch-S3-Data** |
| Chunked | `{data: {chunk_info: {...}}}` | Run **Fetch-S3-Data**, adjust `s3_chunk_index` for subsequent chunks |
| Direct | Raw JSON | Already in the response body |

### Helper requests

- **Fetch-S3-Data** — fetches the current S3 link or chunk; works for both simple and chunked responses
- **Chunk-Navigator** — shows all available chunks and your current position

### Environment variables (auto-managed)

Bruno's post-response scripts keep these up to date after each request:

| Variable | Description |
|---|---|
| `s3_link` | Current S3 URL or first chunk URL |
| `s3_type` | `simple`, `chunked`, or `direct` |
| `s3_expires` | Link expiration time |
| `s3_chunk_count` | Total number of chunks |
| `s3_chunk_index` | Current chunk (0-based) |
| `s3_base_url` | Base URL for chunk files |
| `s3_chunk_files` | Array of chunk filenames |

To fetch a specific chunk: update `s3_chunk_index` in Bruno's Environment panel, then run **Fetch-S3-Data** again.

### Troubleshooting

**"No S3 link found in environment"** — run an API endpoint first; the post-response script populates the variable.

**S3 link expired (404)** — links expire in ~2 minutes; re-run the original endpoint to get a fresh one.

**Authentication issues** — tokens refresh automatically; if stuck, clear Bruno's cache or re-run any request to trigger a fresh OAuth flow.

---

## Running Tests

Install dev dependencies:

```bash
pip install -r requirements-dev.txt
```

Run the test suite:

```bash
python -m pytest tests/ -v
```

## Project Structure

```
app.py              Flask server — OAuth routes, generic API proxy
oauth_client.py     OAuth 2.1 + PKCE flow, token management
requirements.txt    Runtime dependencies
requirements-dev.txt  Dev/test dependencies
templates/
  index.html        Two-panel app shell
static/
  app.js            Dynamic endpoint explorer (sidebar, forms, response display)
  style.css         Layout and styling
  data-docs.json    API docs cache (auto-generated, gitignored)
tests/
  test_oauth.py     PKCE, auth URL, token file operations
  test_response.py  API response processing (S3, chunked, direct)
  test_routes.py    Flask route behaviour and auth guards
```

## Troubleshooting

### **`ModuleNotFoundError: No module named 'flask'`**

Forgot to install dependencies or activate the venv: `pip install -r requirements.txt`

### **`Address already in use`**

Port 3000 is taken. Kill it: `lsof -ti:3000 | xargs kill -9`

### **"Not authenticated" after login**

Token may have expired — click Login again. Check that `.iracing_token` exists in the project root.

### **Sidebar is empty after login**

The API docs fetch failed. Check the terminal for a `⚠️` message. Restarting the app usually resolves it.
