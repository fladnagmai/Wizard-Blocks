// Command nodestatus serves one page with the live status of the Litecoin,
// Dogecoin and Pepecoin nodes (the page of the Litecoin + Dogecoin Node
// Umbrel app).
// Configured by env:
//
//	NS_LISTEN       listen address (default 0.0.0.0:8080)
//	NS_POLL_S       seconds between checks (default 5)
//	NS_PAUSABLE     comma-separated node keys whose sync can be paused from
//	                the page (e.g. "doge,pep")
//	NS_MEM_DIR      where the nodes' containers write their memory use
//	                (one file per node key, nodes/memwrap.sh)
//	NS_STATE_FILE   where pauses and prune choices are kept (default: memory only)
//	LTC_CONF, DOGE_CONF, PEP_CONF  the nodes' -conf files holding their prune target
//	                and database cache; set to let the page change them
//	                (the node restarts)
//	LTC_RPC_URL, LTC_RPC_USER, LTC_RPC_PASS     Litecoin node (optional)
//	DOGE_RPC_URL, DOGE_RPC_USER, DOGE_RPC_PASS  Dogecoin node (optional)
//	PEP_RPC_URL, PEP_RPC_USER, PEP_RPC_PASS     Pepecoin node (optional)
//
// "nodestatus healthcheck" exits 0 once every *_CONF file has a prune
// target and a cache size.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/nodestatus"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	listen := env("NS_LISTEN", "0.0.0.0:8080")
	poll, err := strconv.Atoi(env("NS_POLL_S", "5"))
	if err != nil || poll < 1 || poll > 3600 {
		fmt.Fprintln(os.Stderr, "NS_POLL_S must be 1..3600")
		os.Exit(2)
	}
	pausable := map[string]bool{}
	for _, k := range strings.Split(os.Getenv("NS_PAUSABLE"), ",") {
		if k = strings.TrimSpace(strings.ToLower(k)); k != "" {
			pausable[k] = true
		}
	}
	var nodes []nodestatus.Node
	// Prune targets (MiB): the nodes' minimums are 550 (Litecoin Core),
	// 2200 (Dogecoin Core) and 2200 (Pepecoin Core, a Dogecoin fork); the
	// defaults are the app's shipped settings.
	prunes := map[string]nodestatus.PruneConfig{
		"ltc":  {Default: 2000, Min: 550, Options: []int{550, 2000, 5000, 10000, 20000}},
		"doge": {Default: 2200, Min: 2200, Options: []int{2200, 5000, 10000, 20000}},
		"pep":  {Default: 2200, Min: 2200, Options: []int{2200, 5000, 10000, 20000}},
	}
	// Database cache (MiB): a bigger one speeds up the first sync; set it
	// back once synced. The defaults are the app's shipped settings.
	caches := map[string]nodestatus.CacheConfig{
		"ltc":  {Default: 300, Options: []int{300, 450, 1000, 2000, 4000}},
		"doge": {Default: 200, Options: []int{200, 450, 1000, 2000, 4000}},
		"pep":  {Default: 100, Options: []int{100, 200, 450, 1000, 2000}},
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		// Healthy once every node's conf file has a prune target (written
		// by EnsurePruneConf at start); the nodes wait for this.
		for k, prefix := range map[string]string{"ltc": "LTC", "doge": "DOGE", "pep": "PEP"} {
			if conf := os.Getenv(prefix + "_CONF"); conf != "" && !nodestatus.ConfReady(conf, prunes[k].Min, true) {
				fmt.Fprintln(os.Stderr, "no prune target or cache size yet in", conf)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	memDir := os.Getenv("NS_MEM_DIR")
	for _, n := range []struct{ key, name, prefix string }{{"ltc", "Litecoin Node", "LTC"}, {"doge", "Dogecoin Node", "DOGE"}, {"pep", "Pepecoin Node", "PEP"}} {
		url := os.Getenv(n.prefix + "_RPC_URL")
		if url == "" {
			continue
		}
		nd := nodestatus.Node{Key: n.key, Name: n.name, Pausable: pausable[n.key],
			RPC: node.NewClient(url, os.Getenv(n.prefix+"_RPC_USER"), os.Getenv(n.prefix+"_RPC_PASS"), "", 10*time.Second)}
		if memDir != "" {
			nd.MemFile = filepath.Join(memDir, n.key)
		}
		if conf := os.Getenv(n.prefix + "_CONF"); conf != "" {
			pc := prunes[n.key]
			pc.ConfFile = conf
			nd.Prune = &pc
			cc := caches[n.key]
			nd.Cache = &cc
		}
		nodes = append(nodes, nd)
		log.Info("watching node", "name", n.name, "url", url, "pausable", nd.Pausable, "prune_conf", os.Getenv(n.prefix+"_CONF"))
	}
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, "set LTC_RPC_URL, DOGE_RPC_URL and/or PEP_RPC_URL")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p := nodestatus.NewPoller(nodes, time.Duration(poll)*time.Second, os.Getenv("NS_STATE_FILE"))
	if fixed, err := p.EnsurePruneConf(); err != nil {
		log.Error("prune config", "err", err)
	} else if len(fixed) > 0 {
		log.Warn("wrote missing or reset prune config; a node running another target is restarted once it answers", "nodes", fixed)
	}
	go p.Run(ctx)
	srv := &http.Server{Addr: listen, Handler: nodestatus.Handler(p, version), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Info("node status page", "listen", listen, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
