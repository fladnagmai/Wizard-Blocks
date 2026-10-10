package stratum

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestConnLimitsValidate(t *testing.T) {
	for _, bad := range []ConnLimits{{0, 1}, {10, 0}, {10, 11}, {MaxConnLimit + 1, 1}, {-1, -1}} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	for _, ok := range []ConnLimits{{1024, 64}, {1, 1}, {MaxConnLimit, MaxConnLimit}} {
		if err := ok.Validate(); err != nil {
			t.Errorf("%+v refused: %v", ok, err)
		}
	}
}

// Limits change at runtime for new connections; lowering them never drops
// miners already connected.
func TestConnLimitsLive(t *testing.T) {
	s := testServer(t, testConfig()) // starts at 2 per IP
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Serve(ctx)
	defer s.Close()
	if got := s.ConnLimits(); got.MaxPerIP != 2 {
		t.Fatalf("initial limits from config: %+v", got)
	}
	dial := func() net.Conn {
		c, err := net.Dial("tcp", s.Addr())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	refused := func(c net.Conn) bool {
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, err := c.Read(make([]byte, 1))
		return err == io.EOF
	}
	waitOpen := func(n int) {
		for i := 0; i < 100; i++ {
			s.mu.Lock()
			got := len(s.sessions)
			s.mu.Unlock()
			if got == n {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("sessions never reached %d", n)
	}
	a, b := dial(), dial()
	waitOpen(2)
	if c := dial(); !refused(c) {
		t.Fatal("third connection from one IP accepted at limit 2")
	}
	if err := s.SetConnLimits(ConnLimits{MaxConnections: 100, MaxPerIP: 3}); err != nil {
		t.Fatal(err)
	}
	if c := dial(); refused(c) {
		t.Fatal("third connection refused after raising the per-IP limit to 3")
	}
	waitOpen(3)
	// Lower to 1: nobody is dropped, new connections are refused.
	if err := s.SetConnLimits(ConnLimits{MaxConnections: 100, MaxPerIP: 1}); err != nil {
		t.Fatal(err)
	}
	if c := dial(); !refused(c) {
		t.Fatal("new connection accepted above the lowered limit")
	}
	if refused(a) || refused(b) {
		t.Fatal("an existing miner was dropped when the limit was lowered")
	}
	waitOpen(3)
	// Total limit too.
	if err := s.SetConnLimits(ConnLimits{MaxConnections: 3, MaxPerIP: 3}); err != nil {
		t.Fatal(err)
	}
	if c := dial(); !refused(c) {
		t.Fatal("connection accepted above the total limit")
	}
	if err := s.SetConnLimits(ConnLimits{MaxConnections: 2, MaxPerIP: 3}); err == nil {
		t.Fatal("per-IP above total accepted")
	}
	if got := s.ConnLimits(); got.MaxConnections != 3 {
		t.Fatalf("a refused change was applied: %+v", got)
	}
}
