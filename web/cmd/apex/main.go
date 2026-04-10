package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"apex-iracing/web/assets"
	"apex-iracing/web/internal/browser"
	"apex-iracing/web/internal/oauth"
	"apex-iracing/web/internal/proxy"
)

const (
	iracingAPIBase = "https://members-ng.iracing.com/data"
	iracingDocsURL = "https://members-ng.iracing.com/data/doc"
)

// docsCache holds the API docs in memory with a RW lock.
type docsCache struct {
	mu   sync.RWMutex
	docs map[string]any
}

func (d *docsCache) get() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.docs
}

func (d *docsCache) set(v map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.docs = v
}

func main() {
	addr := flag.String("addr", "127.0.0.1:3000", "listen address")
	tokenFile := flag.String("token-file", ".iracing_token", "OAuth token file path")
	dataDir := flag.String("data-dir", "data", "directory for cached API docs")
	flag.Parse()

	oauthClient := oauth.NewClient(*tokenFile)
	proxyClient := proxy.New(oauthClient, iracingAPIBase)
	docsCachePath := *dataDir + "/data-docs.json"
	dc := &docsCache{}

	// Load docs on startup (from iRacing if authenticated, else from cache)
	dc.set(loadAPIDocs(oauthClient, proxyClient, docsCachePath))

	if oauthClient.IsAuthenticated() {
		fmt.Println("✅ Already authenticated - token loaded from", *tokenFile)
	} else {
		fmt.Println("Starting APEX on http://" + *addr)
		fmt.Println("Click 'Login' on the page to authenticate with iRacing.")
	}

	mux := http.NewServeMux()
	registerRoutes(mux, oauthClient, proxyClient, dc, docsCachePath)

	log.Printf("Listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func registerRoutes(mux *http.ServeMux, oauthClient *oauth.Client, proxyClient *proxy.Proxy, dc *docsCache, docsCachePath string) {
	// Static files — sub-FS rooted at "static/" so paths resolve correctly
	staticSub, err := fs.Sub(assets.Static, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /{$}", handleIndex())
	mux.HandleFunc("GET /login", handleLogin(oauthClient))
	mux.HandleFunc("GET /callback", handleCallback(oauthClient, proxyClient, dc, docsCachePath))
	mux.HandleFunc("GET /api/auth-status", handleAuthStatus(oauthClient))
	mux.HandleFunc("GET /api/docs", handleDocs(dc))
	mux.HandleFunc("GET /api/member-info", handleMemberInfo(oauthClient, proxyClient))
	mux.HandleFunc("GET /api/cars", handleCars(oauthClient, proxyClient))
	mux.HandleFunc("GET /api/search-series", handleSearchSeries(oauthClient, proxyClient))
	mux.HandleFunc("GET /api/search-chunk/{index}", handleSearchChunk(proxyClient))
	mux.HandleFunc("GET /api/proxy", handleProxy(oauthClient, proxyClient, dc))
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleIndex() http.HandlerFunc {
	tmpl := template.Must(template.ParseFS(assets.Templates, "templates/index.html"))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, nil); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	}
}

func handleLogin(oa *oauth.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authURL, err := oa.GenerateAuthURL()
		if err != nil {
			jsonError(w, "failed to generate auth URL: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = browser.Open(authURL) // best-effort; ignore error
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func handleCallback(oa *oauth.Client, p *proxy.Proxy, dc *docsCache, cachePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" || state == "" {
			jsonError(w, "missing code or state", http.StatusBadRequest)
			return
		}
		if err := oa.HandleCallback(code, state); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Refresh docs now that we're authenticated
		dc.set(loadAPIDocs(oa, p, cachePath))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Authentication Successful</title></head><body>
<h1>Authentication Successful!</h1>
<p>You can now close this window and return to the web interface.</p>
<p><a href="/">Back to APEX</a></p>
</body></html>`)
	}
}

func handleAuthStatus(oa *oauth.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]any{
			"authenticated": oa.IsAuthenticated(),
			"client_id":     "apex-api-explorer",
		})
	}
}

func handleDocs(dc *docsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, dc.get())
	}
}

func handleMemberInfo(oa *oauth.Client, p *proxy.Proxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !oa.IsAuthenticated() {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		memberID := r.URL.Query().Get("member_id")
		if memberID == "" {
			id, err := oa.CustID()
			if err != nil {
				jsonError(w, "could not determine member ID: "+err.Error(), http.StatusBadRequest)
				return
			}
			memberID = fmt.Sprintf("%d", id)
		}
		params := url.Values{"include_licenses": {"true"}, "cust_ids": {memberID}}
		proxyAndRespond(w, p, iracingAPIBase+"/member/get", params)
	}
}

func handleCars(oa *oauth.Client, p *proxy.Proxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !oa.IsAuthenticated() {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		proxyAndRespond(w, p, iracingAPIBase+"/car/get", nil)
	}
}

func handleSearchSeries(oa *oauth.Client, p *proxy.Proxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !oa.IsAuthenticated() {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		seasonYear := q.Get("season_year")
		seasonQuarter := q.Get("season_quarter")
		startRange := q.Get("start_range_begin")
		finishRange := q.Get("finish_range_begin")

		if seasonYear != "" && seasonQuarter == "" {
			jsonError(w, "season_quarter is required when using season_year", http.StatusBadRequest)
			return
		}
		if seasonQuarter != "" && seasonYear == "" {
			jsonError(w, "season_year is required when using season_quarter", http.StatusBadRequest)
			return
		}
		if seasonYear == "" && startRange == "" && finishRange == "" {
			jsonError(w, "at least one filter is required: season_year+season_quarter, start_range_begin, or finish_range_begin", http.StatusBadRequest)
			return
		}

		params := url.Values{}
		for _, k := range []string{"season_year", "season_quarter", "start_range_begin", "finish_range_begin", "cust_id", "team_id", "series_id", "race_week_num"} {
			if v := q.Get(k); v != "" {
				params.Set(k, v)
			}
		}
		proxyAndRespond(w, p, iracingAPIBase+"/results/search_series", params)
	}
}

func handleSearchChunk(p *proxy.Proxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		indexStr := r.PathValue("index")
		var index int
		if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
			jsonError(w, "invalid chunk index", http.StatusBadRequest)
			return
		}

		filesJSON := r.URL.Query().Get("files")
		baseURL := r.URL.Query().Get("base_url")
		if filesJSON == "" || baseURL == "" {
			jsonError(w, "missing chunk metadata", http.StatusBadRequest)
			return
		}

		var files []string
		if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
			jsonError(w, "invalid files parameter", http.StatusBadRequest)
			return
		}

		resp, err := p.FetchChunk(baseURL, files, index)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonOK(w, resp)
	}
}

func handleProxy(oa *oauth.Client, p *proxy.Proxy, dc *docsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !oa.IsAuthenticated() {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}

		endpoint := r.URL.Query().Get("_endpoint")
		if endpoint == "" {
			jsonError(w, "missing _endpoint parameter", http.StatusBadRequest)
			return
		}

		parts := strings.SplitN(endpoint, "/", 2)
		if len(parts) != 2 {
			jsonError(w, "invalid endpoint format, expected category/name", http.StatusBadRequest)
			return
		}
		category, name := parts[0], parts[1]

		docs := dc.get()
		cat, ok := docs[category].(map[string]any)
		if !ok {
			jsonError(w, "unknown endpoint: "+endpoint, http.StatusNotFound)
			return
		}
		ep, ok := cat[name].(map[string]any)
		if !ok {
			jsonError(w, "unknown endpoint: "+endpoint, http.StatusNotFound)
			return
		}
		link, _ := ep["link"].(string)
		if link == "" {
			jsonError(w, "endpoint has no link", http.StatusInternalServerError)
			return
		}

		params := url.Values{}
		for k, vs := range r.URL.Query() {
			if k != "_endpoint" {
				params[k] = vs
			}
		}
		proxyAndRespond(w, p, link, params)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// proxyAndRespond makes the API request, processes the response, and writes JSON.
func proxyAndRespond(w http.ResponseWriter, p *proxy.Proxy, apiURL string, params url.Values) {
	body, err := p.MakeRequest(apiURL, params)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "not authenticated") {
			jsonError(w, err.Error(), http.StatusUnauthorized)
		} else {
			jsonError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	resp, err := p.ProcessResponse(body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// loadAPIDocs fetches API docs from iRacing (if authenticated), falling back
// to the local cache file.
func loadAPIDocs(oa *oauth.Client, p *proxy.Proxy, cachePath string) map[string]any {
	if oa.IsAuthenticated() {
		body, err := p.MakeRequest(iracingDocsURL, nil)
		if err == nil {
			var docs map[string]any
			if json.Unmarshal(body, &docs) == nil {
				if data, err := json.Marshal(docs); err == nil {
					_ = os.WriteFile(cachePath, data, 0644)
				}
				log.Println("API docs fetched from iRacing")
				return docs
			}
		}
		log.Printf("Could not fetch API docs (%v), falling back to cache", err)
	} else {
		log.Println("Not authenticated — loading API docs from cache")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		log.Println("No API docs cache found — authenticate to load docs")
		return map[string]any{}
	}
	var docs map[string]any
	if err := json.Unmarshal(data, &docs); err != nil {
		log.Println("Failed to parse cached API docs")
		return map[string]any{}
	}
	log.Println("API docs loaded from cache")
	return docs
}
