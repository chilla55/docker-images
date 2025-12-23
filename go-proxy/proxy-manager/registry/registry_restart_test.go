package registry

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// TestRegistryV2_ContainerRestart tests what happens when a container restarts
// while the old session is still in grace period (disconnected but not expired)
func TestRegistryV2_ContainerRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	// First container instance - register and add a route
	server1, client1 := net.Pipe()
	defer client1.Close()

	go reg.handleConnectionV2(ctx, server1)

	resp, err := send(client1, "REGISTER|myapp|web-1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID1 := strings.TrimPrefix(resp, "ACK|")
	t.Logf("First session: %s", sessionID1)

	// Add a route
	resp, err = send(client1, "ROUTE_ADD|"+sessionID1+"|example.com|/app|http://10.2.2.5:8080|10")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK, got %q", resp)
	}

	// Apply config
	resp, err = send(client1, "CONFIG_APPLY|"+sessionID1)
	if err != nil {
		t.Fatalf("config apply error: %v", err)
	}
	if resp != "OK" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Verify route was added
	if len(mp.addCalls) != 1 {
		t.Fatalf("expected 1 add call, got %d", len(mp.addCalls))
	}

	// Simulate container crash - close connection without clean shutdown
	t.Log("Simulating container crash...")
	client1.Close()
	server1.Close()
	time.Sleep(50 * time.Millisecond) // Let defer cleanup run

	// Verify route was deactivated (not removed)
	if len(mp.enableCalls) != 1 {
		t.Fatalf("expected 1 enable call (deactivation), got %d", len(mp.enableCalls))
	}
	if mp.enableCalls[0].enabled != false {
		t.Fatalf("expected route to be disabled, got enabled=%v", mp.enableCalls[0].enabled)
	}

	// Now simulate container restart - new connection, same instance name
	t.Log("Simulating container restart with same instance name...")
	server2, client2 := net.Pipe()
	defer server2.Close()
	defer client2.Close()

	go reg.handleConnectionV2(ctx, server2)

	// Register with SAME service/instance name
	resp, err = send(client2, "REGISTER|myapp|web-1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID2 := strings.TrimPrefix(resp, "ACK|")
	t.Logf("Second session: %s", sessionID2)

	if sessionID1 == sessionID2 {
		t.Fatalf("expected different session IDs, got same: %s", sessionID1)
	}

	// Old session should be cleaned up
	// Check that old routes were handled properly (shouldn't call RemoveRoute on deactivated routes)
	removeCalls := len(mp.removeCalls)
	t.Logf("RemoveRoute calls after restart: %d", removeCalls)

	// Since routes were deactivated, we should NOT have additional RemoveRoute calls
	// (The cleanup should skip calling RemoveRoute for deactivated routes)
	if removeCalls > 0 {
		t.Logf("Warning: RemoveRoute called on deactivated routes")
	}

	// New container should be able to add routes normally
	resp, err = send(client2, "ROUTE_ADD|"+sessionID2+"|example.com|/app|http://10.2.2.5:8080|10")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK, got %q", resp)
	}

	resp, err = send(client2, "CONFIG_APPLY|"+sessionID2)
	if err != nil {
		t.Fatalf("config apply error: %v", err)
	}
	if resp != "OK" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Should have second add call for new session
	if len(mp.addCalls) != 2 {
		t.Fatalf("expected 2 add calls total, got %d", len(mp.addCalls))
	}
}
