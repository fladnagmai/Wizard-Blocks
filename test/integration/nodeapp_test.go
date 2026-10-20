//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/nodestatus"
)

// ensureNodeImage builds the node app's image for coin from nodes/ (the
// build verifies the release signature and checksum itself).
func ensureNodeImage(t *testing.T, coin string) string {
	t.Helper()
	image := "wbit-node-" + coin + ":test"
	if exec.Command("docker", "image", "inspect", image).Run() == nil {
		return image
	}
	out, err := exec.Command("docker", "build", "-q", "-t", image, "-f", filepath.Join("..", "..", "nodes", coin, "Dockerfile"), filepath.Join("..", "..", "nodes")).CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", image, err, out)
	}
	return image
}

// TestNodeAppThreeNodes runs the node app's three images (Litecoin, Dogecoin,
// Pepecoin) the way its docker-compose.yml does, on regtest: the status page
// writes each node's conf first (prune target, cache), every node starts
// from it, reports its memory through memwrap, the Pepecoin sync can be
// paused and resumed, and each node shuts down cleanly when stopped.
func TestNodeAppThreeNodes(t *testing.T) {
	track(t)
	suffix := randHex(3)
	netName := "wbit-na-" + suffix
	docker(t, "network", "create", netName)
	t.Cleanup(func() { docker(t, "network", "rm", netName) })
	confDir, memDir := t.TempDir(), t.TempDir()
	os.Chmod(confDir, 0o777)
	os.Chmod(memDir, 0o777)

	specs := []struct {
		key, coin string
		prune     nodestatus.PruneConfig
		cache     nodestatus.CacheConfig
	}{
		{"ltc", "litecoin", nodestatus.PruneConfig{Default: 2000, Min: 550, Options: []int{550, 2000}}, nodestatus.CacheConfig{Default: 300, Options: []int{300, 450}}},
		{"doge", "dogecoin", nodestatus.PruneConfig{Default: 2200, Min: 2200, Options: []int{2200, 5000}}, nodestatus.CacheConfig{Default: 200, Options: []int{200, 450}}},
		{"pep", "pepecoin", nodestatus.PruneConfig{Default: 2200, Min: 2200, Options: []int{2200, 5000}}, nodestatus.CacheConfig{Default: 100, Options: []int{100, 200}}},
	}
	var nodes []nodestatus.Node
	names, rpcPorts := map[string]string{}, map[string]int{}
	for _, s := range specs {
		pc, cc := s.prune, s.cache
		pc.ConfFile = filepath.Join(confDir, s.coin+".conf")
		ports := freePorts(t, 1)
		name := fmt.Sprintf("wbit-na-%s-%s", s.key, suffix)
		names[s.key], rpcPorts[s.key] = name, ports[0]
		nodes = append(nodes, nodestatus.Node{Key: s.key, Name: s.coin, Pausable: s.key != "ltc", Prune: &pc, Cache: &cc,
			MemFile: filepath.Join(memDir, s.key),
			RPC:     node.NewClient(fmt.Sprintf("http://127.0.0.1:%d", ports[0]), rpcUser, rpcPass, "", 10*time.Second)})
		t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	}
	p := nodestatus.NewPoller(nodes, time.Second, filepath.Join(t.TempDir(), "state.json"))
	if fixed, err := p.EnsurePruneConf(); err != nil || len(fixed) != 3 {
		t.Fatalf("conf files not written: %v %v", fixed, err)
	}
	for i, s := range specs {
		if !nodestatus.ConfReady(nodes[i].Prune.ConfFile, s.prune.Min, true) {
			t.Fatalf("%s conf not ready", s.key)
		}
		b, _ := os.ReadFile(nodes[i].Prune.ConfFile)
		t.Logf("%s.conf written by the page: %q", s.coin, string(b))
		port := fmt.Sprint(rpcPorts[s.key])
		args := []string{"run", "-d", "--name", names[s.key], "--network", netName, "--user", "1000:1000",
			"-e", "MEM_FILE=/mem/" + s.key, "-v", memDir + ":/mem", "-v", confDir + ":/conf:ro",
			"-p", "127.0.0.1:" + port + ":18443", ensureNodeImage(t, s.coin),
			"-regtest", "-datadir=/data", "-conf=/conf/" + s.coin + ".conf", "-printtoconsole", "-server", "-txindex=0",
			"-rpcbind=0.0.0.0", "-rpcallowip=0.0.0.0/0", "-rpcport=18443", "-rpcuser=" + rpcUser, "-rpcpassword=" + rpcPass}
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("start %s: %v\n%s", s.key, err, out)
		}
		nodesStarted.Add(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	byKey := func() map[string]nodestatus.Status {
		p.CheckAll(ctx)
		m := map[string]nodestatus.Status{}
		for _, s := range p.Snapshot() {
			m[s.Key] = s
		}
		return m
	}
	waitFor(t, "three nodes up with prune and memory reports", 90*time.Second, func() bool {
		m := byKey()
		for _, s := range specs {
			st := m[s.key]
			if (st.State != "synced" && st.State != "syncing") || st.MemBytes == 0 || st.PruneRunning != s.prune.Default || st.CacheSet != s.cache.Default || !st.Pruned {
				return false
			}
		}
		return true
	})
	// memwrap's first report comes as the node starts; wait for one from
	// the running node.
	waitFor(t, "memory reports from the running nodes", 60*time.Second, func() bool {
		m := byKey()
		for _, s := range specs {
			if m[s.key].MemBytes < 8<<20 {
				return false
			}
		}
		return true
	})
	var total int64
	for _, s := range specs {
		st := byKey()[s.key]
		total += st.MemBytes
		t.Logf("%s: %s, prune %d MiB (running), cache %d MiB, memory %.1f MiB, %s", s.key, st.State, st.PruneRunning, st.CacheSet, float64(st.MemBytes)/(1<<20), st.Version)
	}
	t.Logf("regtest memory, all three nodes: %.1f MiB", float64(total)/(1<<20))

	// Pause and resume the Pepecoin sync from the page's API.
	if err := p.SetPaused(ctx, "pep", true); err != nil {
		t.Fatal(err)
	}
	if st := byKey()["pep"]; !st.Paused || st.NetworkActive == nil || *st.NetworkActive {
		t.Fatalf("pep not paused: %+v", st)
	}
	if err := p.SetPaused(ctx, "pep", false); err != nil {
		t.Fatal(err)
	}
	if st := byKey()["pep"]; st.Paused || st.NetworkActive == nil || !*st.NetworkActive {
		t.Fatalf("pep not resumed: %+v", st)
	}

	// A clean shutdown through memwrap: SIGTERM reaches the node, exit 0.
	for _, s := range specs {
		docker(t, "stop", "-t", "60", names[s.key])
		code := docker(t, "inspect", "-f", "{{.State.ExitCode}}", names[s.key])
		logs, _ := exec.Command("docker", "logs", "--tail", "5", names[s.key]).CombinedOutput()
		if strings.TrimSpace(code) != "0" || !strings.Contains(string(logs), "Shutdown: done") {
			t.Fatalf("%s did not shut down cleanly: exit %s\n%s", s.key, code, logs)
		}
	}
}
