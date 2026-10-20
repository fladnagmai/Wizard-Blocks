package nodestatus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/node"
)

func dogeCache() *CacheConfig {
	return &CacheConfig{Default: 200, Options: []int{200, 450, 1000, 2000, 4000}}
}

func cacheNode(srvURL, conf string) []Node {
	return []Node{{Key: "doge", Name: "Dogecoin Node", RPC: node.NewClient(srvURL, "", "", "", time.Second), Prune: dogePrune(conf), Cache: dogeCache()}}
}

// Raise the cache for the first sync, set it back afterwards: each change
// keeps the prune target, is remembered, and restarts the node once.
func TestSetCache(t *testing.T) {
	sn, srv := newSwitchNode(t)
	sn.prune = 5000 << 20
	dir := t.TempDir()
	conf, state := filepath.Join(dir, "dogecoin.conf"), filepath.Join(dir, "state.json")
	p := NewPoller(cacheNode(srv.URL, conf), time.Hour, state)
	if fixed, err := p.EnsurePruneConf(); err != nil || len(fixed) != 1 {
		t.Fatalf("first start: %v %v", fixed, err)
	}
	if v, c, _ := readConf(conf); v != 2200 || c != 200 || !ConfReady(conf, 2200, true) {
		t.Fatalf("defaults: prune %d cache %d", v, c)
	}
	writePrune(conf, 5000) // the owner's prune choice survives cache changes
	ctx := context.Background()
	if err := p.SetCache(ctx, "doge", 333); !errors.Is(err, ErrBadCache) {
		t.Fatalf("not an option: %v", err)
	}
	if err := p.SetCache(ctx, "ltc", 450); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("unknown node: %v", err)
	}
	if sn.stopCount() != 0 {
		t.Fatal("a refused change restarted the node")
	}
	if err := p.SetCache(ctx, "doge", 2000); err != nil {
		t.Fatal(err)
	}
	if v, c, _ := readConf(conf); v != 5000 || c != 2000 || sn.stopCount() != 1 {
		t.Fatalf("raised: prune %d cache %d stops %d", v, c, sn.stopCount())
	}
	p.CheckAll(ctx)
	if s := p.Snapshot()[0]; s.CacheSet != 2000 || len(s.CacheOptions) != 5 {
		t.Fatalf("status: %+v", s)
	}
	if err := p.SetCache(ctx, "doge", 200); err != nil { // synced: back to the default
		t.Fatal(err)
	}
	if _, c, _ := readConf(conf); c != 200 || sn.stopCount() != 2 {
		t.Fatalf("set back: cache %d stops %d", c, sn.stopCount())
	}
	// Remembered across a restart of the page: a file without the line (an
	// app update) gets the saved choice back, without restarting a node that
	// has not read it yet.
	p.SetCache(ctx, "doge", 1000)
	writePrune(conf, 5000)
	os.WriteFile(conf, []byte("prune=5000\n"), 0o644)
	p2 := NewPoller(cacheNode(srv.URL, conf), time.Hour, state)
	if fixed, err := p2.EnsurePruneConf(); err != nil || len(fixed) != 1 {
		t.Fatalf("reconcile: %v %v", fixed, err)
	}
	if v, c, _ := readConf(conf); v != 5000 || c != 1000 {
		t.Fatalf("after reconcile: prune %d cache %d", v, c)
	}
	before := sn.stopCount()
	p2.CheckAll(ctx)
	if sn.stopCount() != before {
		t.Fatal("restarted a node for a cache line it had not read yet")
	}
}

// A cache changed behind the page's back is put back and the node
// restarted, even though its prune target already matches.
func TestEnsureCacheRestoresChanged(t *testing.T) {
	sn, srv := newSwitchNode(t)
	sn.prune = 2200 << 20
	conf := filepath.Join(t.TempDir(), "dogecoin.conf")
	writeConf(conf, 2200, 999)
	p := NewPoller(cacheNode(srv.URL, conf), time.Hour, "")
	if fixed, _ := p.EnsurePruneConf(); len(fixed) != 1 {
		t.Fatal("changed cache not fixed")
	}
	if _, c, _ := readConf(conf); c != 200 {
		t.Fatalf("cache %d", c)
	}
	p.CheckAll(context.Background())
	if sn.stopCount() != 1 {
		t.Fatalf("stops %d, want 1", sn.stopCount())
	}
}

// A cache change made while the node warms up applies once it answers.
func TestSetCacheDuringWarmup(t *testing.T) {
	sn, srv := newSwitchNode(t)
	sn.prune, sn.warmup = 2200<<20, true
	conf := filepath.Join(t.TempDir(), "dogecoin.conf")
	writeConf(conf, 2200, 200)
	p := NewPoller(cacheNode(srv.URL, conf), time.Hour, "")
	if err := p.SetCache(context.Background(), "doge", 2000); err != nil {
		t.Fatal(err)
	}
	p.CheckAll(context.Background())
	if sn.stopCount() != 0 {
		t.Fatal("stopped while warming up")
	}
	sn.mu.Lock()
	sn.warmup = false
	sn.mu.Unlock()
	p.CheckAll(context.Background())
	if sn.stopCount() != 1 {
		t.Fatalf("cache never applied: stops %d", sn.stopCount())
	}
}

func TestCacheHTTP(t *testing.T) {
	sn, srv := newSwitchNode(t)
	conf := filepath.Join(t.TempDir(), "dogecoin.conf")
	writeConf(conf, 2200, 200)
	p := NewPoller(cacheNode(srv.URL, conf), time.Hour, "")
	hs := httptest.NewServer(Handler(p, "t"))
	defer hs.Close()
	post := func(body string, hdr bool) int {
		req, _ := http.NewRequest(http.MethodPost, hs.URL+"/api/nodes/doge/cache", strings.NewReader(body))
		if hdr {
			req.Header.Set("X-NS-Action", "1")
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if c := post(`{"mib":2000}`, false); c != 403 {
		t.Fatalf("without the page's header: %d", c)
	}
	if c := post(`{"mib":123}`, true); c != 400 {
		t.Fatalf("bad size: %d", c)
	}
	if c := post(`{}`, true); c != 400 {
		t.Fatalf("no size: %d", c)
	}
	if _, c, _ := readConf(conf); c != 200 || sn.stopCount() != 0 {
		t.Fatal("refused requests changed something")
	}
	if c := post(`{"mib":2000}`, true); c != 200 {
		t.Fatalf("set: %d", c)
	}
	if _, c, _ := readConf(conf); c != 2000 || sn.stopCount() != 1 {
		t.Fatalf("cache %d stops %d", c, sn.stopCount())
	}
}
