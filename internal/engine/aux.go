package engine

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/address"
	"github.com/fladnagmai/wizard-blocks/internal/config"
	"github.com/fladnagmai/wizard-blocks/internal/merged"
	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/ui"
)

// Merged mining runs for every aux chain whose node is configured
// (Dogecoin, Pepecoin). Each chain's payout address comes from
// <data_dir>/payout-<chain>.json (set from the web UI, wins) or the config;
// without one, that chain is not merged and the page says so. A node that
// is down at startup is not fatal: its chain keeps retrying and the others
// (and Litecoin) are mined meanwhile.

// auxChain pairs an aux chain with its config and address coin.
type auxChain struct {
	spec merged.Spec
	cfg  config.AuxNode
	coin address.Coin
}

func (e *Engine) auxChains() []auxChain {
	return []auxChain{
		{merged.DogeSpec, e.cfg.Doge, address.DOGE},
		{merged.PepSpec, e.cfg.Pep, address.PEP},
	}
}

func (e *Engine) auxPayoutPath(chain string) string {
	if e.cfg.DataDir == "" {
		return ""
	}
	return filepath.Join(e.cfg.DataDir, "payout-"+chain+".json")
}

// validateAux checks an aux chain address locally (for the Litecoin node's
// network) and, when the node answers or requireNode is set, with the aux
// node (same scriptPubKey).
func (e *Engine) validateAux(ctx context.Context, a auxChain, rpc *node.Client, addr string, requireNode bool) (string, error) {
	addr = strings.TrimSpace(addr)
	t := a.spec.Ticker
	net, err := address.NetworkFor(a.coin, e.net.Chain)
	if err != nil {
		return "", err
	}
	dec, err := address.Decode(net, addr)
	if err != nil {
		return "", fmt.Errorf("invalid %s %s address %q: %w", t, e.net.Chain, addr, err)
	}
	v, err := rpc.ValidateAddress(ctx, addr)
	if err != nil {
		if requireNode {
			return "", fmt.Errorf("cannot verify %s address with the %s node: %w", t, a.spec.NodeName, err)
		}
		return addr, nil
	}
	if !v.IsValid || !strings.EqualFold(v.ScriptPubKey, hex.EncodeToString(dec.Script)) {
		return "", fmt.Errorf("%s node does not confirm %s address %q", a.spec.NodeName, t, addr)
	}
	return addr, nil
}

// initAux connects to an aux node (if up, it must be that coin's node on
// the same network) and picks its payout address.
func (e *Engine) initAux(ctx context.Context, a auxChain) (*merged.Chain, error) {
	d, name, t := a.cfg, a.spec.Name, a.spec.Ticker
	rpc := node.NewClient(d.RPCURL, d.RPCUser, d.RPCPassword, d.RPCCookieFile, time.Duration(e.cfg.Node.RPCTimeoutS)*time.Second)
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ci, err := rpc.GetBlockchainInfo(cctx)
	var ni *node.NetworkInfo
	if err == nil {
		ni, err = rpc.GetNetworkInfo(cctx)
	}
	if err == nil {
		if !strings.Contains(strings.ToLower(ni.Subversion), a.spec.Subversion) {
			return nil, fmt.Errorf("%s.rpc_url points at %q, not a %s node", name, ni.Subversion, a.spec.NodeName)
		}
		if ci.Chain != e.net.Chain {
			return nil, fmt.Errorf("%s node is on %q but the Litecoin node is on %q", a.spec.NodeName, ci.Chain, e.net.Chain)
		}
		e.log.Info("connected to "+a.spec.NodeName+" node", "subversion", ni.Subversion, "chain", ci.Chain, "height", ci.Blocks)
	} else {
		e.log.Warn(a.spec.NodeName+" node not reachable: merged mining continues without it until it is", "url", d.RPCURL, "err", err)
	}

	addr := ""
	if p := e.auxPayoutPath(name); p != "" {
		var saved struct {
			Address string `json:"address"`
		}
		if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &saved) == nil && saved.Address != "" {
			if v, err := e.validateAux(cctx, a, rpc, saved.Address, false); err == nil {
				addr = v
				e.log.Info(t+" payout address (saved from the UI)", "address", addr)
			} else {
				e.log.Error("ignoring saved "+t+" payout address", "path", p, "err", err)
			}
		}
	}
	if addr == "" && strings.TrimSpace(d.PayoutAddress) != "" {
		v, err := e.validateAux(cctx, a, rpc, d.PayoutAddress, false)
		if err != nil {
			return nil, fmt.Errorf("%s.payout_address: %w", name, err)
		}
		addr = v
	}
	if addr == "" {
		e.log.Warn("no " + t + " payout address set: not merging " + a.spec.NodeName + " until one is set (web UI Settings)")
	}
	return merged.NewChain(a.spec, merged.ChainConfig{
		PayoutAddr: addr, ZMQEndpoint: d.ZMQHashBlock,
		Poll:    time.Duration(d.PollIntervalMs) * time.Millisecond,
		Refresh: time.Duration(d.RefreshS) * time.Second,
		DataDir: e.cfg.DataDir,
	}, rpc, e.st, e.log), nil
}

// initAuxSet starts every configured aux chain (nil when there are none).
func (e *Engine) initAuxSet(ctx context.Context) (*merged.Set, error) {
	var chains []*merged.Chain
	for _, a := range e.auxChains() {
		if !a.cfg.Enabled() {
			continue
		}
		c, err := e.initAux(ctx, a)
		if err != nil {
			return nil, err
		}
		chains = append(chains, c)
	}
	if len(chains) == 0 {
		return nil, nil
	}
	return merged.NewSet(chains...), nil
}

func (e *Engine) auxChain(name string) (auxChain, *merged.Chain, error) {
	for _, a := range e.auxChains() {
		if a.spec.Name != name {
			continue
		}
		if e.aux == nil || e.aux.Chain(name) == nil {
			return a, nil, fmt.Errorf("%s merged mining is not configured (no %s node)", a.spec.NodeName, a.spec.NodeName)
		}
		return a, e.aux.Chain(name), nil
	}
	return auxChain{}, nil, fmt.Errorf("unknown aux chain %q", name)
}

// AuxPayout implements ui.AuxBackend: the chain's payout address ("" if
// none) and whether it can be merged at all (its node configured).
func (e *Engine) AuxPayout(chain string) (string, bool) {
	_, c, err := e.auxChain(chain)
	if err != nil {
		return "", false
	}
	return c.PayoutAddress(), true
}

// SetAuxPayout implements ui.AuxBackend: a new address must be confirmed
// by the chain's node; "" stops merging it. Saved and applied at once
// (miners get fresh jobs with the new commitment).
func (e *Engine) SetAuxPayout(ctx context.Context, chain, addr string) (string, error) {
	a, c, err := e.auxChain(chain)
	if err != nil {
		return "", err
	}
	t := a.spec.Ticker
	addr = strings.TrimSpace(addr)
	if addr != "" {
		if addr, err = e.validateAux(ctx, a, c.RPC(), addr, true); err != nil {
			return "", err
		}
	}
	if p := e.auxPayoutPath(chain); p != "" {
		b, _ := json.MarshalIndent(map[string]string{"address": addr}, "", "  ")
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return "", fmt.Errorf("cannot save %s payout address: %w", t, err)
		}
		if err := os.Rename(tmp, p); err != nil {
			return "", fmt.Errorf("cannot save %s payout address: %w", t, err)
		}
	}
	c.SetPayoutAddress(addr)
	if addr == "" {
		e.log.Info(t + " payout address cleared from the UI: not merging " + a.spec.NodeName)
	} else {
		e.log.Info(t+" payout address set from the UI", "address", addr)
	}
	return addr, nil
}

// AuxChainNames implements ui.AuxBackend: the configured aux chains.
func (e *Engine) AuxChainNames() []string {
	if e.aux == nil {
		return nil
	}
	var out []string
	for _, c := range e.aux.Chains() {
		out = append(out, c.Spec().Name)
	}
	return out
}

var _ ui.AuxBackend = (*Engine)(nil)
