package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apex-iracing/web/internal/oauth"
	"apex-iracing/web/internal/proxy"
)

// fakeJWT builds a minimal unsigned JWT with an iracing_cust_id claim.
func fakeJWT(custID int) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload, _ := json.Marshal(map[string]any{"iracing_cust_id": custID})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"
}

// newAuthenticatedEnv sets up a mux with a pre-authenticated client and returns
// the test server, the oauth client, and a cleanup func.
func newAuthenticatedEnv(t *testing.T, docs map[string]any) (*httptest.Server, *oauth.Client) {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, ".iracing_token")

	tok := map[string]string{"access_token": fakeJWT(12345), "refresh_token": "refresh"}
	data, _ := json.MarshalIndent(tok, "", "  ")
	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	oa := oauth.NewClient(tokenPath)
	p := proxy.New(oa, "https://members-ng.iracing.com/data")
	dc := &docsCache{}
	dc.set(docs)

	mux := http.NewServeMux()
	registerRoutes(mux, oa, p, dc, filepath.Join(dir, "docs.json"))
	return httptest.NewServer(mux), oa
}

func newUnauthEnv(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	oa := oauth.NewClient(filepath.Join(dir, ".iracing_token"))
	p := proxy.New(oa, "https://members-ng.iracing.com/data")
	dc := &docsCache{}
	dc.set(nil)
	mux := http.NewServeMux()
	registerRoutes(mux, oa, p, dc, filepath.Join(dir, "docs.json"))
	return httptest.NewServer(mux)
}

func getJSON(t *testing.T, url string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return resp, m
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestIndex(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html, got %q", ct)
	}
}

func TestStaticFiles(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	for _, path := range []string{"/static/app.js", "/static/style.css"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestAuthStatus_Unauthenticated(t *testing.T) {
	srv := newUnauthEnv(t)
	defer srv.Close()

	_, body := getJSON(t, srv.URL+"/api/auth-status")
	if body["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", body["authenticated"])
	}
}

func TestAuthStatus_Authenticated(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	_, body := getJSON(t, srv.URL+"/api/auth-status")
	if body["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", body["authenticated"])
	}
}

func TestDocs(t *testing.T) {
	docs := map[string]any{"car": map[string]any{"get": map[string]any{"link": "https://example.com"}}}
	srv, _ := newAuthenticatedEnv(t, docs)
	defer srv.Close()

	resp, body := getJSON(t, srv.URL+"/api/docs")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if _, ok := body["car"]; !ok {
		t.Errorf("expected 'car' key in docs, got %v", body)
	}
}

func TestProxy_Unauthenticated(t *testing.T) {
	srv := newUnauthEnv(t)
	defer srv.Close()

	resp, _ := getJSON(t, srv.URL+"/api/proxy?_endpoint=car/get")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}

func TestProxy_WithMockBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"cars": []any{"car1", "car2"}})
	}))
	defer backend.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, ".iracing_token")
	tok := map[string]string{"access_token": fakeJWT(99), "refresh_token": "refresh"}
	data, _ := json.MarshalIndent(tok, "", "  ")
	_ = os.WriteFile(tokenPath, data, 0600)

	oa := oauth.NewClient(tokenPath)
	p := proxy.New(oa, backend.URL)
	dc := &docsCache{}
	dc.set(map[string]any{
		"car": map[string]any{
			"get": map[string]any{"link": backend.URL + "/data/car/get"},
		},
	})

	mux := http.NewServeMux()
	registerRoutes(mux, oa, p, dc, filepath.Join(dir, "docs.json"))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, body := getJSON(t, srv.URL+"/api/proxy?_endpoint=car/get")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if body["type"] != "direct" {
		t.Errorf("got type %q, want direct", body["type"])
	}
}

func TestSearchSeries_MissingFilter(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	resp, _ := getJSON(t, srv.URL+"/api/search-series")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestSearchChunk_OutOfRange(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	files, _ := json.Marshal([]string{"a.json"})
	resp, _ := getJSON(t, srv.URL+"/api/search-chunk/5?files="+string(files)+"&base_url=http://example.com/")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestSearchChunk_MissingParams(t *testing.T) {
	srv, _ := newAuthenticatedEnv(t, nil)
	defer srv.Close()

	resp, _ := getJSON(t, srv.URL+"/api/search-chunk/0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}
