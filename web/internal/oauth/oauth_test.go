package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── PKCE ──────────────────────────────────────────────────────────────────────

func TestGenerateVerifier(t *testing.T) {
	v1, err := generateVerifier()
	if err != nil {
		t.Fatal(err)
	}
	v2, err := generateVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Error("verifiers should be unique")
	}
	if len(v1) < 32 {
		t.Errorf("verifier too short: %q", v1)
	}
}

func TestGenerateChallenge(t *testing.T) {
	verifier := "test-verifier-string"
	challenge := generateChallenge(verifier)

	// Manually compute expected value
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expected {
		t.Errorf("got %q, want %q", challenge, expected)
	}
}

// ── Token file round-trip ─────────────────────────────────────────────────────

func TestTokenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".iracing_token")

	c := NewClient(path)
	if c.IsAuthenticated() {
		t.Fatal("should not be authenticated before token is saved")
	}

	tok := &Token{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		TokenType:    "Bearer",
	}
	if err := c.saveToken(tok); err != nil {
		t.Fatal(err)
	}

	c2 := NewClient(path)
	if !c2.IsAuthenticated() {
		t.Fatal("should be authenticated after token file written")
	}
	if got := c2.GetAccessToken(); got != "access-abc" {
		t.Errorf("got access token %q, want %q", got, "access-abc")
	}
}

func TestClearToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".iracing_token")

	c := NewClient(path)
	if err := c.saveToken(&Token{AccessToken: "tok"}); err != nil {
		t.Fatal(err)
	}
	c.token = c.loadToken()

	if err := c.ClearToken(); err != nil {
		t.Fatal(err)
	}
	if c.IsAuthenticated() {
		t.Error("should not be authenticated after ClearToken")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("token file should be deleted")
	}
}

// ── JWT payload extraction ────────────────────────────────────────────────────

func TestCustID(t *testing.T) {
	// Build a minimal JWT: header.payload.signature (signature not verified)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]any{"iracing_cust_id": float64(12345)}
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	fakeJWT := header + "." + payloadB64 + ".fakesig"

	dir := t.TempDir()
	c := NewClient(filepath.Join(dir, ".iracing_token"))
	c.token = &Token{AccessToken: fakeJWT}

	id, err := c.CustID()
	if err != nil {
		t.Fatal(err)
	}
	if id != 12345 {
		t.Errorf("got cust_id %d, want 12345", id)
	}
}

// ── State mismatch ────────────────────────────────────────────────────────────

func TestHandleCallback_StateMismatch(t *testing.T) {
	dir := t.TempDir()
	c := NewClient(filepath.Join(dir, ".iracing_token"))
	c.state = "correct-state"
	c.codeVerifier = "verifier"

	err := c.HandleCallback("some-code", "wrong-state")
	if err == nil {
		t.Fatal("expected error on state mismatch")
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── HandleCallback happy path ─────────────────────────────────────────────────

func TestHandleCallback_Success(t *testing.T) {
	// Spin up a fake token server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Token{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := NewClient(filepath.Join(dir, ".iracing_token"))
	c.state = "my-state"
	c.codeVerifier = "my-verifier"

	// Patch the token URL to point at our test server by temporarily replacing
	// the form post target via a round-trip through url.Values.
	origPost := http.DefaultClient
	_ = origPost // unused; we use a custom approach below

	http.DefaultTransport = &redirectTransport{
		from:    "https://oauth.iracing.com",
		to:      srv.URL,
		wrapped: &http.Transport{},
	}
	defer func() { http.DefaultTransport = &http.Transport{} }()

	if err := c.HandleCallback("auth-code", "my-state"); err != nil {
		t.Fatal(err)
	}
	if c.GetAccessToken() != "new-access" {
		t.Errorf("got %q, want new-access", c.GetAccessToken())
	}
}

type redirectTransport struct {
	from    string
	to      string
	wrapped http.RoundTripper
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Host = strings.TrimPrefix(rt.to, "http://")
	req2.URL.Scheme = "http"
	return rt.wrapped.RoundTrip(req2)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
