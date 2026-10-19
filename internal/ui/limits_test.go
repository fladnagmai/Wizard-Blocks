package ui

import (
	"net/http"
	"testing"

	"github.com/fladnagmai/wizard-blocks/internal/stratum"
)

type limitsBackend struct {
	*fakeBackend
	l, def stratum.ConnLimits
	sets   int
}

func (f *limitsBackend) ConnLimits() (stratum.ConnLimits, stratum.ConnLimits, bool) {
	return f.l, f.def, f.l != f.def
}
func (f *limitsBackend) SetConnLimits(l stratum.ConnLimits) error { f.l = l; f.sets++; return nil }
func (f *limitsBackend) ResetConnLimits() error                   { f.l = f.def; return nil }

func TestAdminLimits(t *testing.T) {
	s, fb, base := startUI(t, "pw")
	def := stratum.ConnLimits{MaxConnections: 1024, MaxPerIP: 64}
	lb := &limitsBackend{fakeBackend: fb, l: def, def: def}
	s.SetSettingsBackend(lb)
	c := newClient(t, base)
	if code, _ := c.do("GET", "/api/admin/limits", "", false); code != http.StatusUnauthorized {
		t.Fatalf("GET without login: %d", code)
	}
	if code, _ := c.do("PUT", "/api/admin/limits", `{"max_connections":5000,"max_connections_per_ip":500}`, true); code != http.StatusUnauthorized {
		t.Fatalf("PUT without login: %d", code)
	}
	if code, _ := c.do("POST", "/api/admin/login", `{"password":"pw"}`, true); code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	code, m := c.do("GET", "/api/admin/limits", "", false)
	if code != 200 || m["limits"].(map[string]any)["max_connections"] != 1024.0 || m["saved"] != false {
		t.Fatalf("GET: %d %v", code, m)
	}
	if code, _ := c.do("PUT", "/api/admin/limits", `{"max_connections":5000,"max_connections_per_ip":500}`, false); code != http.StatusForbidden {
		t.Fatalf("PUT without CSRF header: %d", code)
	}
	for _, bad := range []string{
		`{"max_connections":0,"max_connections_per_ip":1}`,
		`{"max_connections":10,"max_connections_per_ip":11}`,
		`{"max_connections":100001,"max_connections_per_ip":1}`,
		`{"max_connections":10,"max_connections_per_ip":5,"x":1}`,
		`nope`,
	} {
		if code, _ := c.do("PUT", "/api/admin/limits", bad, true); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
	if lb.sets != 0 {
		t.Fatal("invalid limits reached the backend")
	}
	code, m = c.do("PUT", "/api/admin/limits", `{"max_connections":5000,"max_connections_per_ip":500}`, true)
	if code != 200 || lb.l.MaxConnections != 5000 || lb.l.MaxPerIP != 500 || m["saved"] != true {
		t.Fatalf("PUT: %d %v %+v", code, m, lb.l)
	}
	if code, _ := c.do("DELETE", "/api/admin/limits", "", false); code != http.StatusForbidden {
		t.Fatalf("DELETE without CSRF header: %d", code)
	}
	if code, m := c.do("DELETE", "/api/admin/limits", "", true); code != 200 || lb.l != def || m["saved"] != false {
		t.Fatalf("DELETE: %d %v", code, m)
	}
	// An engine without adjustable limits answers 404.
	s.SetSettingsBackend(fb)
	if code, _ := c.do("GET", "/api/admin/limits", "", false); code != http.StatusNotFound {
		t.Fatalf("no LimitsBackend: %d", code)
	}
}
