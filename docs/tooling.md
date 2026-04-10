# APEX - Go Tooling

The `tools/` directory contains Go-based developer tools for maintaining the APEX project. Each tool is a standalone command in its own subdirectory and shares a single Go module (`tools/go.mod`). They have no dependency on the Python web app or each other beyond the shared `.iracing_token` file.

## Prerequisites

- Go 1.23 or higher (`go version`)
- An iRacing OAuth token — obtained by authenticating through any APEX tool (see [Authentication](#authentication) below)

## Tools

### sync-bruno

Keeps the Bruno collection in sync with the iRacing API documentation.

```
tools/sync-bruno/main.go
```

**Check for drift** (default mode — exit code 1 if anything is missing):

```bash
cd tools
go run ./sync-bruno
```

**Generate missing `.bru` files:**

```bash
go run ./sync-bruno --sync
```

**Regenerate all `.bru` files** to pick up new parameters, corrected syntax, or updated docs blocks:

```bash
go run ./sync-bruno --update
```

**Force a fresh fetch of API docs from iRacing** (bypasses the local cache):

```bash
go run ./sync-bruno --fetch
```

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--docs` | `../data/data-docs.json` | Path to the API docs cache |
| `--bruno` | `../bruno` | Path to the Bruno collection root |
| `--token` | `../.iracing_token` | Path to the iRacing OAuth token file |
| `--sync` | false | Generate `.bru` files for missing endpoints |
| `--update` | false | Regenerate all `.bru` files (implies `--sync`) |
| `--fetch` | false | Force a live fetch of API docs even if the cache exists |

#### go generate

The `tools/generate.go` file contains a `//go:generate` directive so you can run the check as part of a standard Go workflow:

```bash
cd tools
go generate ./...
```

This is equivalent to running `go run ./sync-bruno` in check mode.

## Authentication

The iRacing API requires a valid OAuth 2.1 Bearer token. All APEX tools share the same token file (`.iracing_token` in the repo root), written by whichever tool authenticated first.

**Doc loading resolves in order:**

1. **Cache hit** — if `data/data-docs.json` exists and `--fetch` is not set, no network call is made and no token is needed.
2. **Cache miss / `--fetch`** — the tool calls the iRacing API using the stored token, writes the result to `data/data-docs.json`, then proceeds. A missing or expired token is a hard error at this point.

To obtain a token, authenticate through any APEX tool — run the web app and click **Login**, or open the Bruno collection and send any request. The token is saved to `.iracing_token` and reused automatically. The Go tools will refresh it once on a 401, but cannot initiate a new OAuth flow on their own.

## Running Tests

```bash
cd tools
go test ./...
```

The test suite has two layers:

**Unit tests** — cover the generator functions (filename formatting, parameter sorting, file generation, note parsing). These run without a token or network access.

**Integration tests** — verify the Bruno collection against the live API docs: every endpoint has a file, no stale files exist, and all parameters are present. These require `data/data-docs.json` and skip gracefully when it is absent.

```bash
# Run only unit tests (no auth required)
go test ./sync-bruno/ -run "^Test(TitleCase|BruFilename|NoteStrings|MaxSeq|GenerateBRU)"

# Run everything including integration tests (requires data-docs.json)
go test ./sync-bruno/ -v
```

## Adding a New Tool

1. Create a subdirectory: `tools/my-tool/main.go`
2. Use `package main` — each tool is its own binary
3. Add a `//go:generate` line to `tools/generate.go` if it should run as part of `go generate ./...`
4. Add tests in `tools/my-tool/main_test.go`

All tools in `tools/` share the same `go.mod` (`module apex-iracing/tools`), so no extra module setup is needed.
