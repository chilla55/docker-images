package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockNginx struct{ reloaded int }

func (m *mockNginx) ScheduleReload(reason string) { m.reloaded++ }

func TestFetchIPRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"ipv4_cidrs": []string{"1.2.3.4/32"},
				"ipv6_cidrs": []string{"2001:db8::/64"},
			},
		})
	}))
	defer srv.Close()

	u := NewCloudflareUpdater(1*time.Hour, &mockNginx{}, true)
	ranges, err := u.fetchIPRanges(srv.URL)
	if err != nil {
		t.Fatalf("fetchIPRanges error: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
}
