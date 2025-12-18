package registry

import "testing"

type dummyProxy struct{}

func (d dummyProxy) AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error {
	return nil
}
func (d dummyProxy) RemoveRoute(domains []string, path string) {}

func TestCalculateRoutesHashAndParity(t *testing.T) {
	r := NewRegistry(0, 0, dummyProxy{}, true)
	svc := &Service{Routes: []ServiceRoute{{Domains: []string{"a.com", "b.com"}, Path: "/x", Backend: "http://upstream"}}, Headers: map[string]string{"X": "1"}}
	h := r.calculateRoutesHash(svc)
	if h == "" {
		t.Fatalf("hash should not be empty")
	}

	p := r.calculateParity(h)
	if p != 0 && p != 1 {
		t.Fatalf("parity should be 0 or 1, got %d", p)
	}

	if r.IsRegistered("host", 80) {
		t.Fatalf("unexpected registered state")
	}
	port, ok := r.GetMaintenancePort("host", 80)
	if ok {
		t.Fatalf("maintenance port should not be found for unknown service")
	}
	if port != 81 {
		t.Fatalf("default maintenance port should be 81")
	}
}
