package merged

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
)

// fakeDogecoind answers createauxblock like dogecoind 1.14.9: a block while
// mode is "ok"; AuxMiningCheck's errors for "nopeers" (-9, e.g. its p2p
// network switched off from the node app's pause) and "ibd" (-10); nothing
// for "down".
type fakeDogecoind struct {
	mu   sync.Mutex
	mode string
	hash string
}

func (f *fakeDogecoind) set(mode string) { f.mu.Lock(); f.mode = mode; f.mu.Unlock() }

func (f *fakeDogecoind) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	mode, hash := f.mode, f.hash
	f.mu.Unlock()
	if mode == "down" {
		hj, _ := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		c.Close()
		return
	}
	var req struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	b, _ := io.ReadAll(r.Body)
	json.Unmarshal(b, &req)
	resp := map[string]any{"id": req.ID, "result": nil, "error": nil}
	switch {
	case req.Method == "getbestblockhash":
		resp["result"] = strings.Repeat("11", 32)
	case req.Method == "createauxblock" && mode == "nopeers":
		resp["error"] = map[string]any{"code": -9, "message": "Dogecoin is not connected!"}
	case req.Method == "createauxblock" && mode == "ibd":
		resp["error"] = map[string]any{"code": -10, "message": "Dogecoin is downloading blocks..."}
	case req.Method == "createauxblock":
		resp["result"] = map[string]any{
			"hash": hash, "chainid": DogeChainID, "previousblockhash": strings.Repeat("11", 32),
			"coinbasevalue": 1_000_000_000_000, "bits": "207fffff", "height": 5412346,
			// "target" is the bits' target in little-endian byte order.
			"target": strings.Repeat("00", 29) + "ffff7f",
		}
	default:
		resp["error"] = map[string]any{"code": -32601, "message": "Method not found"}
	}
	if resp["error"] != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(resp)
}

// Merged mining switches off while the Dogecoin node has no peers (its sync
// paused from the node app: dogecoind then refuses createauxblock with -9
// on mainnet; regtest skips that check, so the regtests cannot see it), the
// engine is told at once (Changed), the page shows why, and merged mining
// comes back on its own when the node reconnects.
func TestDogeMergedMiningOffWhileNoPeers(t *testing.T) {
	f := &fakeDogecoind{mode: "ok", hash: strings.Repeat("ab", 32)}
	srv := httptest.NewServer(f)
	defer srv.Close()
	st := stats.New("ltc", "t", "")
	d := NewDoge(DogeConfig{PayoutAddr: "DQkwDpRYUyNNnoBjfERSzPWbKdtr2JUSyL", Poll: time.Hour, Refresh: time.Hour},
		node.NewClient(srv.URL, "u", "p", "", time.Second), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	changed := func() bool {
		select {
		case <-d.Changed():
			return true
		default:
			return false
		}
	}
	aux := func() stats.AuxStatus { return st.Snapshot().Aux["doge"] }

	d.fetch(ctx, "test")
	if len(d.Current()) != 1 || !changed() || !aux().Merged || aux().Reason != "" {
		t.Fatalf("merged mining not on at start: %v %+v", d.Current(), aux())
	}

	// Sync paused: the node drops its peers and refuses aux work.
	f.set("nopeers")
	d.fetch(ctx, "test")
	if len(d.Current()) != 0 {
		t.Fatal("still offering DOGE aux work while the node has no peers")
	}
	if !changed() {
		t.Fatal("engine not told that DOGE aux work is gone (no clean job without the tag)")
	}
	if a := aux(); a.Merged || a.Reason != ReasonNodeNoPeers || !a.Connected {
		t.Fatalf("status while paused: %+v", a)
	}
	// Repeated checks while paused do not signal again.
	d.fetch(ctx, "test")
	if changed() {
		t.Fatal("signalled again with nothing changed")
	}

	// Still syncing after the resume: off, with its own reason.
	f.set("ibd")
	d.fetch(ctx, "test")
	if a := aux(); a.Merged || a.Reason != ReasonNodeSyncing || len(d.Current()) != 0 {
		t.Fatalf("status while syncing: %+v", a)
	}

	// Node unreachable: off, "node down".
	f.set("down")
	d.fetch(ctx, "test")
	if a := aux(); a.Merged || a.Reason != ReasonNodeDown || a.Connected {
		t.Fatalf("status while down: %+v", a)
	}

	// Resumed and caught up: merged mining back on by itself.
	f.set("ok")
	d.fetch(ctx, "test")
	if len(d.Current()) != 1 || !changed() || !aux().Merged || aux().Reason != "" {
		t.Fatalf("merged mining not back after the resume: %v %+v", d.Current(), aux())
	}
}
