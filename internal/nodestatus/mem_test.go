package nodestatus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A node's memory report (kB from memwrap) is shown in bytes; a stale one
// (node stopped) or a broken one is not shown.
func TestReadMem(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pep")
	if b, _ := readMem(p); b != 0 {
		t.Fatal("missing file shown")
	}
	os.WriteFile(p, []byte("52788\n"), 0o644)
	if b, age := readMem(p); b != 52788*1024 || age > 5 {
		t.Fatalf("got %d bytes, age %d", b, age)
	}
	old := time.Now().Add(-5 * time.Minute)
	os.Chtimes(p, old, old)
	if b, _ := readMem(p); b != 0 {
		t.Fatal("stale report shown")
	}
	os.WriteFile(p, []byte("garbage"), 0o644)
	if b, _ := readMem(p); b != 0 {
		t.Fatal("garbage shown")
	}
	if b, _ := readMem(""); b != 0 {
		t.Fatal("no file configured but shown")
	}
	if total, _ := HostMemory(); total <= 0 {
		t.Skip("no /proc/meminfo here")
	}
}
