// Command poolview serves the public, view-only pages of the Wizard-Blocks
// pools (the Wizard-Blocks View Umbrel app). Configured by env:
//
//	PV_LISTEN   listen address (default 0.0.0.0:8431)
//	PV_BCH_URL  the BCH pool's web UI inside Umbrel, e.g.
//	            http://fladnagmai-wizard-blocks_engine_1:8420 (empty: no tab)
//	PV_LTC_URL  the LTC pool's (fladnagmai-wizard-blocks-ltc_engine_1:8420)
//	PV_BTC_URL  the BTC pool's (fladnagmai-wizard-blocks-btc_engine_1:8420)
//
// It only reads each pool's state and never offers settings.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/poolview"
	"github.com/fladnagmai/wizard-blocks/internal/ui"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	listen := os.Getenv("PV_LISTEN")
	if listen == "" {
		listen = "0.0.0.0:8431"
	}
	var pools []poolview.Pool
	for _, p := range []poolview.Pool{{Key: "bch", Label: "BCH"}, {Key: "ltc", Label: "LTC"}, {Key: "btc", Label: "BTC"}} {
		if p.URL = os.Getenv("PV_" + map[string]string{"bch": "BCH", "ltc": "LTC", "btc": "BTC"}[p.Key] + "_URL"); p.URL != "" {
			pools = append(pools, p)
			log.Info("pool", "key", p.Key, "url", p.URL)
		}
	}
	if len(pools) == 0 {
		log.Error("set PV_BCH_URL, PV_LTC_URL and/or PV_BTC_URL")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: listen, Handler: poolview.New(pools, ui.SimpleFiles(), ui.StaticFiles(), log).Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Info("pool view", "listen", listen, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http", "err", err)
		os.Exit(1)
	}
}
