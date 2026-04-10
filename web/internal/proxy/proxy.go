package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"apex-iracing/web/internal/oauth"
)

// Proxy handles iRacing API requests, S3 resolution, and chunked responses.
type Proxy struct {
	oauth  *oauth.Client
	base   string
	client *http.Client
}

// New creates a Proxy for the given OAuth client and API base URL.
func New(oauthClient *oauth.Client, baseURL string) *Proxy {
	return &Proxy{
		oauth: oauthClient,
		base:  baseURL,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// MakeRequest performs a GET to the given URL with Bearer auth, retrying once
// on 401 after attempting a token refresh.
func (p *Proxy) MakeRequest(rawURL string, params url.Values) ([]byte, error) {
	body, status, err := p.doGet(rawURL, params)
	if err != nil {
		return nil, err
	}

	if status == http.StatusUnauthorized {
		if refreshErr := p.oauth.RefreshAccessToken(); refreshErr != nil {
			_ = p.oauth.ClearToken()
			return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
		}
		body, status, err = p.doGet(rawURL, params)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized {
			return nil, fmt.Errorf("unauthorized after token refresh")
		}
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("API request failed (%d): %s", status, body)
	}

	return body, nil
}

func (p *Proxy) doGet(rawURL string, params url.Values) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.oauth.GetAccessToken())
	if len(params) > 0 {
		req.URL.RawQuery = params.Encode()
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// Response is the unified envelope returned to the frontend.
type Response struct {
	Type         string    `json:"type"`
	Data         any       `json:"data,omitempty"`
	Expires      string    `json:"expires,omitempty"`
	ChunkInfo    ChunkMeta `json:"chunk_info,omitempty"`
	CurrentChunk int       `json:"current_chunk,omitempty"`
}

// ChunkMeta holds pagination metadata for chunked responses.
type ChunkMeta struct {
	TotalChunks int      `json:"total_chunks,omitempty"`
	TotalRows   int      `json:"total_rows,omitempty"`
	ChunkSize   int      `json:"chunk_size,omitempty"`
	ChunkFiles  []string `json:"chunk_files,omitempty"`
	BaseURL     string   `json:"base_url,omitempty"`
}

// ProcessResponse inspects an API response and handles the three patterns:
//  1. Simple S3:  top-level "link" key → fetch S3, return {type:"simple_s3", ...}
//  2. Chunked:    data.chunk_info present → fetch first chunk, return {type:"chunked", ...}
//  3. Direct:     everything else → return {type:"direct", data: ...}
func (p *Proxy) ProcessResponse(raw []byte) (*Response, error) {
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		// Not a JSON object — return as direct
		var val any
		_ = json.Unmarshal(raw, &val)
		return &Response{Type: "direct", Data: val}, nil
	}

	// Pattern 1: simple S3 link
	if linkRaw, ok := generic["link"]; ok {
		var s3URL string
		if err := json.Unmarshal(linkRaw, &s3URL); err != nil {
			return nil, fmt.Errorf("invalid link field: %w", err)
		}
		s3Data, err := p.FetchS3Data(s3URL)
		if err != nil {
			return nil, err
		}
		var expires string
		if exp, ok := generic["expires"]; ok {
			_ = json.Unmarshal(exp, &expires)
		}
		return &Response{Type: "simple_s3", Expires: expires, Data: s3Data}, nil
	}

	// Pattern 2: chunked
	if dataRaw, ok := generic["data"]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(dataRaw, &inner); err == nil {
			if ciRaw, ok := inner["chunk_info"]; ok {
				var ci struct {
					BaseDownloadURL string   `json:"base_download_url"`
					ChunkFileNames  []string `json:"chunk_file_names"`
					NumChunks       int      `json:"num_chunks"`
					Rows            int      `json:"rows"`
					ChunkSize       int      `json:"chunk_size"`
				}
				if err := json.Unmarshal(ciRaw, &ci); err != nil {
					return nil, fmt.Errorf("invalid chunk_info: %w", err)
				}

				var firstChunk any
				if len(ci.ChunkFileNames) > 0 {
					firstChunk, err = p.FetchS3Data(ci.BaseDownloadURL + ci.ChunkFileNames[0])
					if err != nil {
						return nil, err
					}
				}

				return &Response{
					Type: "chunked",
					ChunkInfo: ChunkMeta{
						TotalChunks: ci.NumChunks,
						TotalRows:   ci.Rows,
						ChunkSize:   ci.ChunkSize,
						ChunkFiles:  ci.ChunkFileNames,
						BaseURL:     ci.BaseDownloadURL,
					},
					CurrentChunk: 0,
					Data:         firstChunk,
				}, nil
			}
		}
	}

	// Pattern 3: direct
	var val any
	_ = json.Unmarshal(raw, &val)
	return &Response{Type: "direct", Data: val}, nil
}

// FetchS3Data fetches data from an S3 URL. JSON responses are decoded into a
// value; non-JSON responses (e.g. CSV) are returned as a raw string.
func (p *Proxy) FetchS3Data(rawURL string) (any, error) {
	resp, err := p.client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch S3 data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("S3 fetch failed (%d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 response: %w", err)
	}

	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		// Not JSON (e.g. CSV) — return as raw string
		return string(body), nil
	}
	return v, nil
}

// FetchChunk fetches a specific chunk by index from the given base URL + file list.
func (p *Proxy) FetchChunk(baseURL string, files []string, index int) (*Response, error) {
	if index < 0 || index >= len(files) {
		return nil, fmt.Errorf("chunk index %d out of range (0-%d)", index, len(files)-1)
	}
	data, err := p.FetchS3Data(baseURL + files[index])
	if err != nil {
		return nil, err
	}
	return &Response{
		Type:         "chunked",
		CurrentChunk: index,
		Data:         data,
	}, nil
}
