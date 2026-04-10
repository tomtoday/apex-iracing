package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	clientID    = "apex-api-explorer"
	callbackURL = "http://127.0.0.1:3000/callback"
	authURL     = "https://oauth.iracing.com/oauth2/authorize"
	tokenURL    = "https://oauth.iracing.com/oauth2/token"
)

// Token holds the OAuth token data as stored in the token file.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

// Client manages the OAuth 2.1 + PKCE flow for iRacing.
type Client struct {
	mu            sync.Mutex
	tokenFile     string
	token         *Token
	state         string
	codeVerifier  string
}

// NewClient creates a Client and loads any existing token from tokenFile.
func NewClient(tokenFile string) *Client {
	c := &Client{tokenFile: tokenFile}
	c.token = c.loadToken()
	return c
}

func (c *Client) loadToken() *Token {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return nil
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}
	return &t
}

func (c *Client) saveToken(t *Token) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenFile, data, 0600)
}

// IsAuthenticated reports whether a token with an access_token is loaded.
func (c *Client) IsAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token != nil && c.token.AccessToken != ""
}

// GetAccessToken returns the current access token, or empty string if none.
func (c *Client) GetAccessToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == nil {
		return ""
	}
	return c.token.AccessToken
}

// ClearToken removes the token file and clears the in-memory token.
func (c *Client) ClearToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = nil
	if err := os.Remove(c.tokenFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GenerateAuthURL builds the OAuth 2.1 authorization URL with PKCE.
func (c *Client) GenerateAuthURL() (string, error) {
	verifier, err := generateVerifier()
	if err != nil {
		return "", err
	}
	challenge := generateChallenge(verifier)

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	c.mu.Lock()
	c.codeVerifier = verifier
	c.state = state
	c.mu.Unlock()

	params := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {callbackURL},
		"scope":                 {"iracing.auth"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return authURL + "?" + params.Encode(), nil
}

// HandleCallback exchanges the authorization code for a token.
func (c *Client) HandleCallback(code, state string) error {
	c.mu.Lock()
	expectedState := c.state
	verifier := c.codeVerifier
	c.mu.Unlock()

	if state != expectedState {
		return fmt.Errorf("state mismatch: possible CSRF attack")
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {callbackURL},
		"code_verifier": {verifier},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, body)
	}

	var t Token
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return err
	}

	c.mu.Lock()
	c.token = &t
	c.mu.Unlock()

	return c.saveToken(&t)
}

// RefreshAccessToken uses the refresh token to obtain a new access token.
func (c *Client) RefreshAccessToken() error {
	c.mu.Lock()
	if c.token == nil || c.token.RefreshToken == "" {
		c.mu.Unlock()
		return fmt.Errorf("no refresh token available")
	}
	refreshToken := c.token.RefreshToken
	c.mu.Unlock()

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, body)
	}

	var t Token
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return err
	}

	c.mu.Lock()
	c.token = &t
	c.mu.Unlock()

	return c.saveToken(&t)
}

// CustID decodes the JWT access token payload and returns the iracing_cust_id.
func (c *Client) CustID() (int64, error) {
	token := c.GetAccessToken()
	if token == "" {
		return 0, fmt.Errorf("not authenticated")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	v, ok := claims["iracing_cust_id"]
	if !ok {
		return 0, fmt.Errorf("iracing_cust_id not found in token")
	}

	// JSON numbers unmarshal as float64
	switch id := v.(type) {
	case float64:
		return int64(id), nil
	default:
		return 0, fmt.Errorf("unexpected type for iracing_cust_id: %T", v)
	}
}

// generateVerifier creates a PKCE code verifier.
func generateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateChallenge computes the S256 PKCE challenge from a verifier.
func generateChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
