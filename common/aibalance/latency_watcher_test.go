package aibalance

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// TestIsProviderProblematic tests the isProviderProblematic logic
func TestIsProviderProblematic(t *testing.T) {
	watcher := NewLatencyWatcher()

	tests := []struct {
		name     string
		provider *schema.AiProvider
		expected bool
	}{
		{
			name: "first check not completed - should be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             true,
				LastLatency:           500,
				IsFirstCheckCompleted: false,
			},
			expected: true,
		},
		{
			name: "healthy with good latency - should not be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             true,
				LastLatency:           500,
				IsFirstCheckCompleted: true,
			},
			expected: false,
		},
		{
			name: "not healthy - should be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             false,
				LastLatency:           500,
				IsFirstCheckCompleted: true,
			},
			expected: true,
		},
		{
			name: "zero latency - should be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             true,
				LastLatency:           0,
				IsFirstCheckCompleted: true,
			},
			expected: true,
		},
		{
			name: "high latency (>= 10s) - should be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             true,
				LastLatency:           10000, // 10 seconds
				IsFirstCheckCompleted: true,
			},
			expected: true,
		},
		{
			name: "latency just below threshold - should not be problematic",
			provider: &schema.AiProvider{
				IsHealthy:             true,
				LastLatency:           9999,
				IsFirstCheckCompleted: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := watcher.isProviderProblematic(tt.provider)
			if result != tt.expected {
				t.Errorf("isProviderProblematic() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestLatencyWatcherSingleton tests that GetGlobalLatencyWatcher returns the same instance
func TestLatencyWatcherSingleton(t *testing.T) {
	// Note: We cannot reset sync.Once, so we just verify the singleton behavior
	// by calling GetGlobalLatencyWatcher multiple times
	watcher1 := GetGlobalLatencyWatcher()
	watcher2 := GetGlobalLatencyWatcher()

	if watcher1 != watcher2 {
		t.Error("GetGlobalLatencyWatcher should return the same instance")
	}
}

// TestLatencyWatcherStartStop tests the start and stop functionality
func TestLatencyWatcherStartStop(t *testing.T) {
	watcher := NewLatencyWatcher()

	// Start the watcher
	watcher.Start()

	// Verify it's running
	watcher.mutex.RLock()
	running := watcher.running
	watcher.mutex.RUnlock()

	if !running {
		t.Error("Watcher should be running after Start()")
	}

	// Starting again should not cause issues
	watcher.Start() // Should just log that it's already running

	// Stop the watcher
	watcher.Stop()

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	watcher.mutex.RLock()
	running = watcher.running
	watcher.mutex.RUnlock()

	if running {
		t.Error("Watcher should not be running after Stop()")
	}
}

// TestMarkProviderAsProblematic tests marking a provider as problematic
func TestMarkProviderAsProblematic(t *testing.T) {
	watcher := NewLatencyWatcher()

	// Initially should have no problematic providers
	if watcher.GetProblematicProviderCount() != 0 {
		t.Error("Should have 0 problematic providers initially")
	}

	// Mark a provider as problematic (this will try to trigger health check which may fail without DB)
	watcher.MarkProviderAsProblematic(1, "test-provider")

	// Should now have 1 problematic provider
	if watcher.GetProblematicProviderCount() != 1 {
		t.Errorf("Should have 1 problematic provider, got %d", watcher.GetProblematicProviderCount())
	}

	// Marking the same provider again should not increase count
	watcher.MarkProviderAsProblematic(1, "test-provider")

	if watcher.GetProblematicProviderCount() != 1 {
		t.Errorf("Should still have 1 problematic provider, got %d", watcher.GetProblematicProviderCount())
	}

	// Mark a different provider
	watcher.MarkProviderAsProblematic(2, "test-provider-2")

	if watcher.GetProblematicProviderCount() != 2 {
		t.Errorf("Should have 2 problematic providers, got %d", watcher.GetProblematicProviderCount())
	}
}

// TestNewLatencyWatcher tests the default values of a new LatencyWatcher
func TestNewLatencyWatcher(t *testing.T) {
	watcher := NewLatencyWatcher()

	if watcher.normalInterval != 5*time.Minute {
		t.Errorf("normalInterval should be 5m, got %v", watcher.normalInterval)
	}

	if watcher.fastInterval != 10*time.Second {
		t.Errorf("fastInterval should be 10s, got %v", watcher.fastInterval)
	}

	if watcher.latencyThreshold != 10000 {
		t.Errorf("latencyThreshold should be 10000, got %d", watcher.latencyThreshold)
	}

	if watcher.running {
		t.Error("watcher should not be running initially")
	}

	if len(watcher.problematicIDs) != 0 {
		t.Error("problematicIDs should be empty initially")
	}
}
