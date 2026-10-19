//go:build integration && upgrade

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Upgrade and rollback: the engine of the currently published app versions
// is started on a data folder with saved settings, stopped, and the new
// engine is started on the same folder; every saved setting must come back
// unchanged. Then the old engine is started on that same folder again
// (rollback) and must still start with the same settings.
//
// The binaries are built from the old and the new source with the same
// command as the Dockerfile; WB_OLD_BIN and WB_NEW_BIN name them. The
// coin's node runs in Docker as in the other regtests. Run with:
//
//	WB_OLD_BIN=... WB_NEW_BIN=... go test -race -tags "integration upgrade" -run TestUpgradeRollback ./test/integration

type proc struct {
	cmd  *exec.Cmd
	base string
	hc   *http.Client
	log  string
}

func startProc(t *testing.T, bin string, env []string, dataDir, name string) *proc {
	t.Helper()
	ports := freePorts(t, 3)
	f, path := logFile(t, name)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("WB_STRATUM_LISTEN=127.0.0.1:%d", ports[0]), fmt.Sprintf("WB_UI_LISTEN=127.0.0.1:%d", ports[1]),
		fmt.Sprintf("WB_API_LISTEN=127.0.0.1:%d", ports[2]), "WB_DATA_DIR="+dataDir, "WB_UI_SETTINGS_OPEN=true", "WB_LOG_FORMAT=json")
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	jar, _ := cookiejar.New(nil)
	p := &proc{cmd: cmd, base: fmt.Sprintf("http://127.0.0.1:%d", ports[1]), hc: &http.Client{Jar: jar, Timeout: 20 * time.Second}, log: path}
	t.Cleanup(func() { p.stop(t); f.Close() })
	waitFor(t, name+" UI", 60*time.Second, func() bool {
		r, err := p.hc.Get(p.base + "/api/state")
		if err != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == 200
	})
	return p
}

func (p *proc) stop(t *testing.T) {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
		t.Errorf("engine did not stop within 90 s")
	}
	p.cmd = nil
}

func (p *proc) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, p.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", p.base)
	req.Header.Set("X-WB-Admin", "1")
	r, err := p.hc.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer r.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(r.Body).Decode(&m)
	return r.StatusCode, m
}

// dirHashes maps every file in the data folder to its sha256.
func dirHashes(t *testing.T, dir string) map[string]string {
	out := map[string]string{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	return out
}

func TestUpgradeRollback(t *testing.T) {
	oldBin, newBin := os.Getenv("WB_OLD_BIN"), os.Getenv("WB_NEW_BIN")
	if oldBin == "" || newBin == "" {
		t.Fatal("WB_OLD_BIN and WB_NEW_BIN must name the old and the new engine binaries")
	}
	coin := os.Getenv("WB_UPGRADE_COIN")
	if coin == "" {
		t.Fatal("WB_UPGRADE_COIN must be bch, btc or ltc")
	}
	track(t)
	// The page style each app runs today (the new compose files all use simple).
	oldStyle := map[string]string{"bch": "mine", "btc": "simple", "ltc": "stats"}[coin]

	suffix := randHex(3)
	netName := "wbup-" + suffix
	docker(t, "network", "create", netName)
	t.Cleanup(func() { docker(t, "network", "rm", netName) })
	a := startNode(t, coin, netName, "wbup-"+coin+"-"+suffix)
	var none any
	a.call(t, a.RPC, "createwallet", &none, "w")
	payout := a.newAddress(t, "")
	if coin == "btc" {
		payout = a.newAddress(t, "bech32")
	}

	dataDir := t.TempDir()
	env := func(style string) []string {
		return []string{
			"WB_COIN=" + coin, "WB_RPC_URL=" + a.RPCURL, "WB_RPC_USER=" + rpcUser, "WB_RPC_PASSWORD=" + rpcPass,
			"WB_ZMQ_HASHBLOCK=" + a.ZMQ, "WB_PAYOUT_MODE=fixed", "WB_PAYOUT_ADDRESS=", "WB_UI_STYLE=" + style,
			"WB_POLL_INTERVAL_MS=500", "WB_TEMPLATE_REFRESH_S=2", "WB_COINBASE_TAG=/wizard-blocks-up/",
		}
	}
	checklist := map[string]string{}
	mark := func(name string, ok bool, detail string) {
		res := "PASS"
		if !ok {
			res = "FAIL"
			t.Errorf("%s: %s", name, detail)
		}
		checklist[name] = res
		t.Logf("CHECK %-52s %s %s", name, res, detail)
	}

	// 1. Old engine: save every setting that engine has.
	old := startProc(t, oldBin, env(oldStyle), dataDir, "old")
	if code, m := old.do(t, "PUT", "/api/admin/payout", map[string]string{"address": payout}); code != 200 {
		t.Fatalf("old: set payout: %d %v", code, m)
	}
	want := map[string]any{
		"vardiff_min": 2.0, "vardiff_max": 5e6, "vardiff_target_seconds": 7.0, "fixed_diff": 0.0,
		"worker_overrides": map[string]any{"rental.*": 1e6, "ernie": 4096.0},
	}
	full := map[string]any{"start_diff": 1000.0, "ignore_password_diff": true, "ignore_suggest_difficulty": true}
	body := map[string]any{}
	for k, v := range want {
		body[k] = v
	}
	for k, v := range full {
		body[k] = v
	}
	code, m := old.do(t, "PUT", "/api/admin/settings", body)
	if code != 200 { // an older engine without the extra fields
		body = map[string]any{}
		for k, v := range want {
			body[k] = v
		}
		code, m = old.do(t, "PUT", "/api/admin/settings", body)
	}
	if code != 200 {
		t.Fatalf("old: save settings: %d %v", code, m)
	}
	savedSettings := m["settings"].(map[string]any)
	hasLimits := false
	var savedLimits map[string]any
	if code, m = old.do(t, "PUT", "/api/admin/limits", map[string]any{"max_connections": 777, "max_connections_per_ip": 33}); code == 200 {
		hasLimits = true
		savedLimits = m["limits"].(map[string]any)
	}
	if code, m := old.do(t, "POST", "/api/admin/password", map[string]string{"password": "upgrade-test-pw"}); code != 200 {
		t.Fatalf("old: set password: %d %v", code, m)
	}
	old.stop(t)
	before := dirHashes(t, dataDir)
	t.Logf("data folder after the old engine: %d files", len(before))
	for _, f := range []string{"settings-" + coin + ".json", "payout-" + coin + ".json", "ui-password-" + coin + ".json"} {
		if _, ok := before[f]; !ok {
			t.Fatalf("old engine did not save %s", f)
		}
	}
	if hasLimits {
		if _, ok := before["limits-"+coin+".json"]; !ok {
			t.Fatal("old engine did not save limits file")
		}
	}

	// verify opens a session and compares every setting with what was saved.
	verify := func(p *proc, who string) {
		if code, _ := p.do(t, "GET", "/api/admin/settings", nil); code != 401 {
			mark(who+": password still required", false, fmt.Sprintf("settings answered %d without login", code))
		} else {
			mark(who+": password still required", true, "")
		}
		if code, m := p.do(t, "POST", "/api/admin/login", map[string]string{"password": "upgrade-test-pw"}); code != 200 {
			t.Fatalf("%s: login with the saved password failed: %d %v", who, code, m)
		}
		mark(who+": saved password accepted", true, "")
		_, m := p.do(t, "GET", "/api/admin/settings", nil)
		got, _ := m["settings"].(map[string]any)
		for k, v := range savedSettings {
			mark(who+": setting "+k, reflect.DeepEqual(got[k], v), fmt.Sprintf("saved %v, now %v", v, got[k]))
		}
		_, st := p.do(t, "GET", "/api/state", nil)
		sx, _ := st["stratum"].(map[string]any)
		mark(who+": payout address", sx["payout_address"] == payout && sx["payout_set"] == true, fmt.Sprintf("saved %s, now %v", payout, sx["payout_address"]))
		if hasLimits {
			_, lm := p.do(t, "GET", "/api/admin/limits", nil)
			gl, _ := lm["limits"].(map[string]any)
			mark(who+": connection limits", reflect.DeepEqual(gl, savedLimits), fmt.Sprintf("saved %v, now %v", savedLimits, gl))
		}
	}

	// 2. New engine on the same data: everything survives, nothing is rewritten.
	nw := startProc(t, newBin, env("simple"), dataDir, "new")
	verify(nw, "new")
	if got := dirHashes(t, dataDir); true {
		for f, h := range before {
			if got[f] != h && !strings.HasPrefix(f, "state-") && !strings.HasPrefix(f, "history-") {
				mark("new: "+f+" left unchanged on start", false, "file changed")
			}
		}
		mark("new: saved files left unchanged on start", !t.Failed(), "")
	}
	// The new page: served, themed, with every settings control.
	r, err := nw.hc.Get(nw.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(r.Body)
	r.Body.Close()
	for _, id := range []string{`id="d-min"`, `id="d-max"`, `id="d-target"`, `id="d-fixed"`, `id="d-start"`, `id="d-over"`, `id="d-pw"`, `id="d-sg"`,
		`id="d-reset"`, `id="s-addr"`, `id="s-maxconn"`, `id="s-maxip"`, `id="s-lim-reset"`, `id="s-pw"`, `id="s-logout"`} {
		mark("new page: control "+id, strings.Contains(string(page), id), "")
	}
	// A limits change in the new engine (the control that old BCH/BTC lack).
	if code, m := nw.do(t, "PUT", "/api/admin/limits", map[string]any{"max_connections": 555, "max_connections_per_ip": 22}); code != 200 {
		t.Fatalf("new: save limits: %d %v", code, m)
	}
	mark("new: connection limits saved", true, "")
	// Every setting of the new engine, including the ones an old engine may
	// lack (start difficulty, ignore d=, ignore suggest), saved and reloaded.
	newBody := map[string]any{}
	for k, v := range want {
		newBody[k] = v
	}
	for k, v := range map[string]any{"start_diff": 1500.0, "ignore_password_diff": true, "ignore_suggest_difficulty": false} {
		newBody[k] = v
	}
	newBody["vardiff_target_seconds"] = 9.0
	code, m = nw.do(t, "PUT", "/api/admin/settings", newBody)
	if code != 200 {
		t.Fatalf("new: save every setting: %d %v", code, m)
	}
	savedNew := m["settings"].(map[string]any)
	nw.stop(t)
	afterNew := dirHashes(t, dataDir)
	if _, ok := afterNew["limits-"+coin+".json"]; !ok {
		mark("new: limits-"+coin+".json written", false, "missing")
	} else {
		mark("new: limits-"+coin+".json written", true, "")
	}

	// 3. New engine again (restart reload), then rollback to the old engine.
	nw2 := startProc(t, newBin, env("simple"), dataDir, "new-restart")
	_, lm := nw2.do(t, "POST", "/api/admin/login", map[string]string{"password": "upgrade-test-pw"})
	_ = lm
	_, lmr := nw2.do(t, "GET", "/api/admin/limits", nil)
	gl, _ := lmr["limits"].(map[string]any)
	mark("new restart: connection limits reloaded", gl["max_connections"] == 555.0 && gl["max_connections_per_ip"] == 22.0, fmt.Sprintf("%v", gl))
	verifySettingsOnly := func(p *proc, who string, saved map[string]any) {
		_, m := p.do(t, "GET", "/api/admin/settings", nil)
		got, _ := m["settings"].(map[string]any)
		for k, v := range saved {
			mark(who+": setting "+k, reflect.DeepEqual(got[k], v), fmt.Sprintf("saved %v, now %v", v, got[k]))
		}
	}
	verifySettingsOnly(nw2, "new restart", savedNew)
	nw2.stop(t)

	rb := startProc(t, oldBin, env(oldStyle), dataDir, "old-rollback")
	if code, m := rb.do(t, "POST", "/api/admin/login", map[string]string{"password": "upgrade-test-pw"}); code != 200 {
		t.Fatalf("rollback: login failed: %d %v", code, m)
	}
	mark("rollback: old engine starts on the upgraded data", true, "")
	// The old engine reads the fields it knows; the rest stays in the file.
	oldKnown := map[string]any{}
	for k := range savedSettings {
		oldKnown[k] = savedNew[k]
	}
	verifySettingsOnly(rb, "rollback", oldKnown)
	_, st := rb.do(t, "GET", "/api/state", nil)
	sx, _ := st["stratum"].(map[string]any)
	mark("rollback: payout address", sx["payout_address"] == payout, fmt.Sprintf("%v", sx["payout_address"]))
	rb.stop(t)

	names := make([]string, 0, len(checklist))
	for k := range checklist {
		names = append(names, k)
	}
	sort.Strings(names)
	pass := 0
	for _, k := range names {
		if checklist[k] == "PASS" {
			pass++
		}
	}
	t.Logf("UPGRADE SUMMARY %s: %d checks, %d passed, %d failed", coin, len(names), pass, len(names)-pass)
}
