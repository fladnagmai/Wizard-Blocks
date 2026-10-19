package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fladnagmai/wizard-blocks/internal/stratum"
)

// Connection limits changed in Settings are saved to
// <data_dir>/limits-<coin>.json and override the config/env values on the
// next start. Reset deletes the file and restores the config values.

func (e *Engine) limitsPath() string {
	if e.cfg.DataDir == "" {
		return ""
	}
	return filepath.Join(e.cfg.DataDir, "limits-"+e.cfg.Coin+".json")
}

func (e *Engine) baseLimits() stratum.ConnLimits {
	return stratum.ConnLimits{MaxConnections: e.cfg.Stratum.MaxConnections, MaxPerIP: e.cfg.Stratum.MaxConnsPerIP}
}

func (e *Engine) loadSavedLimits() {
	p := e.limitsPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var l stratum.ConnLimits
	if err == nil {
		err = json.Unmarshal(b, &l)
	}
	if err == nil {
		err = e.srv.SetConnLimits(l)
	}
	if err != nil {
		e.log.Warn("ignoring saved connection limits", "path", p, "err", err)
		return
	}
	e.log.Info("using connection limits saved from the UI (they override config/env)", "path", p,
		"max_connections", l.MaxConnections, "max_per_ip", l.MaxPerIP)
}

// ConnLimits implements ui.LimitsBackend: the live limits, the config/env
// defaults, and whether they come from a saved file.
func (e *Engine) ConnLimits() (live, defaults stratum.ConnLimits, saved bool) {
	if p := e.limitsPath(); p != "" {
		_, err := os.Stat(p)
		saved = err == nil
	}
	return e.srv.ConnLimits(), e.baseLimits(), saved
}

// SetConnLimits implements ui.LimitsBackend: validate, apply (to new
// connections) and save.
func (e *Engine) SetConnLimits(l stratum.ConnLimits) error {
	e.settingsMu.Lock()
	defer e.settingsMu.Unlock()
	if err := e.srv.SetConnLimits(l); err != nil {
		return err
	}
	p := e.limitsPath()
	if p == "" {
		return nil // applied, but there is nowhere to persist it
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("applied but not saved: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("applied but not saved: %w", err)
	}
	return nil
}

// ResetConnLimits implements ui.LimitsBackend: back to config/env.
func (e *Engine) ResetConnLimits() error {
	e.settingsMu.Lock()
	defer e.settingsMu.Unlock()
	if p := e.limitsPath(); p != "" {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return e.srv.SetConnLimits(e.baseLimits())
}
