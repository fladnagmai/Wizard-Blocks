package nodestatus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Pruning: each node reads its prune target (MiB) from a small config file
// (-conf=/conf/<coin>.conf) that only this service writes. Changing it
// rewrites the file and stops the node over RPC; its container restarts
// (restart: unless-stopped) with the new target. Pruning cannot be turned
// off here: going back to a full node needs the whole chain downloaded again.

// PruneConfig is a node's pruning setup.
type PruneConfig struct {
	ConfFile string // e.g. /conf/litecoin.conf
	Default  int    // MiB, written when the file is missing or has no valid prune line
	Min      int    // the node's minimum (Litecoin 550, Dogecoin 2200)
	Options  []int  // MiB choices offered on the page
}

// ErrBadPrune is returned for a target that is not one of the options.
var ErrBadPrune = errors.New("not one of the prune options")

// CacheConfig is a node's database cache (dbcache, MiB) setup. It is kept
// in the same conf file as the prune target: a bigger cache speeds up the
// first sync, and can be set back once the node is synced.
type CacheConfig struct {
	Default int   // MiB, written when the file has no dbcache line
	Options []int // MiB choices offered on the page
}

// ErrBadCache is returned for a cache size that is not one of the options.
var ErrBadCache = errors.New("not one of the cache options")

// ErrNoCache is returned for a node without a cache config.
var ErrNoCache = errors.New("this node's cache is not set here")

// readConf returns the prune target and dbcache in the conf file (0 if
// none).
func readConf(path string) (prune, dbcache int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "prune":
			prune = n
		case "dbcache":
			dbcache = n
		}
	}
	return prune, dbcache, sc.Err()
}

// readPrune returns the prune target in the conf file (0 if none).
func readPrune(path string) (int, error) {
	v, _, err := readConf(path)
	return v, err
}

// writePrune sets the prune target, keeping the file's dbcache line.
func writePrune(path string, mib int) error {
	_, cache, _ := readConf(path)
	return writeConf(path, mib, cache)
}

// writeConf replaces the conf file atomically (dbcache 0: no line).
func writeConf(path string, prune, dbcache int) error {
	body := fmt.Sprintf("# Written by the Litecoin + Dogecoin Node page: change it there.\n# Prune target and database cache in MiB (the node restarts to apply them).\nprune=%d\n", prune)
	if dbcache > 0 {
		body += fmt.Sprintf("dbcache=%d\n", dbcache)
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EnsurePruneConf makes sure every node's conf file has a valid prune line
// (a node without one runs unpruned and fills the disk) that matches the
// last choice made on the page. The app ships no conf files (an app update
// would overwrite them); they are created here, before the nodes start.
// Nodes whose file it rewrote are restarted once they answer RPC unless
// they already run that target. It returns their keys.
func (p *Poller) EnsurePruneConf() ([]string, error) {
	var fixed []string
	var errs []error
	for _, n := range p.nodes {
		pc := n.Prune
		if pc == nil {
			continue
		}
		p.mu.Lock()
		saved, savedCache := p.prune[n.Key], p.cache[n.Key]
		p.mu.Unlock()
		v, c, err := readConf(pc.ConfFile)
		pruneOK := err == nil && v >= pc.Min && (saved == 0 || v == saved)
		cacheOK := n.Cache == nil || (savedCache == 0 && slices.Contains(n.Cache.Options, c)) || (savedCache > 0 && c == savedCache)
		if pruneOK && cacheOK {
			continue
		}
		want := pc.Default
		switch {
		case pruneOK:
			want = v // only the cache needs fixing
		case saved >= pc.Min:
			want = saved // the last choice made on the page
		}
		wantCache := c
		if n.Cache != nil {
			wantCache = n.Cache.Default
			if savedCache > 0 {
				wantCache = savedCache
			}
		}
		if werr := writeConf(pc.ConfFile, want, wantCache); werr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n.Name, werr))
			continue
		}
		fixed = append(fixed, n.Key)
		p.mu.Lock()
		p.restart[n.Key] = true
		// A changed cache cannot be read back from the node: restart it,
		// unless the line was only missing (the node has not read it yet:
		// an app update brought the file without one, before the nodes
		// started).
		if c > 0 && c != wantCache {
			p.restartCache[n.Key] = true
		}
		p.mu.Unlock()
	}
	return fixed, errors.Join(errs...)
}

// SetPrune sets a node's prune target and restarts the node to apply it.
func (p *Poller) SetPrune(ctx context.Context, key string, mib int) error {
	n := p.node(key)
	if n == nil {
		return ErrUnknownNode
	}
	if n.Prune == nil {
		return ErrNotPrunable
	}
	if !slices.Contains(n.Prune.Options, mib) || mib < n.Prune.Min {
		return fmt.Errorf("%w: %d MiB (choose one of %v)", ErrBadPrune, mib, n.Prune.Options)
	}
	if err := writePrune(n.Prune.ConfFile, mib); err != nil {
		return fmt.Errorf("cannot save the prune target: %w", err)
	}
	p.mu.Lock()
	p.prune[key] = mib
	err := p.saveLocked()
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("cannot save the prune target: %w", err)
	}
	if err := n.RPC.Call(ctx, "stop", nil, nil); err != nil {
		// Warming up or briefly unreachable: restart it once it answers.
		p.mu.Lock()
		p.restart[key] = true
		p.mu.Unlock()
	}
	return nil
}

// ErrNotPrunable is returned for a node without a prune config.
var ErrNotPrunable = errors.New("this node's pruning is not set here")

// SetCache sets a node's database cache and restarts the node to apply it.
func (p *Poller) SetCache(ctx context.Context, key string, mib int) error {
	n := p.node(key)
	if n == nil {
		return ErrUnknownNode
	}
	if n.Cache == nil || n.Prune == nil {
		return ErrNoCache
	}
	if !slices.Contains(n.Cache.Options, mib) {
		return fmt.Errorf("%w: %d MiB (choose one of %v)", ErrBadCache, mib, n.Cache.Options)
	}
	prune, _, err := readConf(n.Prune.ConfFile)
	if err != nil || prune < n.Prune.Min {
		prune = n.Prune.Default
		p.mu.Lock()
		if v := p.prune[key]; v >= n.Prune.Min {
			prune = v
		}
		p.mu.Unlock()
	}
	if err := writeConf(n.Prune.ConfFile, prune, mib); err != nil {
		return fmt.Errorf("cannot save the cache size: %w", err)
	}
	p.mu.Lock()
	p.cache[key] = mib
	err = p.saveLocked()
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("cannot save the cache size: %w", err)
	}
	if err := n.RPC.Call(ctx, "stop", nil, nil); err != nil {
		// Warming up or briefly unreachable: restart it once it answers.
		p.mu.Lock()
		p.restartCache[key] = true
		p.mu.Unlock()
	}
	return nil
}

// ConfReady reports whether path holds a valid prune target (at least min)
// and, if needCache, a dbcache line. The web container's healthcheck uses
// it so the nodes, which wait for it, never start without them.
func ConfReady(path string, min int, needCache bool) bool {
	v, c, err := readConf(path)
	return err == nil && v >= min && (!needCache || c > 0)
}
