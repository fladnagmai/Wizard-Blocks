//go:build screens

package poolview

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fladnagmai/wizard-blocks/internal/node"
	"github.com/fladnagmai/wizard-blocks/internal/stats"
	"github.com/fladnagmai/wizard-blocks/internal/ui"
)

// TestScreens screenshots the portal and the view-only BCH and LTC pages
// (with real engine UIs behind them) at desktop and phone width.
// Run: WB_SCREENS_OUT=<dir> go test -tags screens -run TestScreens ./internal/poolview
func TestScreens(t *testing.T) {
	out := os.Getenv("WB_SCREENS_OUT")
	if out == "" {
		out = t.TempDir()
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := func(coin string, netDiff float64) string {
		st := stats.New(coin, "1.0.0-beta", "")
		st.SetNode(func(n *stats.NodeStatus) {
			n.Connected, n.Synced, n.Height, n.Headers, n.Chain, n.ZMQEnabled, n.ZMQConnected = true, true, 950123, 950123, "main", true, true
		})
		st.SetTemplate(func(ti *stats.TemplateInfo) { ti.PrevHash = "p"; ti.Height = 950124; ti.NetworkDiff = netDiff })
		for _, w := range []string{"ash", "bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a"} {
			st.Connected(w)
			for i := 0; i < 40; i++ {
				st.ShareAccepted(w, 2048, 2048*float64(1+i%9), "bip310")
			}
		}
		st.BlockSubmitted(stats.BlockRecord{Height: 950100, Hash: "00000000000000000a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778", Worker: "ash", Address: "bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a", Status: "accepted", Time: time.Now().Add(-5 * time.Hour)})
		srv := ui.New(ui.Config{Coin: coin, Version: "1.0.0-beta", StratumPort: 62023, PayoutMode: "fixed", PayoutAddress: "bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a", SettingsOpen: true, Style: "simple"},
			st, node.NewClient("http://127.0.0.1:1", "u", "p", "", time.Second), log)
		if err := srv.Listen("127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		go srv.Serve()
		t.Cleanup(func() { srv.Shutdown(context.Background()) })
		return "http://" + srv.Addr()
	}
	pv := New([]Pool{{Key: "bch", Label: "BCH", URL: engine("bch", 4.3e11)}, {Key: "ltc", Label: "LTC", URL: engine("ltc", 87e6*65536)}, {Key: "btc", Label: "BTC", URL: "http://127.0.0.1:1"}},
		ui.SimpleFiles(), ui.StaticFiles(), log)
	srv := httptest.NewServer(pv.Handler())
	defer srv.Close()
	cfg, _ := json.Marshal(map[string]any{"out": out, "base": srv.URL})
	b, err := exec.Command("node", filepath.Join("testdata", "screens.js"), string(cfg)).CombinedOutput()
	t.Logf("%s", b)
	if err != nil {
		t.Fatal(err)
	}
}
