package stats

import (
	"testing"
	"time"
)

// A worker's accepted shares land in its minute history (and the raw 10 s
// buckets); the history survives a save and reload; an unknown worker has
// none.
func TestWorkerHistory(t *testing.T) {
	dir := t.TempDir()
	c := New("bch", "t", dir)
	c.Connected("ash")
	for i := 0; i < 5; i++ {
		c.ShareAccepted("ash", 2048, 4096, "x")
	}
	s, ok := c.WorkerHistory("ash")
	if !ok || len(s.Minutes) != histMinutes || len(s.Raw) != numBuckets || s.Diff1Hashes != 4294967296 {
		t.Fatalf("series: ok=%v minutes=%d raw=%d", ok, len(s.Minutes), len(s.Raw))
	}
	if s.Minutes[len(s.Minutes)-1] != 5*2048 || s.Raw[len(s.Raw)-1] != 5*2048 {
		t.Fatalf("newest minute %v, newest raw %v, want %v", s.Minutes[len(s.Minutes)-1], s.Raw[len(s.Raw)-1], 5*2048)
	}
	if _, ok := c.WorkerHistory("nobody"); ok {
		t.Fatal("history for an unknown worker")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2 := New("bch", "t", dir)
	s2, ok := c2.WorkerHistory("ash")
	if !ok || s2.Minutes[len(s2.Minutes)-1] != 5*2048 {
		t.Fatalf("history not reloaded: %v", ok)
	}
	// A minute that is a week old falls out of the ring.
	var h minuteHist
	old := time.Unix(0, 0).Add(1000 * time.Minute)
	h.add(old, 7)
	end := old.Unix()/60 + histMinutes
	if v := h.series(end); v[0] != 0 || v[len(v)-1] != 0 {
		t.Fatal("a week-old minute still counted")
	}
	if v := h.series(old.Unix() / 60); v[len(v)-1] != 7 {
		t.Fatal("minute not stored")
	}
}
