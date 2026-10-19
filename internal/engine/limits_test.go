package engine

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/address"
	"github.com/fladnagmai/wizard-blocks/internal/config"
	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
	"github.com/fladnagmai/wizard-blocks/internal/stratum"
	"github.com/fladnagmai/wizard-blocks/internal/work"
)

// limitsEngine is an engine with only what the limits code touches.
func limitsEngine(t *testing.T, dir string) *Engine {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{Coin: "ltc", DataDir: dir}
	cfg.Stratum.MaxConnections, cfg.Stratum.MaxConnsPerIP = 1024, 64
	st := stats.New("ltc", "t", "")
	p, _ := work.ParamsFor(address.BTC)
	mgr := work.NewManager(work.Config{Params: p, Chain: "regtest", Extranonce2Size: 8, PollInterval: time.Second, RefreshInterval: time.Second},
		node.NewClient("http://127.0.0.1:1", "u", "p", "", time.Second), st, log)
	e := &Engine{cfg: cfg, log: log}
	e.srv = stratum.NewServer(stratum.Config{MaxConnections: 1024, MaxConnsPerIP: 64}, mgr, nil, st, log)
	e.loadSavedLimits()
	return e
}

func TestConnLimitsSavedAndReset(t *testing.T) {
	dir := t.TempDir()
	e := limitsEngine(t, dir)
	live, def, saved := e.ConnLimits()
	if live != (stratum.ConnLimits{MaxConnections: 1024, MaxPerIP: 64}) || def != live || saved {
		t.Fatalf("defaults: live %+v def %+v saved %v", live, def, saved)
	}
	if err := e.SetConnLimits(stratum.ConnLimits{MaxConnections: 10, MaxPerIP: 20}); err == nil {
		t.Fatal("per-IP above total accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "limits-ltc.json")); !os.IsNotExist(err) {
		t.Fatal("invalid limits were saved")
	}
	want := stratum.ConnLimits{MaxConnections: 5000, MaxPerIP: 500}
	if err := e.SetConnLimits(want); err != nil {
		t.Fatal(err)
	}
	if live, _, saved := e.ConnLimits(); live != want || !saved {
		t.Fatalf("after set: %+v saved %v", live, saved)
	}
	// A restart picks the saved limits up.
	if live, def, saved := limitsEngine(t, dir).ConnLimits(); live != want || def.MaxConnections != 1024 || !saved {
		t.Fatalf("after restart: live %+v def %+v saved %v", live, def, saved)
	}
	if err := e.ResetConnLimits(); err != nil {
		t.Fatal(err)
	}
	if live, _, saved := e.ConnLimits(); live.MaxConnections != 1024 || live.MaxPerIP != 64 || saved {
		t.Fatalf("after reset: %+v saved %v", live, saved)
	}
	if live, _, _ := limitsEngine(t, dir).ConnLimits(); live.MaxConnections != 1024 {
		t.Fatalf("reset did not survive a restart: %+v", live)
	}
}

// A corrupt or out-of-range file is ignored: the config defaults stay.
func TestConnLimitsBadFileIgnored(t *testing.T) {
	for _, body := range []string{"not json", `{"max_connections":0,"max_connections_per_ip":64}`, `{"max_connections":10,"max_connections_per_ip":64}`} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "limits-ltc.json"), []byte(body), 0o600)
		if live, _, _ := limitsEngine(t, dir).ConnLimits(); live.MaxConnections != 1024 || live.MaxPerIP != 64 {
			t.Errorf("%s: %+v", body, live)
		}
	}
}
