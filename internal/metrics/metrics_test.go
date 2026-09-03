package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/cronus/internal/ntp"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	return rec.Body.String()
}

func TestObserveServerExportsGauges(t *testing.T) {
	m := New()
	m.ObserveServer("id-1", ntp.ServerResult{
		Target: "time.cloudflare.com", Reachable: true,
		Offset: 5 * time.Millisecond, RTT: 20 * time.Millisecond, Stratum: 3,
	})
	body := scrape(t, m)
	for _, want := range []string{
		`cronus_reachable{id="id-1",server="time.cloudflare.com"} 1`,
		`cronus_offset_seconds{id="id-1",server="time.cloudflare.com"} 0.005`,
		`cronus_stratum{id="id-1",server="time.cloudflare.com"} 3`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\n---\n%s", want, body)
		}
	}
}

func TestForgetServerRemovesSeries(t *testing.T) {
	m := New()
	m.ObserveServer("id-1", ntp.ServerResult{Target: "a", Reachable: true})
	m.ForgetServer("id-1", "a")
	if strings.Contains(scrape(t, m), `id="id-1"`) {
		t.Error("series for forgotten server still present")
	}
}

func TestUnreachableSetsReachableZero(t *testing.T) {
	m := New()
	m.ObserveServer("id-2", ntp.ServerResult{Target: "b", Reachable: false})
	body := scrape(t, m)
	if !strings.Contains(body, `cronus_reachable{id="id-2",server="b"} 0`) {
		t.Errorf("expected reachable 0 for unreachable server:\n%s", body)
	}
}
