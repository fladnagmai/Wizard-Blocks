// Package poolview serves the public, view-only pages of the Wizard-Blocks
// pools (the Wizard-Blocks View Umbrel app): a portal page with the wizard,
// and each pool's stats page without its settings. It only reads each
// engine's /api/state, strips payout addresses (and worker names that are
// addresses) from it, and never forwards anything else: no settings, no
// logins, no writes.
package poolview

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

// Pool is one engine whose page is shown.
type Pool struct {
	Key   string // "bch", "btc", "ltc": the page's path
	Label string // tab text: "BCH"
	URL   string // the engine's UI base URL, e.g. http://fladnagmai-wizard-blocks_engine_1:8420
}

// Server is the public view server.
type Server struct {
	pools   []Pool
	simple  fs.FS // the pools' one-page UI (internal/ui simple/)
	sprites fs.FS // the Mine UI's files, for the wizard sprite
	client  *http.Client
	log     *slog.Logger

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at   time.Time
	body []byte
	code int
}

// New creates the server. simple is the pools' stats page (ui.SimpleFiles),
// static the Mine UI's files (ui.StaticFiles, for js/sprites.js).
func New(pools []Pool, simple, static fs.FS, log *slog.Logger) *Server {
	return &Server{pools: pools, simple: simple, sprites: static, log: log,
		client: &http.Client{Timeout: 5 * time.Second}, cache: map[string]cached{}}
}

func (s *Server) pool(key string) *Pool {
	for i := range s.pools {
		if s.pools[i].Key == key {
			return &s.pools[i]
		}
	}
	return nil
}

// Handler serves:
//
//	/                     the portal (click the wizard)
//	/sprites.js           the wizard's sprite data
//	/pools.json           the pools shown, for the tabs
//	/<pool>/              that pool's stats page, view only
//	/<pool>/api/state     its state, without payout addresses
//	/<pool>/api/worker    a worker's hashrate graph data (not for address names)
//
// Only GET and HEAD; everything else is 404 or 405.
func (s *Server) Handler() http.Handler {
	portal, _ := fs.Sub(webFS, "web")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		p := path.Clean("/" + r.URL.Path)
		switch {
		case p == "/" || p == "/index.html":
			serveFile(w, r, portal, "index.html")
			return
		case p == "/portal.js" || p == "/portal.css" || p == "/view.js" || p == "/view.css":
			serveFile(w, r, portal, strings.TrimPrefix(p, "/"))
			return
		case p == "/sprites.js":
			serveFile(w, r, s.sprites, "js/sprites.js")
			return
		case p == "/pools.json":
			type tab struct {
				Key   string `json:"key"`
				Label string `json:"label"`
			}
			var tabs []tab
			for _, pl := range s.pools {
				tabs = append(tabs, tab{pl.Key, pl.Label})
			}
			h.Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tabs)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)
		pl := s.pool(parts[0])
		if pl == nil {
			http.NotFound(w, r)
			return
		}
		rest := ""
		if len(parts) == 2 {
			rest = parts[1]
		}
		if rest == "" && !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, "/"+pl.Key+"/", http.StatusFound) // relative URLs need the slash
			return
		}
		switch rest {
		case "", "index.html":
			s.servePage(w, r)
		case "app.js", "style.css":
			serveFile(w, r, s.simple, rest)
		case "api/state":
			s.serveState(w, r, pl)
		case "api/worker":
			s.serveWorker(w, r, pl)
		default:
			http.NotFound(w, r)
		}
	})
}

func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

// servePage serves the pool's stats page with view.js loaded first: it marks
// the page view only (app.js then leaves out the settings) and adds the tabs.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(s.simple, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page := strings.Replace(string(b), `<link rel="stylesheet" href="style.css">`,
		`<link rel="stylesheet" href="style.css">`+"\n"+`<link rel="stylesheet" href="/view.css">`+"\n"+`<script src="/view.js"></script>`, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, page)
}

// serveWorker passes on a worker's graph data. Workers named after an
// address are shown shortened, so their graphs are not offered.
func (s *Server) serveWorker(w http.ResponseWriter, r *http.Request, pl *Pool) {
	name := r.URL.Query().Get("name")
	if name == "" || len(name) > 256 || maskName(name) != name {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(pl.URL, "/")+"/api/worker?name="+url.QueryEscape(name), nil)
	res, err := s.client.Do(req)
	if err != nil {
		http.Error(w, pl.Label+" pool not answering", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		http.NotFound(w, r)
		return
	}
	var v any
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&v); err != nil {
		http.Error(w, pl.Label+" pool sent bad data", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Sanitize(v))
}

// serveState answers with the engine's state minus payout addresses, cached
// for 3 seconds so many viewers cost the engine one request.
func (s *Server) serveState(w http.ResponseWriter, r *http.Request, pl *Pool) {
	s.mu.Lock()
	c, ok := s.cache[pl.Key]
	s.mu.Unlock()
	if !ok || time.Since(c.at) > 3*time.Second {
		c = s.fetch(r.Context(), pl)
		s.mu.Lock()
		s.cache[pl.Key] = c
		s.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c.code)
	w.Write(c.body)
}

func (s *Server) fetch(ctx context.Context, pl *Pool) cached {
	fail := func(msg string) cached {
		b, _ := json.Marshal(map[string]string{"error": msg})
		return cached{at: time.Now(), body: b, code: http.StatusBadGateway}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(pl.URL, "/")+"/api/state", nil)
	res, err := s.client.Do(req)
	if err != nil {
		var ue interface{ Timeout() bool }
		if errors.As(err, &ue) && ue.Timeout() {
			return fail(pl.Label + " pool not answering")
		}
		return fail(pl.Label + " pool not running")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil || res.StatusCode != http.StatusOK {
		return fail(pl.Label + " pool not answering")
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fail(pl.Label + " pool sent bad data")
	}
	out, _ := json.Marshal(Sanitize(v))
	return cached{at: time.Now(), body: out, code: http.StatusOK}
}

// hiddenKeys are removed wherever they appear: payout addresses.
var hiddenKeys = map[string]bool{"payout_address": true, "address": true, "doge_payout_address": true}

// nameKeys hold worker names, which some miners set to their own address.
var nameKeys = map[string]bool{"name": true, "worker": true, "best_share_worker": true, "best_worker": true}

// looksLikeAddress matches the address forms of BTC, BCH, LTC, DOGE and PEP
// (CashAddr with or without prefix, bech32, Base58).
var looksLikeAddress = regexp.MustCompile(`^(?i)(bitcoincash:|bchtest:|bchreg:)?(q|p)[0-9a-z]{38,}$|^(?i)(bc|tb|bcrt|ltc|tltc|rltc)1[0-9a-z]{20,}$|^[1-9A-HJ-NP-Za-km-z]{25,40}$`)

// maskName shortens a worker name that is (or starts with) an address to
// its first and last 4 characters; other names are kept.
func maskName(n string) string {
	base, suffix, _ := strings.Cut(n, ".")
	if !looksLikeAddress.MatchString(base) {
		return n
	}
	masked := base[:4] + "…" + base[len(base)-4:]
	if suffix != "" {
		masked += "." + suffix
	}
	return masked
}

// Sanitize removes payout addresses from a decoded state and masks worker
// names that are addresses.
func Sanitize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if hiddenKeys[k] {
				delete(t, k)
				continue
			}
			if s, ok := x.(string); ok && nameKeys[k] {
				t[k] = maskName(s)
				continue
			}
			t[k] = Sanitize(x)
		}
		return t
	case []any:
		for i := range t {
			t[i] = Sanitize(t[i])
		}
		return t
	}
	return v
}
