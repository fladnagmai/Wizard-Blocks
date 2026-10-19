package work

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/address"
	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
)

// proposalNode answers getblocktemplate in proposal mode with verdict and
// records the block it was asked to check.
func proposalNode(t *testing.T, verdict string, got *atomic.Value, calls *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		if req.Method != "getblocktemplate" || len(req.Params) != 1 || req.Params[0]["mode"] != "proposal" {
			io.WriteString(w, `{"result":null,"error":{"code":-32601,"message":"not found"}}`)
			return
		}
		calls.Add(1)
		got.Store(req.Params[0]["data"])
		res := "null"
		if verdict != "" {
			res = `"` + verdict + `"`
		}
		io.WriteString(w, `{"result":`+res+`,"error":null}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func checkFixture(t *testing.T, url string) (*Manager, *stats.Collector, *Job) {
	t.Helper()
	st := stats.New("bch", "t", "")
	p, _ := ParamsFor(address.BCH)
	m := NewManager(Config{Params: p, Chain: "regtest", Extranonce2Size: 8, PollInterval: time.Second, RefreshInterval: time.Second},
		node.NewClient(url, "u", "p", "", 5*time.Second), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	raw := &node.BlockTemplate{Version: 536870912, PreviousBlockHash: strings.Repeat("0", 63) + "1",
		CoinbaseValue: 5000000000, MinTime: 1000, CurTime: 2000, Bits: "207fffff", Height: 1}
	tmpl, err := NewTemplate(raw, p, "regtest")
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := BuildCoinbase(CoinbaseParams{Height: 1, Extranonce2Size: 8, PayoutScript: []byte{0x51}, Value: tmpl.CoinbaseValue, MinTxSize: 100})
	return m, st, newJob("1", 1, tmpl, []byte{0x51}, "x", cb)
}

func waitCheck(t *testing.T, st *stats.Collector) stats.TemplateInfo {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ti := st.Snapshot().Template; ti.BlockCheck != "" {
			return ti
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no block check result")
	return stats.TemplateInfo{}
}

// The node is asked about exactly the block a found share would submit
// (same coinbase, payout and transactions), and its verdict is shown.
func TestBlockCheckValid(t *testing.T) {
	var got atomic.Value
	var calls atomic.Int32
	m, st, job := checkFixture(t, proposalNode(t, "", &got, &calls).URL)
	m.checkBlock(job)
	ti := waitCheck(t, st)
	if ti.BlockCheck != "valid" || ti.BlockCheckHeight != 1 {
		t.Fatalf("check: %+v", ti)
	}
	en1, en2 := make([]byte, 4), make([]byte, 8)
	h := job.Header(en1, en2, job.Tmpl.CurTime, 0, job.Tmpl.Version)
	if got.Load() != hex.EncodeToString(job.Block(&h, en1, en2)) {
		t.Fatal("the node was not asked about the block a share would submit")
	}
	// Same job again: not re-checked.
	m.checkBlock(job)
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("checked %d times, want 1", calls.Load())
	}
}

// A rejection is shown with the node's reason.
func TestBlockCheckRejected(t *testing.T) {
	var got atomic.Value
	var calls atomic.Int32
	m, st, job := checkFixture(t, proposalNode(t, "bad-cb-amount", &got, &calls).URL)
	m.checkBlock(job)
	if ti := waitCheck(t, st); ti.BlockCheck != "REJECTED: bad-cb-amount" {
		t.Fatalf("check: %q", ti.BlockCheck)
	}
}

// A new block, or the interval passing, makes a new check due; nothing
// else does.
func TestBlockCheckDue(t *testing.T) {
	_, _, job := checkFixture(t, "http://127.0.0.1:1")
	var b blockChecker
	now := time.Unix(1000, 0)
	if !b.due(job, now) {
		t.Fatal("first job not checked")
	}
	if b.due(job, now) {
		t.Fatal("checked while a check is running")
	}
	b.done(job, now)
	j2 := *job
	j2.Gen = 2 // same block, refreshed template
	if b.due(&j2, now.Add(time.Minute)) {
		t.Fatal("re-checked the same block within the interval")
	}
	if !b.due(&j2, now.Add(blockCheckEvery)) {
		t.Fatal("not re-checked after the interval")
	}
	b.done(&j2, now.Add(blockCheckEvery))
	t3 := *job.Tmpl
	t3.PrevHash[0] ^= 1 // a new block
	j3 := *job
	j3.Gen, j3.Tmpl = 3, &t3
	if !b.due(&j3, now.Add(blockCheckEvery+time.Second)) {
		t.Fatal("new block not checked")
	}
}
