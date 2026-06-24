
package aibalance

import (
	"testing"
	"time"
)

// TestLatencyWatcher_SingleflightCooldown verifies that the
// shouldTriggerLatencyCheckLocked gate blocks when a check is already in flight
// (singleflight) and until the cooldown elapses after the last check. This is
// the core fix for the health-check amplification that piled up duplicate
// 20s-timeout goroutines for the same unhealthy provider every fast tick.
func TestLatencyWatcher_SingleflightCooldown(t *testing.T) {
	w := NewLatencyWatcher()

	// Initially allowed.
	if !w.shouldTriggerLatencyCheckLocked(42) {
		t.Fatalf("first check for a provider should be allowed")
	}

	// Mark a check in flight -> singleflight blocks.
	w.healthCheckInFlight[42] = true
	if w.shouldTriggerLatencyCheckLocked(42) {
		t.Fatalf("check in flight should block a new check (singleflight)")
	}
	// Different provider unaffected.
	if !w.shouldTriggerLatencyCheckLocked(99) {
		t.Fatalf("singleflight on one provider must not block a different provider")
	}

	// Check completes; cooldown now applies.
	w.healthCheckInFlight[42] = false
	w.lastLatencyCheckAt[42] = time.Now()
	if w.shouldTriggerLatencyCheckLocked(42) {
		t.Fatalf("cooldown should block an immediate re-check")
	}

	// After cooldown elapses, allowed again.
	w.lastLatencyCheckAt[42] = time.Now().Add(-(latencyCheckCooldown + time.Second))
	if !w.shouldTriggerLatencyCheckLocked(42) {
		t.Fatalf("check should be allowed after cooldown elapses")
	}
}

// TestLatencyWatcher_NewInstanceMapsInit verifies the new tracking maps are
// initialized (non-nil) so the background loop doesn't panic on a fresh watcher.
func TestLatencyWatcher_NewInstanceMapsInit(t *testing.T) {
	w := NewLatencyWatcher()
	if w.healthCheckInFlight == nil {
		t.Fatalf("healthCheckInFlight must be initialized")
	}
	if w.lastLatencyCheckAt == nil {
		t.Fatalf("lastLatencyCheckAt must be initialized")
	}
}
