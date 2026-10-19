package work

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/bitcoin"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
)

// Block check: the node validates the exact block a job would produce if a
// share met the network target (getblocktemplate proposal mode: everything
// except proof of work: coinbase, payout, transactions, witness commitment,
// Litecoin's MWEB part). It runs for the first job of each new block and at
// most every blockCheckEvery otherwise, so the page shows that real blocks
// are being built correctly long before one is found.

var blockCheckEvery = 2 * time.Minute

type blockChecker struct {
	mu       sync.Mutex
	running  bool
	lastPrev bitcoin.Hash
	lastGen  uint64
	lastAt   time.Time
}

// due reports whether job j should be checked now and, if so, marks a
// check as running.
func (b *blockChecker) due(j *Job, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running || j.Gen == b.lastGen && !b.lastAt.IsZero() {
		return false
	}
	if j.Tmpl.PrevHash == b.lastPrev && now.Sub(b.lastAt) < blockCheckEvery {
		return false
	}
	b.running = true
	return true
}

func (b *blockChecker) done(j *Job, now time.Time) {
	b.mu.Lock()
	b.running, b.lastPrev, b.lastGen, b.lastAt = false, j.Tmpl.PrevHash, j.Gen, now
	b.mu.Unlock()
}

// proposalBlock is the block j would produce, with zero extranonces, the
// template's time and version, and nonce 0 (proof of work is not checked).
func (j *Job) proposalBlock(en2Size int) []byte {
	en1, en2 := make([]byte, 4), make([]byte, en2Size)
	h := j.Header(en1, en2, j.Tmpl.CurTime, 0, j.Tmpl.Version)
	return j.Block(&h, en1, en2)
}

// checkBlock runs the block check for j in the background when due.
func (m *Manager) checkBlock(j *Job) {
	now := time.Now()
	if m.rpc == nil || !m.check.due(j, now) { // no node: nothing to ask
		return
	}
	go func() {
		defer m.check.done(j, time.Now())
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data := hex.EncodeToString(j.proposalBlock(m.cfg.Extranonce2Size))
		var res *string
		err := m.rpc.Call(ctx, "getblocktemplate", []any{map[string]any{"mode": "proposal", "data": data}}, &res)
		result := "valid"
		switch {
		case err != nil:
			result = "not checked: " + err.Error()
			m.log.Warn("block check: node did not answer", "height", j.Tmpl.Height, "err", err)
		case res == nil || *res == "":
			m.log.Info("block check passed: the node accepts the block this job would produce", "height", j.Tmpl.Height, "job", j.ID)
		case *res == "inconclusive-not-best-prevblk":
			result = "not checked: a new block arrived"
		default:
			result = "REJECTED: " + *res
			m.log.Error("block check FAILED: the node rejects the block this job would produce", "height", j.Tmpl.Height, "job", j.ID, "reason", *res)
		}
		m.st.SetTemplate(func(t *stats.TemplateInfo) {
			t.BlockCheck, t.BlockCheckHeight, t.BlockCheckAt = result, j.Tmpl.Height, time.Now()
		})
	}()
}
