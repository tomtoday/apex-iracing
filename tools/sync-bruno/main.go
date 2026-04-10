// sync-bruno keeps the Bruno collection in sync with the iRacing API documentation.
//
// It compares the API docs against the Bruno collection and reports missing or
// stale request files. With --sync it generates .bru files for any missing
// endpoints; --update regenerates all existing files to pick up new parameters
// or corrected syntax.
//
// # Authentication
//
// The iRacing API requires a valid OAuth 2.1 Bearer token. sync-bruno reads
// this token from --token (default: ../data/.iracing_token), the same file written
// by any APEX authentication flow (web app, Bruno collection). Whichever tool
// you authenticated with first is sufficient — they all share the token file.
//
// Doc loading resolves in order:
//
//  1. Cache hit — if --docs exists and --fetch is not set, the cache is used
//     directly. No network call is made and no token is needed.
//
//  2. Cache miss / --fetch — the tool calls the iRacing API using the stored
//     token, writes the result to --docs, then proceeds. A missing or expired
//     token is a hard error at this point.
//
// To obtain a token, authenticate through any APEX tool (e.g. run the web app
// and click Login, or use the Bruno collection). The token is persisted to
// .iracing_token and reused automatically. sync-bruno will refresh it once on
// a 401, but cannot initiate a new OAuth flow on its own.
//
// Usage:
//
//	go run ./sync-bruno [--sync] [--update] [--fetch] [--docs PATH] [--bruno PATH] [--token PATH]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// categories maps each iRacing API namespace to the Bruno folder and name prefix.
// Add new entries here when iRacing introduces a new top-level namespace.
var categories = map[string]struct{ folder, prefix string }{
	"car":                      {"03-Cars", "Car"},
	"carclass":                 {"03-Cars", "CarClass"},
	"constants":                {"10-Constants", "Constants"},
	"driver_stats_by_category": {"12-DriverStats", "DriverStats"},
	"hosted":                   {"13-Hosted", "Hosted"},
	"league":                   {"08-Leagues", "League"},
	"lookup":                   {"09-Lookup", "Lookup"},
	"member":                   {"02-Member", "Member"},
	"results":                  {"06-Results", "Results"},
	"season":                   {"14-Season", "Season"},
	"series":                   {"07-Series", "Series"},
	"session":                  {"15-Session", "Session"},
	"stats":                    {"05-Stats", "Stats"},
	"team":                     {"16-Team", "Team"},
	"time_attack":              {"17-TimeAttack", "TimeAttack"},
	"track":                    {"04-Tracks", "Track"},
}

// endpointDoc mirrors the structure of each endpoint entry in data-docs.json.
type endpointDoc struct {
	Link              string                    `json:"link"`
	ExpirationSeconds int                       `json:"expirationSeconds"`
	Note              json.RawMessage           `json:"note"` // string or []string
	Parameters        map[string]parameterDoc   `json:"parameters"`
}

type parameterDoc struct {
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Note     string `json:"note"`
}

// urlRe extracts the path portion of a Bruno request URL.
var urlRe = regexp.MustCompile(`url:\s+\{\{base_url\}\}(/data/[^\s]+)`)

// seqRe extracts the seq value from a .bru file.
var seqRe = regexp.MustCompile(`seq:\s+(\d+)`)

func main() {
	docsPath := flag.String("docs", "../data/data-docs.json", "path to data-docs.json cache")
	brunoPath := flag.String("bruno", "../bruno", "path to Bruno collection root")
	tokenPath := flag.String("token", "../data/.iracing_token", "path to iRacing OAuth token file")
	sync := flag.Bool("sync", false, "generate missing .bru files")
	update := flag.Bool("update", false, "regenerate all .bru files to match current API docs (implies --sync)")
	fetch := flag.Bool("fetch", false, "force a fresh fetch from the iRacing API even if the cache exists")
	flag.Parse()

	if *update {
		*sync = true
	}

	docs, err := loadDocs(*docsPath, *tokenPath, *fetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Scan Bruno files and build a set of known URL paths.
	brunoURLs, err := scanBrunoURLs(*brunoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot scan Bruno directory %q: %v\n", *brunoPath, err)
		os.Exit(1)
	}

	// Compare docs against Bruno files.
	type missing struct {
		namespace, endpoint string
		doc                 endpointDoc
	}

	var missingList []missing
	var staleList []string
	var unknownNS []string
	matched := 0

	for ns, endpoints := range docs {
		if _, ok := categories[ns]; !ok {
			unknownNS = append(unknownNS, ns)
			continue
		}
		for ep, doc := range endpoints {
			path := fmt.Sprintf("/data/%s/%s", ns, ep)
			if brunoURLs[path] {
				matched++
			} else {
				missingList = append(missingList, missing{ns, ep, doc})
			}
		}
	}

	// Stale: Bruno files whose URL is not in docs.
	docURLs := buildDocURLs(docs)
	for url := range brunoURLs {
		if !docURLs[url] {
			staleList = append(staleList, url)
		}
	}

	sort.Slice(missingList, func(i, j int) bool {
		return missingList[i].namespace+missingList[i].endpoint <
			missingList[j].namespace+missingList[j].endpoint
	})
	sort.Strings(staleList)
	sort.Strings(unknownNS)

	total := matched + len(missingList)
	fmt.Printf("iRacing API endpoints:  %d\n", total)
	fmt.Printf("Bruno files matched:    %d\n", matched)
	fmt.Printf("Missing Bruno files:    %d\n", len(missingList))
	fmt.Printf("Stale Bruno files:      %d\n", len(staleList))

	if len(unknownNS) > 0 {
		fmt.Printf("\nWARNING: unknown namespaces (add to categories map in sync-bruno):\n")
		for _, ns := range unknownNS {
			fmt.Printf("  %s\n", ns)
		}
	}

	if len(staleList) > 0 {
		fmt.Printf("\nStale (in Bruno but not in docs — review manually):\n")
		for _, url := range staleList {
			fmt.Printf("  %s\n", url)
		}
	}

	if len(missingList) == 0 && !*update {
		fmt.Printf("\nBruno collection is in sync with the API docs.\n")
		return
	}

	if len(missingList) > 0 {
		fmt.Printf("\nMissing")
		if *sync {
			fmt.Printf(" (generating):\n")
		} else {
			fmt.Printf(" (run --sync to generate):\n")
		}
		for _, m := range missingList {
			cat := categories[m.namespace]
			filename := bruFilename(cat.prefix, m.endpoint)
			fmt.Printf("  [%s] %-50s  ← /data/%s/%s\n", cat.folder, filename, m.namespace, m.endpoint)
		}
	}

	if !*sync {
		os.Exit(1)
	}

	generated := 0

	// Generate missing files.
	for _, m := range missingList {
		if err := generateBRU(*brunoPath, m.namespace, m.endpoint, m.doc); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s/%s: %v\n", m.namespace, m.endpoint, err)
			os.Exit(1)
		}
		generated++
	}

	// With --update, regenerate all existing files so they match the current
	// API docs (fixes outdated syntax, missing params, adds docs blocks).
	if *update {
		fmt.Printf("\nUpdating all existing .bru files...\n")
		for ns, endpoints := range docs {
			if _, ok := categories[ns]; !ok {
				continue
			}
			for ep, doc := range endpoints {
				path := fmt.Sprintf("/data/%s/%s", ns, ep)
				if brunoURLs[path] {
					if err := generateBRU(*brunoPath, ns, ep, doc); err != nil {
						fmt.Fprintf(os.Stderr, "error updating %s/%s: %v\n", ns, ep, err)
						os.Exit(1)
					}
					generated++
				}
			}
		}
	}

	fmt.Printf("\n%d file(s) written.\n", generated)
}

// iRacing API endpoints and OAuth client ID used for docs fetch and token
// refresh. oauthClient must match the client_id used during the original
// authentication flow (see oauth_client.py: CLIENT_ID).
const (
	docsURL     = "https://members-ng.iracing.com/data/doc"
	tokenURL    = "https://oauth.iracing.com/oauth2/token"
	oauthClient = "apex-api-explorer"
)

// tokenData mirrors the JSON structure of .iracing_token.
type tokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// loadDocs returns the parsed API docs. When the cache file (docsPath) exists
// and forceFetch is false, it is returned immediately with no network call and
// no token required. Otherwise the docs are fetched from the iRacing API using
// the token at tokenPath, and the result is written to docsPath for future use.
func loadDocs(docsPath, tokenPath string, forceFetch bool) (map[string]map[string]endpointDoc, error) {
	if !forceFetch {
		if raw, err := os.ReadFile(docsPath); err == nil {
			var docs map[string]map[string]endpointDoc
			if err := json.Unmarshal(raw, &docs); err != nil {
				return nil, fmt.Errorf("invalid JSON in %q: %w", docsPath, err)
			}
			return docs, nil
		}
	}

	// Cache miss or --fetch: a valid token is required from here on.
	fmt.Println("Fetching API docs from iRacing...")
	raw, err := fetchDocs(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch API docs: %w\nhint: authenticate with iRacing first (run the web app or Bruno collection)", err)
	}

	// Write cache so the next run is instant.
	if err := os.MkdirAll(filepath.Dir(docsPath), 0o755); err == nil {
		_ = os.WriteFile(docsPath, raw, 0o644)
	}

	var docs map[string]map[string]endpointDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, fmt.Errorf("invalid JSON from API: %w", err)
	}
	fmt.Printf("Fetched and cached to %s\n", docsPath)
	return docs, nil
}

// fetchDocs fetches the iRacing API documentation from /data/doc using the
// Bearer token stored at tokenPath. If the initial request returns 401, it
// attempts a single token refresh and retries. A second 401 means the refresh
// token is also expired and the user must re-authenticate through any APEX
// tool to obtain a new token.
func fetchDocs(tokenPath string) ([]byte, error) {
	tok, err := readToken(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read token from %q: %w", tokenPath, err)
	}

	body, status, err := apiGET(docsURL, tok.AccessToken)
	if err != nil {
		return nil, err
	}

	if status == http.StatusUnauthorized {
		// Token expired — try to refresh once.
		tok, err = refreshToken(tok, tokenPath)
		if err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		body, status, err = apiGET(docsURL, tok.AccessToken)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized {
			return nil, fmt.Errorf("still unauthorized after token refresh — please re-authenticate")
		}
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("iRacing API returned HTTP %d", status)
	}
	return body, nil
}

// apiGET performs a GET request with a Bearer token and returns the body and status code.
func apiGET(endpoint, accessToken string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// refreshToken exchanges the refresh_token for a new access_token and saves it.
func refreshToken(tok tokenData, tokenPath string) (tokenData, error) {
	resp, err := http.PostForm(tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {oauthClient},
		"refresh_token": {tok.RefreshToken},
	})
	if err != nil {
		return tokenData{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenData{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return tokenData{}, fmt.Errorf("token refresh HTTP %d: %s", resp.StatusCode, body)
	}

	var newTok tokenData
	if err := json.Unmarshal(body, &newTok); err != nil {
		return tokenData{}, fmt.Errorf("invalid token response: %w", err)
	}

	// Persist the refreshed token so the next invocation uses it.
	if raw, err := json.MarshalIndent(newTok, "", "  "); err == nil {
		_ = os.WriteFile(tokenPath, raw, 0o600)
	}
	return newTok, nil
}

// readToken reads the token file shared by all APEX tools. The file is written
// by oauth_client.py (web app) or the Bruno collection's OAuth flow and holds
// the access_token and refresh_token from the iRacing OAuth 2.1 server.
func readToken(tokenPath string) (tokenData, error) {
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		return tokenData{}, err
	}
	var tok tokenData
	if err := json.Unmarshal(raw, &tok); err != nil {
		return tokenData{}, fmt.Errorf("invalid token JSON: %w", err)
	}
	if tok.AccessToken == "" {
		return tokenData{}, fmt.Errorf("token file has no access_token")
	}
	return tok, nil
}

// scanBrunoURLs walks the Bruno directory and returns a set of URL paths found
// in .bru request files, skipping known helper/infrastructure files.
func scanBrunoURLs(root string) (map[string]bool, error) {
	// Files that are intentional parts of the collection but not API data endpoints.
	skipFiles := map[string]bool{
		"collection.bru":      true,
		"Fetch-S3-Data.bru":   true,
		"Chunk-Navigator.bru": true,
	}
	// URL paths that are intentionally in the Bruno collection but absent from
	// data-docs.json (e.g. /data/doc is the endpoint that *produces* data-docs.json).
	skipURLs := map[string]bool{
		"/data/doc": true,
	}
	urls := map[string]bool{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".bru" || skipFiles[filepath.Base(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := urlRe.FindSubmatch(data); m != nil {
			urlPath := string(m[1])
			if !skipURLs[urlPath] {
				urls[urlPath] = true
			}
		}
		return nil
	})
	return urls, err
}

func buildDocURLs(docs map[string]map[string]endpointDoc) map[string]bool {
	urls := map[string]bool{}
	for ns, endpoints := range docs {
		for ep := range endpoints {
			urls[fmt.Sprintf("/data/%s/%s", ns, ep)] = true
		}
	}
	return urls
}

// titleCase converts snake_case to Title_Case, preserving underscores.
// e.g. "world_records" → "World_Records"
func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "_")
}

// bruFilename returns the canonical .bru filename for an endpoint.
func bruFilename(prefix, endpoint string) string {
	return fmt.Sprintf("Data-%s-%s.bru", prefix, titleCase(endpoint))
}

// maxSeqInFolder scans a folder for .bru files and returns the highest seq value found.
func maxSeqInFolder(folder string) int {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return 0
	}
	max := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".bru" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, e.Name()))
		if err != nil {
			continue
		}
		if m := seqRe.FindSubmatch(data); m != nil {
			if n, err := strconv.Atoi(string(m[1])); err == nil && n > max {
				max = n
			}
		}
	}
	return max
}

// noteStrings extracts endpoint notes as a slice of strings,
// handling both `"note": "string"` and `"note": ["a", "b"]`.
func noteStrings(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var ss []string
	if json.Unmarshal(raw, &ss) == nil {
		return ss
	}
	return nil
}

// generateBRU creates a .bru file for the given endpoint.
func generateBRU(brunoRoot, namespace, endpoint string, doc endpointDoc) error {
	cat := categories[namespace]
	folder := filepath.Join(brunoRoot, cat.folder)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}

	filename := bruFilename(cat.prefix, endpoint)
	dest := filepath.Join(folder, filename)
	seq := maxSeqInFolder(folder) + 1
	metaName := fmt.Sprintf("Data-%s-%s", cat.prefix, titleCase(endpoint))
	url := fmt.Sprintf("{{base_url}}/data/%s/%s", namespace, endpoint)

	var sb strings.Builder

	fmt.Fprintf(&sb, "meta {\n")
	fmt.Fprintf(&sb, "  name: %s\n", metaName)
	fmt.Fprintf(&sb, "  type: http\n")
	fmt.Fprintf(&sb, "  seq: %d\n", seq)
	fmt.Fprintf(&sb, "}\n")
	fmt.Fprintf(&sb, "\n")
	fmt.Fprintf(&sb, "get {\n")
	fmt.Fprintf(&sb, "  url: %s\n", url)
	fmt.Fprintf(&sb, "  body: none\n")
	fmt.Fprintf(&sb, "  auth: inherit\n")
	fmt.Fprintf(&sb, "}\n")

	if len(doc.Parameters) > 0 {
		// Sort params: required first, then alphabetical.
		type param struct {
			name string
			parameterDoc
		}
		params := make([]param, 0, len(doc.Parameters))
		for k, v := range doc.Parameters {
			params = append(params, param{k, v})
		}
		sort.Slice(params, func(i, j int) bool {
			if params[i].Required != params[j].Required {
				return params[i].Required
			}
			return params[i].name < params[j].name
		})

		fmt.Fprintf(&sb, "\nparams:query {\n")
		for _, p := range params {
			fmt.Fprintf(&sb, "  %s: \n", p.name)
		}
		fmt.Fprintf(&sb, "}\n")

		// Build docs block with parameter table.
		notes := noteStrings(doc.Note)
		fmt.Fprintf(&sb, "\ndocs {\n")
		fmt.Fprintf(&sb, "  # %s\n", metaName)
		if len(notes) > 0 {
			fmt.Fprintf(&sb, "\n")
			for _, n := range notes {
				fmt.Fprintf(&sb, "  %s\n", n)
			}
		}
		fmt.Fprintf(&sb, "\n  ## Parameters\n\n")
		fmt.Fprintf(&sb, "  | Parameter | Type | Required | Notes |\n")
		fmt.Fprintf(&sb, "  |-----------|------|----------|-------|\n")
		for _, p := range params {
			req := "no"
			if p.Required {
				req = "yes"
			}
			note := strings.ReplaceAll(p.Note, "|", "\\|")
			fmt.Fprintf(&sb, "  | %s | %s | %s | %s |\n", p.name, p.Type, req, note)
		}
		fmt.Fprintf(&sb, "}\n")
	} else {
		// No parameters — still emit a docs block if there's an endpoint note.
		notes := noteStrings(doc.Note)
		if len(notes) > 0 {
			fmt.Fprintf(&sb, "\ndocs {\n")
			fmt.Fprintf(&sb, "  # %s\n\n", metaName)
			for _, n := range notes {
				fmt.Fprintf(&sb, "  %s\n", n)
			}
			fmt.Fprintf(&sb, "}\n")
		}
	}

	return os.WriteFile(dest, []byte(sb.String()), 0o644)
}
