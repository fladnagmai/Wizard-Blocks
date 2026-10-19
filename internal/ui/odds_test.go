package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// The chance of a block is shown as odds "1:N", not a percentage. Runs the
// page's own formatter under node (skipped where node is not installed).
func TestFmtOdds(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	js := `import {fmtOdds as o} from './static/js/format.js'; console.log([0.5, 0.0123, 1e-7, 2.3e-14, 1, 0, null, NaN].map(o).join('|'))`
	cmd := exec.Command(node, "--input-type=module", "-e", js)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "1:2|1:81|1:10,000,000|1:4.35E13|1:1|—|—|—"; got != want {
		t.Fatalf("fmtOdds: got %q, want %q", got, want)
	}
}
