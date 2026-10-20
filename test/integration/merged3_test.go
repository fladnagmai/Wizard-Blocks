//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/auxpow"
	"github.com/fladnagmai/wizard-blocks/internal/bitcoin"
	"github.com/fladnagmai/wizard-blocks/internal/pow"
)

// pepAuxpowHeight is the first Pepecoin regtest height that must carry an
// AuxPoW (pepecoin chainparams.cpp: auxpowConsensus.nHeightEffective = 20).
const pepAuxpowHeight = 20

// TestRegtestMergedLTCDOGEPEP mines Litecoin with Dogecoin and Pepecoin
// merged on top, one Scrypt miner for all three. Regtest gives the three
// chains the same easy target, so a solving share is a block on every
// chain merged at that moment:
//   - LTC + DOGE (no PEP address yet), then LTC + DOGE + PEP (the PEP
//     address set from Settings), then LTC + PEP (the DOGE address cleared);
//   - the Pepecoin node stopped and started mid-run (LTC + DOGE meanwhile),
//     then the Dogecoin node (LTC + PEP meanwhile), then both (LTC alone).
//
// Every aux block is read back from its node, checked byte for byte and
// with that chain's own AuxPoW rules, and tied to the Litecoin block whose
// header proves it.
func TestRegtestMergedLTCDOGEPEP(t *testing.T) {
	track(t)
	suffix := randHex(3)
	netName := "wbit-m3-" + suffix
	docker(t, "network", "create", netName)
	t.Cleanup(func() { docker(t, "network", "rm", netName) })
	l := startNode(t, "ltc", netName, "wbit-m3-ltc-"+suffix)
	d := startNode(t, "doge", netName, "wbit-m3-doge-"+suffix)
	p := startNode(t, "pep", netName, "wbit-m3-pep-"+suffix)
	var none any
	l.call(t, l.RPC, "createwallet", &none, "w")
	var hs []string
	l.call(t, l.RPC, "generatetoaddress", &hs, 101, l.newAddress(t, "bech32"))
	var dogeMine, dogePayout, pepMine, pepPayout string
	d.call(t, d.RPC, "getnewaddress", &dogeMine)
	d.call(t, d.RPC, "getnewaddress", &dogePayout)
	d.call(t, d.RPC, "generatetoaddress", &hs, dogeAuxpowHeight+5, dogeMine)
	p.call(t, p.RPC, "getnewaddress", &pepMine)
	p.call(t, p.RPC, "getnewaddress", &pepPayout)
	p.call(t, p.RPC, "generatetoaddress", &hs, pepAuxpowHeight+5, pepMine)
	var pni struct {
		Subversion string `json:"subversion"`
	}
	p.call(t, p.RPC, "getnetworkinfo", &pni)
	t.Logf("nodes: litecoin, dogecoin, %s; DOGE payout %s, PEP payout %s", pni.Subversion, dogePayout, pepPayout)
	scripts := map[string]string{"doge": d.scriptOf(t, dogePayout), "pep": p.scriptOf(t, pepPayout)}
	nodes := map[string]*Node{"doge": d, "pep": p}
	ids := map[string]uint32{"doge": 0x62, "pep": 0x3f}

	cfg := engineConfig(l, "ltc")
	cfg.Payout.Mode, cfg.Payout.Address = "fixed", l.newAddress(t, "bech32")
	cfg.Doge.RPCURL, cfg.Doge.RPCUser, cfg.Doge.RPCPassword, cfg.Doge.ZMQHashBlock = d.RPCURL, rpcUser, rpcPass, d.ZMQ
	cfg.Doge.PollIntervalMs, cfg.Doge.RefreshS, cfg.Doge.PayoutAddress = 500, 2, dogePayout
	cfg.Pep.RPCURL, cfg.Pep.RPCUser, cfg.Pep.RPCPassword, cfg.Pep.ZMQHashBlock = p.RPCURL, rpcUser, rpcPass, p.ZMQ
	cfg.Pep.PollIntervalMs, cfg.Pep.RefreshS = 500, 2 // no PEP address yet: set from Settings below
	cfg.UI.SettingsOpen = true
	cfg.DataDir = t.TempDir()
	en := startEngine(t, cfg, "merged3")
	rec := &recorder{m: map[string]submission{}}
	base := "http://" + en.E.UIAddr()

	merging := func() string {
		w := en.E.Manager().Current()
		if w == nil || w.Aux == nil {
			return ""
		}
		var names []string
		for _, b := range w.Aux.Blocks {
			names = append(names, b.Chain)
		}
		sort.Strings(names)
		return strings.Join(names, "+")
	}
	waitMerging := func(want string) {
		t.Helper()
		waitFor(t, "merging "+want, 60*time.Second, func() bool { return merging() == want })
	}
	chainView := func(name string) map[string]any {
		t.Helper()
		r, err := http.Get(base + "/api/state")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		ch, _ := m["chains"].(map[string]any)
		v, _ := ch[name].(map[string]any)
		return v
	}
	waitReason := func(name, want string) {
		t.Helper()
		waitFor(t, name+" reason "+want, 60*time.Second, func() bool {
			v := chainView(name)
			if want == "" {
				return v != nil && v["merged"] == true
			}
			return v != nil && v["merged"] == false && v["reason"] == want
		})
	}
	put := func(path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", base)
		req.Header.Set("X-WB-Admin", "1")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	auxOf := func(chain, status string) int {
		n := 0
		for _, b := range auxBlocks(en, status) {
			if b.Chain == chain {
				n++
			}
		}
		return n
	}
	settle := func(what string) {
		t.Helper()
		waitFor(t, what+": aux blocks resolved", 90*time.Second, func() bool { return len(auxBlocks(en, "pending")) == 0 })
	}
	// mine mines n LTC blocks while want is merged and checks that every
	// chain in want gained blocks.
	mine := func(want, worker string, n int) {
		t.Helper()
		waitMerging(want)
		before := map[string]int{"doge": auxOf("doge", "accepted"), "pep": auxOf("pep", "accepted")}
		mineLTC(t, en, l, rec, worker, n)
		settle(worker)
		for _, c := range strings.Split(want, "+") {
			if c != "" && auxOf(c, "accepted") <= before[c] {
				t.Fatalf("%s: no %s block while merging %s", worker, c, want)
			}
		}
		for _, c := range []string{"doge", "pep"} {
			if !strings.Contains(want, c) && auxOf(c, "accepted") != before[c] {
				t.Fatalf("%s: %s blocks while %s was not merged", worker, c, c)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	nodeBack := func(n *Node) {
		t.Helper()
		docker(t, "start", n.Name)
		waitFor(t, n.Name+" RPC back", 60*time.Second, func() bool { _, err := n.RPC.GetBlockchainInfo(ctx); return err == nil })
	}

	// 1. LTC + DOGE: no PEP address yet, the page says why.
	waitReason("pep", "no address")
	mine("doge", "rig-ltc-doge", 3)
	// 2. PEP address from Settings (an LTC address is refused; regtest DOGE
	// and PEP addresses share version bytes, mainnet ones do not): all three.
	if code, body := put("/api/admin/aux-payout", `{"chain":"pep","address":"`+cfg.Payout.Address+`"}`); code != http.StatusBadRequest {
		t.Fatalf("an LTC address accepted as PEP payout: %d %s", code, body)
	}
	if code, body := put("/api/admin/aux-payout", `{"chain":"pep","address":"`+pepPayout+`"}`); code != http.StatusOK {
		t.Fatalf("set PEP payout: %d %s", code, body)
	}
	waitReason("pep", "")
	mine("doge+pep", "rig-three", 6)
	// 3. DOGE address cleared: LTC + PEP.
	if code, body := put("/api/admin/doge-payout", `{"address":""}`); code != http.StatusOK {
		t.Fatalf("clear DOGE payout: %d %s", code, body)
	}
	waitReason("doge", "no address")
	mine("pep", "rig-ltc-pep", 3)
	if code, body := put("/api/admin/doge-payout", `{"address":"`+dogePayout+`"}`); code != http.StatusOK {
		t.Fatalf("restore DOGE payout: %d %s", code, body)
	}
	mine("doge+pep", "rig-three2", 2)
	// 4. Pepecoin node down mid-run: LTC + DOGE carry on; PEP comes back.
	docker(t, "stop", "-t", "5", p.Name)
	waitReason("pep", "node down")
	mine("doge", "rig-pep-down", 3)
	nodeBack(p)
	waitReason("pep", "")
	mine("doge+pep", "rig-pep-back", 3)
	// 5. Dogecoin node down: LTC + PEP carry on; DOGE comes back.
	docker(t, "stop", "-t", "5", d.Name)
	waitReason("doge", "node down")
	mine("pep", "rig-doge-down", 3)
	nodeBack(d)
	waitReason("doge", "")
	// 6. Both aux nodes down: LTC alone, untagged; both come back.
	docker(t, "stop", "-t", "5", p.Name)
	docker(t, "stop", "-t", "5", d.Name)
	waitFor(t, "no aux work", 60*time.Second, func() bool { return merging() == "" })
	soloFrom := len(en.E.Stats().Blocks())
	mine("", "rig-ltc-alone", 3)
	soloBlocks := en.E.Stats().Blocks()[soloFrom:]
	nodeBack(p)
	nodeBack(d)
	mine("doge+pep", "rig-all-back", 3)
	pageCounts := map[string]any{"doge": chainView("doge")["blocks_found"], "pep": chainView("pep")["blocks_found"]}
	en.Stop(t)

	// ---- verify ----
	ltcAccepted := map[string]bool{}
	for _, b := range en.E.Stats().Blocks() {
		if b.Status != "accepted" {
			t.Errorf("LTC block %s at %d: %s (%s)", b.Hash, b.Height, b.Status, b.Reason)
			continue
		}
		var best string
		l.call(t, l.RPC, "getblockhash", &best, b.Height)
		if best != b.Hash {
			t.Fatalf("LTC block %d not on the active chain", b.Height)
		}
		ltcAccepted[b.Hash] = true
	}
	for _, b := range soloBlocks {
		var raw string
		l.call(t, l.RPC, "getblock", &raw, b.Hash, 0)
		rb, _ := hex.DecodeString(raw)
		blk, err := bitcoin.ParseBlockMWEB(rb)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(blk.Txs[0].Inputs[0].Script, auxpow.MergedMiningHeader) {
			t.Fatalf("LTC block %d mined with both aux nodes down carries a merged-mining tag", b.Height)
		}
	}
	parents := map[string][]string{} // LTC block -> aux chains it proved
	count := map[string]int{}
	for _, r := range en.E.Stats().AuxBlocks() {
		if r.Status != "accepted" {
			t.Errorf("%s block %s at %d: %s (%s)", r.Chain, r.Hash, r.Height, r.Status, r.Reason)
			continue
		}
		n := nodes[r.Chain]
		var raw string
		n.call(t, n.RPC, "getblock", &raw, r.Hash, false)
		rb, _ := hex.DecodeString(raw)
		blk, err := auxpow.ParseBlock(rb)
		if err != nil {
			t.Fatalf("%s block %d: %v", r.Chain, r.Height, err)
		}
		if !bytes.Equal(blk.Serialize(), rb) || blk.Header.Hash().String() != r.Hash || blk.AuxPow == nil {
			t.Fatalf("%s block %d: not byte for byte, wrong hash or no AuxPoW", r.Chain, r.Height)
		}
		if got := auxpow.ChainID(blk.Header.Version); got != ids[r.Chain] {
			t.Fatalf("%s block %d: chain ID %#x in its version", r.Chain, r.Height, got)
		}
		if err := blk.AuxPow.Check(blk.Header.Hash(), ids[r.Chain], true); err != nil {
			t.Fatalf("%s block %d: AuxPoW check: %v", r.Chain, r.Height, err)
		}
		tgt, _ := bitcoin.CompactToTarget(blk.Header.Bits)
		if !bitcoin.HashMeetsTarget(pow.Scrypt.PoWHash(blk.AuxPow.Parent[:]), tgt) {
			t.Fatalf("%s block %d: parent Scrypt work misses the target", r.Chain, r.Height)
		}
		if hex.EncodeToString(blk.Txs[0].Outputs[0].Script) != scripts[r.Chain] {
			t.Fatalf("%s block %d does not pay the %s payout address", r.Chain, r.Height, r.Chain)
		}
		ph, _ := blk.AuxPow.ParentHeader()
		if ph.Hash().String() != r.ParentHash {
			t.Fatalf("%s block %d: parent %s, recorded %s", r.Chain, r.Height, ph.Hash(), r.ParentHash)
		}
		if ltcAccepted[r.ParentHash] {
			var lraw string
			l.call(t, l.RPC, "getblock", &lraw, r.ParentHash, 0)
			lb, _ := hex.DecodeString(lraw)
			lblk, err := bitcoin.ParseBlockMWEB(lb)
			if err != nil {
				t.Fatal(err)
			}
			if lblk.Txs[0].TxID != blk.AuxPow.Coinbase.TxID {
				t.Fatalf("%s block %d: AuxPoW coinbase is not LTC block %s's coinbase", r.Chain, r.Height, r.ParentHash)
			}
			parents[r.ParentHash] = append(parents[r.ParentHash], r.Chain)
		}
		count[r.Chain]++
	}
	three, ltcDoge, ltcPep := 0, 0, 0
	for _, chains := range parents {
		sort.Strings(chains)
		switch strings.Join(chains, "+") {
		case "doge+pep":
			three++
		case "doge":
			ltcDoge++
		case "pep":
			ltcPep++
		}
	}
	for c, v := range pageCounts {
		if n, ok := v.(float64); !ok || int(n) != count[c] {
			t.Fatalf("page %s blocks_found %v, node has %d", c, v, count[c])
		}
	}
	blocksVerified.Add(int64(len(ltcAccepted) + count["doge"] + count["pep"]))
	t.Logf("merged3: VERIFIED %d LTC, %d DOGE and %d PEP blocks; one share was a block on LTC+DOGE+PEP %d times, LTC+DOGE %d, LTC+PEP %d; %d LTC blocks with both aux nodes down",
		len(ltcAccepted), count["doge"], count["pep"], three, ltcDoge, ltcPep, len(soloBlocks))
	if three < 5 || ltcDoge < 3 || ltcPep < 3 || len(soloBlocks) < 3 {
		t.Fatal("not every case was covered")
	}
}
