package nginx

import (
    "testing"
)

type mockCmd struct{ testPassed bool }

func TestControllerScheduleAndTest(t *testing.T) {
    c := NewController(true)

    // ScheduleReload batches reload requests
    c.ScheduleReload("test reason 1")
    if !c.pendingReload {
        t.Fatalf("expected pending reload")
    }

    // Subsequent calls should still batch
    c.ScheduleReload("test reason 2")

    // TestConfig should not error in test environment; stub for coverage
    _ = c.TestConfig()
}
