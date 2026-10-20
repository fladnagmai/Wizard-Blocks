package work

import (
	"bytes"
	"encoding/hex"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"

	"github.com/fladnagmai/wizard-blocks/internal/address"
	"github.com/fladnagmai/wizard-blocks/internal/auxpow"
	"github.com/fladnagmai/wizard-blocks/internal/bitcoin"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
)

type fakeAux struct {
	mu     sync.Mutex
	blocks []AuxBlock
	got    []*auxpow.AuxPow
	chains []string
	ch     chan struct{}
}

func (f *fakeAux) Current() []AuxBlock      { f.mu.Lock(); defer f.mu.Unlock(); return f.blocks }
func (f *fakeAux) Changed() <-chan struct{} { return f.ch }
func (f *fakeAux) Submit(b AuxBlock, ap *auxpow.AuxPow, _ string, _ float64, _ bitcoin.Hash) {
	f.mu.Lock()
	f.got = append(f.got, ap)
	f.chains = append(f.chains, b.Chain)
	f.mu.Unlock()
}

// A job built on a real Litecoin MWEB template commits to the aux block in
// its coinbase; a share whose PoW hash meets the aux target becomes an
// AuxPoW that passes Dogecoin's checks, with our coinbase (without witness)
// and the share's header as parent. A share above the aux target, or the
// same solution again, submits nothing.
func TestMergedMiningJobAndProof(t *testing.T) {
	p, _ := ParamsFor(address.LTC)
	_, b := loadLTCBlock(t, "testnet4Block2319633.dat")
	height, _ := bitcoin.DecodeBIP34Height(b.Txs[0].Inputs[0].Script)
	gbt := templateFromBlock(t, b, height, true)
	gbt.MWEB = hex.EncodeToString(b.MWEB)
	tmpl, err := NewTemplate(gbt, p, "test")
	if err != nil {
		t.Fatal(err)
	}
	easy := new(big.Int).Lsh(big.NewInt(1), 255)
	aux := &fakeAux{blocks: []AuxBlock{{Chain: "doge", ChainID: 0x62, Hash: bitcoin.Hash{0xd0, 0x6e}, Target: easy, Height: 5}}, ch: make(chan struct{})}
	m := NewManager(Config{Params: p, Chain: "test", Tag: []byte("/t/"), Extranonce2Size: 8, Aux: aux}, nil,
		stats.New("ltc", "t", ""), slog.New(slog.NewTextHandler(io.Discard, nil)))
	aw, err := newAuxWork(aux.Current(), m.PoW())
	if err != nil {
		t.Fatal(err)
	}
	w := &Work{Tmpl: tmpl, Gen: 1, Aux: aw, m: m, byScript: map[string]*Job{}}
	job, err := w.Job([]byte{0x00, 0x14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, "x")
	if err != nil {
		t.Fatal(err)
	}
	en1, en2 := []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8, 9, 10, 11, 12}
	cb, err := bitcoin.ParseTx(job.CB.Tx(en1, en2), false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cb.Inputs[0].Script, aw.Commitment.Tag()) {
		t.Fatal("coinbase does not carry the merged-mining tag")
	}
	// The assembled parent block is still a valid MWEB block.
	hdr := job.Header(en1, en2, tmpl.CurTime, 7, tmpl.Version)
	if _, err := bitcoin.ParseBlockMWEB(job.Block(&hdr, en1, en2)); err != nil {
		t.Fatal(err)
	}
	c := Candidate{Job: job, Header: hdr, En1: en1, En2: en2, Worker: "w"}
	m.SubmitAux(c, bitcoin.Hash{}) // hash 0 meets any target
	m.SubmitAux(c, bitcoin.Hash{}) // the same aux block again: ignored
	if len(aux.got) != 1 {
		t.Fatalf("%d aux submissions, want 1", len(aux.got))
	}
	ap := aux.got[0]
	if err := ap.Check(aux.blocks[0].Hash, 0x62, true); err != nil {
		t.Fatalf("our AuxPoW fails Dogecoin's check: %v", err)
	}
	hb := hdr.Serialize()
	if ap.Parent != hb || ap.Coinbase.TxID != cb.TxID || !bytes.Equal(ap.Coinbase.Raw, job.CB.Tx(en1, en2)) {
		t.Fatal("AuxPoW parent header or coinbase is not the share's")
	}
	// A share that misses the aux target submits nothing.
	aux.blocks[0].Hash = bitcoin.Hash{0xd0, 0x6f}
	aw2, _ := newAuxWork(aux.Current(), m.PoW())
	aw2.Blocks[0].Target = big.NewInt(1)
	job.Aux = aw2
	m.SubmitAux(c, bitcoin.Hash{0xff})
	if len(aux.got) != 1 {
		t.Fatal("share above the aux target was submitted")
	}
	// Share difficulty is capped at the aux network difficulty.
	if aw.MinDiff <= 0 {
		t.Fatal("MinDiff not set")
	}
}

// When the aux source withdraws its work (the Dogecoin node has no peers,
// e.g. its sync paused), updateAux publishes the same parent template again
// without the merged-mining tag, as a clean job in the same generation, so
// miners stop wasting work on a DOGE block that cannot be submitted; when
// the work returns, the tag comes back.
func TestAuxWithdrawnDropsTag(t *testing.T) {
	p, _ := ParamsFor(address.LTC)
	_, b := loadLTCBlock(t, "testnet4Block2319633.dat")
	height, _ := bitcoin.DecodeBIP34Height(b.Txs[0].Inputs[0].Script)
	gbt := templateFromBlock(t, b, height, true)
	gbt.MWEB = hex.EncodeToString(b.MWEB)
	tmpl, err := NewTemplate(gbt, p, "test")
	if err != nil {
		t.Fatal(err)
	}
	easy := new(big.Int).Lsh(big.NewInt(1), 255)
	block := AuxBlock{Chain: "doge", ChainID: 0x62, Hash: bitcoin.Hash{0xd0, 0x6e}, Target: easy, Height: 5}
	aux := &fakeAux{blocks: []AuxBlock{block}, ch: make(chan struct{})}
	m := NewManager(Config{Params: p, Chain: "test", Tag: []byte("/t/"), Extranonce2Size: 8, Aux: aux}, nil,
		stats.New("ltc", "t", ""), slog.New(slog.NewTextHandler(io.Discard, nil)))
	aw, _ := newAuxWork(aux.Current(), m.PoW())
	m.cur = &Work{Tmpl: tmpl, Gen: 7, Aux: aw, m: m, byScript: map[string]*Job{}}
	var got []*Work
	m.listeners = append(m.listeners, func(w *Work) { got = append(got, w) })
	script := []byte{0x00, 0x14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	hasTag := func(w *Work) bool {
		job, err := w.Job(script, "x")
		if err != nil {
			t.Fatal(err)
		}
		cb, err := bitcoin.ParseTx(job.CB.Tx([]byte{1, 2, 3, 4}, make([]byte, 8)), false)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Contains(cb.Inputs[0].Script, []byte{0xfa, 0xbe, 0x6d, 0x6d})
	}
	if !hasTag(m.cur) {
		t.Fatal("setup: no tag")
	}
	aux.mu.Lock()
	aux.blocks = nil
	aux.mu.Unlock()
	m.updateAux(nil)
	if len(got) != 1 || got[0].Aux != nil || !got[0].Clean || got[0].Gen != 7 || got[0].Tmpl != tmpl {
		t.Fatalf("withdrawn aux work: %+v", got)
	}
	if hasTag(got[0]) {
		t.Fatal("coinbase still carries the merged-mining tag after the aux work was withdrawn")
	}
	m.updateAux(nil) // nothing changed: no new jobs
	if len(got) != 1 {
		t.Fatal("republished with nothing changed")
	}
	aux.mu.Lock()
	aux.blocks = []AuxBlock{block}
	aux.mu.Unlock()
	m.updateAux(nil)
	if len(got) != 2 || got[1].Aux == nil || !hasTag(got[1]) {
		t.Fatal("tag not back when the aux work returned")
	}
}

// Two aux chains on one share (DOGE chain ID 0x62, PEP 0x3f): the chain
// merkle tree has two slots (size 2, nonce 0: DOGE slot 0, PEP slot 1), one
// coinbase tag commits to both, and a share meeting both targets gives two
// AuxPoWs that each pass that chain's own check. A share meeting only the
// easier target solves only that chain.
func TestMergedMiningTwoChains(t *testing.T) {
	p, _ := ParamsFor(address.LTC)
	_, b := loadLTCBlock(t, "testnet4Block2319633.dat")
	height, _ := bitcoin.DecodeBIP34Height(b.Txs[0].Inputs[0].Script)
	gbt := templateFromBlock(t, b, height, true)
	gbt.MWEB = hex.EncodeToString(b.MWEB)
	tmpl, err := NewTemplate(gbt, p, "test")
	if err != nil {
		t.Fatal(err)
	}
	easy := new(big.Int).Lsh(big.NewInt(1), 255)
	harder := new(big.Int).Lsh(big.NewInt(1), 240)
	doge := AuxBlock{Chain: "doge", ChainID: 0x62, Hash: bitcoin.Hash{0xd0, 0x6e}, Target: easy, Height: 5}
	pep := AuxBlock{Chain: "pep", ChainID: 0x3f, Hash: bitcoin.Hash{0x9e, 0x9e}, Target: easy, Height: 9}
	aux := &fakeAux{blocks: []AuxBlock{doge, pep}, ch: make(chan struct{})}
	m := NewManager(Config{Params: p, Chain: "test", Tag: []byte("/t/"), Extranonce2Size: 8, Aux: aux}, nil,
		stats.New("ltc", "t", ""), slog.New(slog.NewTextHandler(io.Discard, nil)))
	aw, err := newAuxWork(aux.Current(), m.PoW())
	if err != nil {
		t.Fatal(err)
	}
	if aw.Commitment.Size != 2 || aw.Commitment.Nonce != 0 {
		t.Fatalf("tree size %d nonce %d, want 2 / 0", aw.Commitment.Size, aw.Commitment.Nonce)
	}
	for _, c := range []struct {
		id   uint32
		slot int32
	}{{0x62, 0}, {0x3f, 1}} {
		if _, idx, ok := aw.Commitment.Branch(c.id); !ok || idx != c.slot || uint32(idx) != auxpow.ExpectedIndex(0, c.id, 1) {
			t.Fatalf("chain %#x in slot %d, want %d", c.id, idx, c.slot)
		}
	}
	w := &Work{Tmpl: tmpl, Gen: 1, Aux: aw, m: m, byScript: map[string]*Job{}}
	job, err := w.Job([]byte{0x00, 0x14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, "x")
	if err != nil {
		t.Fatal(err)
	}
	en1, en2 := []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8, 9, 10, 11, 12}
	hdr := job.Header(en1, en2, tmpl.CurTime, 7, tmpl.Version)
	c := Candidate{Job: job, Header: hdr, En1: en1, En2: en2, Worker: "w"}
	m.SubmitAux(c, bitcoin.Hash{}) // meets both targets
	if len(aux.got) != 2 {
		t.Fatalf("%d aux submissions, want 2 (%v)", len(aux.got), aux.chains)
	}
	for i, ch := range aux.chains {
		blk := map[string]AuxBlock{"doge": doge, "pep": pep}[ch]
		if err := aux.got[i].Check(blk.Hash, blk.ChainID, true); err != nil {
			t.Fatalf("%s AuxPoW fails its check: %v", ch, err)
		}
		// The proof for one chain is no proof for the other.
		other := map[string]AuxBlock{"doge": pep, "pep": doge}[ch]
		if aux.got[i].Check(other.Hash, other.ChainID, true) == nil {
			t.Fatalf("%s proof also passes as the other chain", ch)
		}
	}
	// New aux blocks; PEP now harder: a share between the targets solves DOGE only.
	doge.Hash, pep.Hash, pep.Target = bitcoin.Hash{0xd0, 0x70}, bitcoin.Hash{0x9e, 0xa0}, harder
	aux.mu.Lock()
	aux.blocks, aux.got, aux.chains = []AuxBlock{doge, pep}, nil, nil
	aux.mu.Unlock()
	aw2, _ := newAuxWork(aux.Current(), m.PoW())
	w2 := &Work{Tmpl: tmpl, Gen: 1, Aux: aw2, m: m, byScript: map[string]*Job{}}
	job2, err := w2.Job([]byte{0x00, 0x14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, "x")
	if err != nil {
		t.Fatal(err)
	}
	c = Candidate{Job: job2, Header: job2.Header(en1, en2, tmpl.CurTime, 7, tmpl.Version), En1: en1, En2: en2, Worker: "w"}
	mid := bitcoin.Hash{}
	mid[31] = 0x01 // internal byte order: about 2^248, above PEP's 2^240, below DOGE's 2^255
	m.SubmitAux(c, mid)
	if len(aux.chains) != 1 || aux.chains[0] != "doge" {
		t.Fatalf("submitted %v, want doge only", aux.chains)
	}
	if aw2.MinDiff != m.PoW().Difficulty(easy) {
		t.Fatalf("share difficulty cap %v, want the easier chain's", aw2.MinDiff)
	}
}
