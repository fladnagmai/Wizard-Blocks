package poolview

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fladnagmai/wizard-blocks/internal/ui"
)

// A fake engine: its state carries payout addresses and an address-named
// worker; its admin API must never be reachable through the view.
func fakeEngine(t *testing.T) (*httptest.Server, *int) {
	admin := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin") {
			admin++
		}
		if r.URL.Path != "/api/state" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"coin":{"coin":"bch","ticker":"BCH","address_hint":{"main":"bitcoincash:q…"}},
			"stratum":{"port":62023,"payout_set":true,"payout_address":"bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a"},
			"pool":{"best_share_worker":"qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a.rig1"},
			"workers":[{"name":"ash","hashrate_live":1e12},{"name":"bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a"}],
			"chains":{"ltc":{"payout_address":"ltc1qg82e7d7ky5uc8x0xmdmfdzt6yp7eqlzg7gj9pr"}},
			"blocks":[{"height":1,"worker":"ash","address":"bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &admin
}

func view(t *testing.T, eng string) *httptest.Server {
	s := New([]Pool{{Key: "bch", Label: "BCH", URL: eng}, {Key: "btc", Label: "BTC", URL: "http://127.0.0.1:1"}},
		ui.SimpleFiles(), ui.StaticFiles(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (int, string, http.Header) {
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b), r.Header
}

// The view's state has no payout address anywhere, and worker names that
// are addresses are cut to their ends; ordinary names stay.
func TestStateWithoutAddresses(t *testing.T) {
	eng, _ := fakeEngine(t)
	srv := view(t, eng.URL)
	code, body, _ := get(t, srv.URL+"/bch/api/state")
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if strings.Contains(body, "qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a") || strings.Contains(body, "ltc1qg82e7d") || strings.Contains(body, "payout_address") {
		t.Fatalf("address leaked: %s", body)
	}
	var st map[string]any
	json.Unmarshal([]byte(body), &st)
	ws := st["workers"].([]any)
	if ws[0].(map[string]any)["name"] != "ash" || ws[1].(map[string]any)["name"] != "bitc…dx6a" {
		t.Fatalf("worker names: %v", ws)
	}
	if st["pool"].(map[string]any)["best_share_worker"] != "qpm2…dx6a.rig1" {
		t.Fatalf("best share worker: %v", st["pool"])
	}
	if st["stratum"].(map[string]any)["payout_set"] != true || st["coin"].(map[string]any)["address_hint"] == nil {
		t.Fatal("ordinary fields lost")
	}
	// A pool that is not running answers 502 with a reason, not a hang.
	if code, body, _ := get(t, srv.URL+"/btc/api/state"); code != http.StatusBadGateway || !strings.Contains(body, "BTC pool") {
		t.Fatalf("down pool: %d %s", code, body)
	}
}

// Only the pages and the state: no settings, no admin API, no writes.
func TestViewOnly(t *testing.T) {
	eng, admin := fakeEngine(t)
	srv := view(t, eng.URL)
	for _, p := range []string{"/bch/api/admin/settings", "/bch/api/admin/session", "/bch/api/events", "/bch/api/history", "/ltc/", "/nope", "/bch/../../etc/passwd"} {
		if code, _, _ := get(t, srv.URL+p); code != http.StatusNotFound {
			t.Fatalf("GET %s: %d", p, code)
		}
	}
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		req, _ := http.NewRequest(m, srv.URL+"/bch/api/state", strings.NewReader("{}"))
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s: %d", m, r.StatusCode)
		}
	}
	if *admin != 0 {
		t.Fatal("an admin request reached the engine")
	}
	// The pool page loads view.js before app.js; /bch redirects to /bch/.
	code, page, h := get(t, srv.URL+"/bch/")
	if code != 200 || !strings.Contains(page, `<script src="/view.js"></script>`) || strings.Index(page, "/view.js") > strings.Index(page, "app.js") {
		t.Fatalf("pool page: %d", code)
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("no CSP")
	}
	if code, _, h := get(t, srv.URL+"/bch"); code != http.StatusFound || h.Get("Location") != "/bch/" {
		t.Fatalf("redirect: %d %s", code, h.Get("Location"))
	}
	for _, p := range []string{"/", "/portal.js", "/sprites.js", "/view.js", "/view.css", "/bch/app.js", "/bch/style.css"} {
		if code, _, _ := get(t, srv.URL+p); code != 200 {
			t.Fatalf("GET %s: %d", p, code)
		}
	}
	_, pools, _ := get(t, srv.URL+"/pools.json")
	if pools != `[{"key":"bch","label":"BCH"},{"key":"btc","label":"BTC"}]`+"\n" {
		t.Fatalf("pools: %s", pools)
	}
}
