# APEX - API Explorer for iRacing

In 2025, the old method for authenticating to the iRacing Data API using your username and password was phased out in favor of a more secure OAuth-based method. You can still use a [password limited flow](https://oauth.iracing.com/oauth2/book/password_limited_flow.html) but it requires pre-registering a client and can expose credentials if not handled carefully. This makes it hard to explore the iRacing Data API.

APEX provides secure, [authenticated access](https://oauth.iracing.com/oauth2/book/authorization_code_flow.html) to the iRacing Data API. It's a developer-friendly tool for exploring and testing iRacing data, with 70+ endpoints organized into 16 logical categories and intelligent S3 data handling.

APEX is available in two interfaces:
- **Web Interface** — browser-based explorer covering all 70+ endpoints with automatic S3 handling and auto updating from the data documentation made available by iRacing.
- **Bruno Collection** (`/bruno/`) — for direct API exploration with the [Bruno](https://www.usebruno.com) client, if you prefer

**Please note:** this tool was created "agent-first" on the documented endpoints at https://members-ng.iracing.com/data/doc

## Download

The quickest way to get started and explore the iRacing Data API is to download one of the pre-built binaries for macOS, Linux, and Windows. These are are available on the [Releases](https://github.com/tomtoday/apex-iracing/releases) page.

APEX is made available as a single self-contained executable — no dependencies, no install step. Download the archive for your platform, extract it, and run the binary. It opens your browser automatically on startup.

| Platform | File |
|---|---|
| macOS (Apple Silicon) | `apex_*_darwin_arm64.tar.gz` |
| macOS (Intel) | `apex_*_darwin_amd64.tar.gz` |
| Linux (amd64) | `apex_*_linux_amd64.tar.gz` |
| Linux (arm64) | `apex_*_linux_arm64.tar.gz` |
| Windows (amd64) | `apex_*_windows_amd64.zip` |
| Windows (arm64) | `apex_*_windows_arm64.zip` |

> **macOS note:** Gatekeeper may block the binary on first run. Right-click → Open to bypass the warning, or run `xattr -d com.apple.quarantine ./apex` in the terminal.

## Authentication

APEX uses **OAuth 2.1 Authorization Code Flow with PKCE** — the recommended approach for public/shared applications. A client ID has already been registered with iRacing.

**Public client model:**
- All users share the same `client_id`: `apex-api-explorer`
- No `client_secret` is needed (registered as a public client)
- Each user authenticates individually with their own iRacing account
- Tokens are stored locally and never shared
- Safe to publish on GitHub with no security concerns

**Registered details:**
- Client Type: Public client
- Redirect URI: `http://localhost:3000/callback`
- Scope: `iracing.auth`

## Project Structure

```
apex-iracing/
├── bruno/              # Bruno API collection (70+ endpoints, 16 categories)
├── data/               # Runtime data (gitignored) — API docs cache, token file
├── docs/               # Documentation
│   ├── setup.md        # Installation and usage instructions
│   ├── bruno.md        # Bruno collection guide
│   └── tooling.md      # Go developer tools
├── tools/              # Go-based developer tools
│   ├── go.mod
│   ├── generate.go     # go generate entry point
│   └── sync-bruno/     # Keeps Bruno collection in sync with API docs
├── web/                # Go web application
│   ├── go.mod
│   ├── cmd/apex/       # Main server entry point
│   ├── internal/       # OAuth, proxy, browser packages
│   ├── assets/         # Embedded static files and HTML template
│   └── Dockerfile
├── Makefile
└── LICENSE
```

## Documentation

- [Setup & Usage](docs/setup.md) — install, run, authenticate, web interface, tests
- [Bruno Collection](docs/bruno.md) — S3 handling, workflows, helper requests, environment variables
- [Go Tooling](docs/tooling.md) — sync-bruno, go generate, running Go tests
- [iRacing API Docs](https://members-ng.iracing.com/data/doc)

## License

See LICENSE file for details.
