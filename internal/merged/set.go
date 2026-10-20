package merged

import (
	"context"
	"sync"

	"github.com/fladnagmai/wizard-blocks/internal/auxpow"
	"github.com/fladnagmai/wizard-blocks/internal/bitcoin"
	"github.com/fladnagmai/wizard-blocks/internal/work"
)

// Set is the work.AuxSource over several aux chains: one commitment covers
// every chain that has work right now, and each solved aux block goes back
// to its own chain. A chain without work (node down or syncing, no payout
// address) simply drops out; the others and the parent keep being mined.
type Set struct {
	chains  []*Chain
	changed chan struct{}
}

// NewSet combines chains (in a fixed order: the commitment sorts by chain ID).
func NewSet(chains ...*Chain) *Set {
	return &Set{chains: chains, changed: make(chan struct{}, 1)}
}

// Chains returns the chains of the set.
func (s *Set) Chains() []*Chain { return s.chains }

// Chain returns the chain called name, or nil.
func (s *Set) Chain(name string) *Chain {
	for _, c := range s.chains {
		if c.spec.Name == name {
			return c
		}
	}
	return nil
}

// Current implements work.AuxSource.
func (s *Set) Current() []work.AuxBlock {
	var out []work.AuxBlock
	for _, c := range s.chains {
		out = append(out, c.Current()...)
	}
	return out
}

// Changed implements work.AuxSource: signalled when any chain changes.
func (s *Set) Changed() <-chan struct{} { return s.changed }

// Submit implements work.AuxSource: the block goes to its own chain.
func (s *Set) Submit(b work.AuxBlock, ap *auxpow.AuxPow, worker string, shareDiff float64, parentHash bitcoin.Hash) {
	if c := s.Chain(b.Chain); c != nil {
		c.Submit(b, ap, worker, shareDiff, parentHash)
	}
}

// Run runs every chain until ctx ends and forwards their changes.
func (s *Set) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range s.chains {
		wg.Add(2)
		go func(c *Chain) { defer wg.Done(); c.Run(ctx) }(c)
		go func(c *Chain) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-c.Changed():
					select {
					case s.changed <- struct{}{}:
					default:
					}
				}
			}
		}(c)
	}
	wg.Wait()
}
