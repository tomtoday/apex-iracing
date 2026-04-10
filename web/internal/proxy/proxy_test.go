package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"apex-iracing/web/internal/oauth"
)

func newTestProxy(t *testing.T, baseURL string) *Proxy {
	t.Helper()
	c := oauth.NewClient(filepath.Join(t.TempDir(), ".iracing_token"))
	return New(c, baseURL)
}

// ── ProcessResponse ───────────────────────────────────────────────────────────

func TestProcessResponse_Direct(t *testing.T) {
	p := newTestProxy(t, "")
	raw := []byte(`{"foo":"bar","count":3}`)

	resp, err := p.ProcessResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != "direct" {
		t.Errorf("got type %q, want direct", resp.Type)
	}
	m, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", resp.Data)
	}
	if m["foo"] != "bar" {
		t.Errorf("unexpected data: %v", m)
	}
}

func TestProcessResponse_SimpleS3(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"result": "s3-data"})
	}))
	defer s3.Close()

	p := newTestProxy(t, "")
	raw, _ := json.Marshal(map[string]any{
		"link":    s3.URL + "/data",
		"expires": "2099-01-01T00:00:00Z",
	})

	resp, err := p.ProcessResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != "simple_s3" {
		t.Errorf("got type %q, want simple_s3", resp.Type)
	}
	if resp.Expires != "2099-01-01T00:00:00Z" {
		t.Errorf("unexpected expires: %q", resp.Expires)
	}
	m, ok := resp.Data.(map[string]any)
	if !ok || m["result"] != "s3-data" {
		t.Errorf("unexpected S3 data: %v", resp.Data)
	}
}

func TestProcessResponse_Chunked(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1}, {"id": 2}})
	}))
	defer s3.Close()

	p := newTestProxy(t, "")
	raw, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"chunk_info": map[string]any{
				"base_download_url": s3.URL + "/",
				"chunk_file_names":  []string{"chunk_0.json", "chunk_1.json"},
				"num_chunks":        2,
				"rows":              200,
				"chunk_size":        100,
			},
		},
	})

	resp, err := p.ProcessResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != "chunked" {
		t.Errorf("got type %q, want chunked", resp.Type)
	}
	if resp.ChunkInfo.TotalChunks != 2 {
		t.Errorf("got TotalChunks %d, want 2", resp.ChunkInfo.TotalChunks)
	}
	if resp.CurrentChunk != 0 {
		t.Errorf("expected current chunk 0, got %d", resp.CurrentChunk)
	}
	if resp.Data == nil {
		t.Error("expected first chunk data to be populated")
	}
}

// ── FetchChunk ────────────────────────────────────────────────────────────────

func TestFetchChunk(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"chunk": r.URL.Path})
	}))
	defer s3.Close()

	p := newTestProxy(t, "")
	files := []string{"a.json", "b.json", "c.json"}

	resp, err := p.FetchChunk(s3.URL+"/", files, 1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.CurrentChunk != 1 {
		t.Errorf("got CurrentChunk %d, want 1", resp.CurrentChunk)
	}
}

func TestFetchChunk_OutOfRange(t *testing.T) {
	p := newTestProxy(t, "")
	_, err := p.FetchChunk("http://example.com/", []string{"a.json"}, 5)
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
}

// ── 401 retry ─────────────────────────────────────────────────────────────────

func TestMakeRequest_401Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	// Token server that accepts refresh
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed",
			"refresh_token": "new-refresh",
		})
	}))
	defer tokenSrv.Close()

	// Use a transport that redirects oauth.iracing.com → tokenSrv
	orig := http.DefaultTransport
	http.DefaultTransport = &redirectTransport{
		from:    "https://oauth.iracing.com",
		to:      tokenSrv.URL,
		wrapped: &http.Transport{},
	}
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, ".iracing_token")
	if err := writeToken(tokenPath, "old-access", "old-refresh"); err != nil {
		t.Fatal(err)
	}
	c2 := oauth.NewClient(tokenPath)

	p := New(c2, srv.URL)
	body, err := p.MakeRequest(srv.URL+"/data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true {
		t.Errorf("unexpected result: %v", result)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeToken(path, access, refresh string) error {
	data, _ := json.MarshalIndent(map[string]string{
		"access_token":  access,
		"refresh_token": refresh,
	}, "", "  ")
	return os.WriteFile(path, data, 0600)
}

type redirectTransport struct {
	from    string
	to      string
	wrapped http.RoundTripper
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	if req.URL.Scheme+"://"+req.URL.Host == rt.from {
		r2.URL.Scheme = "http"
		r2.URL.Host = rt.to[len("http://"):]
	}
	return rt.wrapped.RoundTrip(r2)
}
