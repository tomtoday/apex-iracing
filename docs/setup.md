# APEX - Setup Instructions

## Prerequisites

- [Go 1.23+](https://go.dev/dl/) — for running or building locally
- [Docker](https://www.docker.com) — for running via container (optional)
- `make` — for the convenience commands below
  - macOS/Linux: included by default
  - Windows: available via [Git Bash](https://gitforwindows.org), WSL, or `choco install make`

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/tomtoday/apex-iracing.git
cd apex-iracing
```

### 2. Run the application

**Locally:**

```bash
make run
```

**Via Docker:**

```bash
make build
make run-docker
```

### 3. Open in browser and authenticate

Go to `http://127.0.0.1:3000` and click **Login with iRacing**. After logging in:

1. You're redirected back to the app
2. Your token is saved to `data/.iracing_token` (gitignored — never committed)
3. The app fetches the full API documentation from iRacing
4. All 70+ endpoints are immediately available in the sidebar

Click **Logout** in the header to clear your token and return to the login screen.

## Configuration

The server accepts the following flags:

| Flag | Default | Description |
|---|---|---|
| `--addr` | `127.0.0.1:3000` | Listen address |
| `--token-file` | `.iracing_token` | OAuth token file path |
| `--data-dir` | `data` | Directory for cached API docs |

When running via Docker (`make run`), both the token file and docs cache are stored under `data/` and mounted as a volume, so they persist between container restarts.

## Makefile commands

```bash
make run         # Run locally with go run
make build       # Build the Docker image
make run-docker  # Run via Docker (mounts ./data for token + docs cache)
make test        # Run the Go test suite
```

## How It Works

### OAuth Flow

1. Click **Login** → browser opens `https://oauth.iracing.com/...`
2. Log in with your iRacing credentials
3. Redirected to `http://127.0.0.1:3000/callback`
4. Token saved to the configured token file; API docs fetched from iRacing

The app uses OAuth 2.1 with PKCE — no client secret is required.

### API Explorer

The sidebar is built dynamically from the iRacing API documentation. Select any endpoint, fill in the parameters, and click **Call Endpoint**. The backend:

- Adds your Bearer token to the request
- Calls `https://members-ng.iracing.com/data/...`
- Automatically follows S3 redirect links and fetches the actual data
- Handles chunked (paginated) responses with Prev/Next navigation
- Returns the result to the frontend as JSON or a table

### API Docs Caching

On startup the app attempts to fetch fresh docs from iRacing. If not yet authenticated, it falls back to a local cache (`data/data-docs.json`, gitignored). The cache is written on every successful fetch, so it stays current automatically.

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

### Troubleshooting Bruno

**"No S3 link found in environment"**
> Run an API endpoint first; the post-response script populates the variable.

**S3 link expired (404)**
> Links expire in ~2 minutes; re-run the original endpoint to get a fresh one.

**Authentication issues**
> Tokens refresh automatically; if stuck, clear Bruno's cache or re-run any request to trigger a fresh OAuth flow.

---

## Running Tests

```bash
make test
```

Or directly:

```bash
cd web && go test ./...
```

## Project Structure

```
web/
  cmd/apex/
    main.go           Server entry point — routing, handlers, docs cache
  internal/
    oauth/            OAuth 2.1 + PKCE client and token management
    proxy/            API proxy — S3 resolution, chunked response handling
    browser/          Cross-platform browser open helper
  assets/
    embed.go          Embedded static files and templates
    static/           app.js, style.css
    templates/        index.html
  Dockerfile
data/
  data-docs.json      API docs cache (auto-generated, gitignored)
  .iracing_token      OAuth token (auto-generated, gitignored)
tools/                Go developer tools (see docs/tooling.md)
```

## Troubleshooting

**`Port already in use`**
> Port 3000 is taken. Kill it: `lsof -ti:3000 | xargs kill -9`

**"Not authenticated" after login**
> Token may have expired — click Login again. Check that `data/.iracing_token` exists.

**Sidebar is empty after login**
> The API docs fetch failed. Check the terminal output. Restarting the app usually resolves it.

**Docker: token lost between runs**
> Make sure you're running with `-v "$PWD/data:/data"` so the data directory is mounted.
