package scannode

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/node"
)

func TestInvokeLimiterMaxOneIsNonBlockingAndReleaseIsIdempotent(t *testing.T) {
	limiter := newInvokeLimiter(1)
	release, acquired := limiter.TryAcquire()
	if !acquired || release == nil {
		t.Fatal("first slot was not acquired")
	}
	if limiter.activeCount() != 1 {
		t.Fatalf("active count = %d, want 1", limiter.activeCount())
	}
	if _, acquired := limiter.TryAcquire(); acquired {
		t.Fatal("second slot unexpectedly blocked/acquired at max=1")
	}
	release()
	release()
	if limiter.activeCount() != 0 {
		t.Fatalf("active count after idempotent release = %d, want 0", limiter.activeCount())
	}
}

func TestInvokeLimiterMaxZeroIsUnlimitedAndStillCountsRunningJobs(t *testing.T) {
	limiter := newInvokeLimiter(0)
	first, acquired := limiter.TryAcquire()
	if !acquired {
		t.Fatal("first unlimited slot was not acquired")
	}
	second, acquired := limiter.TryAcquire()
	if !acquired {
		t.Fatal("second unlimited slot was not acquired")
	}
	if limiter.activeCount() != 2 || limiter.capacity() != 0 {
		t.Fatalf("unlimited limiter snapshot = active %d capacity %d", limiter.activeCount(), limiter.capacity())
	}
	first()
	second()
	if limiter.activeCount() != 0 {
		t.Fatalf("active count after releases = %d, want 0", limiter.activeCount())
	}
}

func TestEffectiveMaxRunningJobsEnvOverride(t *testing.T) {
	tests := []struct {
		name       string
		env        *string
		configured uint32
		want       uint32
		wantError  bool
	}{
		{name: "unset uses config", configured: 3, want: 3},
		{name: "positive overrides", env: stringPointer("5"), configured: 3, want: 5},
		{name: "zero means unlimited", env: stringPointer("0"), configured: 3, want: 0},
		{name: "surrounding whitespace is accepted", env: stringPointer(" 4 "), configured: 3, want: 4},
		{name: "negative is rejected", env: stringPointer("-1"), configured: 3, wantError: true},
		{name: "non numeric is rejected", env: stringPointer("many"), configured: 3, wantError: true},
		{name: "overflow is rejected", env: stringPointer("4294967296"), configured: 3, wantError: true},
		{name: "explicit empty is rejected", env: stringPointer(""), configured: 3, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setOptionalEnvironment(t, envScanNodeMaxParallel, tt.env)
			got, err := effectiveMaxRunningJobs(tt.configured)
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), envScanNodeMaxParallel) {
					t.Fatalf("effective max error = %v, want %s validation error", err, envScanNodeMaxParallel)
				}
				return
			}
			if err != nil {
				t.Fatalf("effective max error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("effective max = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewScanNodeRejectsInvalidMaxParallelOverride(t *testing.T) {
	t.Setenv(envScanNodeMaxParallel, "invalid")
	if _, err := NewScanNode(node.BaseConfig{}); err == nil ||
		!strings.Contains(err.Error(), envScanNodeMaxParallel) {
		t.Fatalf("NewScanNode error = %v, want %s validation error", err, envScanNodeMaxParallel)
	}
}

func stringPointer(value string) *string {
	return &value
}

func setOptionalEnvironment(t *testing.T, name string, value *string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
			return
		}
		_ = os.Unsetenv(name)
	})
	if value == nil {
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		return
	}
	if err := os.Setenv(name, *value); err != nil {
		t.Fatalf("set %s: %v", name, err)
	}
}

func TestSSARuntimeDBIsolationFollowsEffectiveCapacity(t *testing.T) {
	tests := []struct {
		maximum uint32
		want    bool
	}{
		{maximum: 0, want: true},
		{maximum: 1, want: false},
		{maximum: 2, want: true},
	}
	for _, tt := range tests {
		node := &ScanNode{invokeLimiter: newInvokeLimiter(tt.maximum)}
		if got := node.needIsolateSSARuntimeDB(); got != tt.want {
			t.Fatalf("max=%d isolation=%t, want %t", tt.maximum, got, tt.want)
		}
	}
}

func TestScanNodeSnapshotUsesSlotCountAndEffectiveMaximum(t *testing.T) {
	limiter := newInvokeLimiter(4)
	release, acquired := limiter.TryAcquire()
	if !acquired {
		t.Fatal("slot was not acquired")
	}
	defer release()
	node := &ScanNode{
		manager:        newTaskManager(),
		invokeLimiter:  limiter,
		maxRunningJobs: 4,
	}
	snapshot := node.Snapshot()
	if snapshot.RunningJobs != 1 || snapshot.MaxRunningJobs != 4 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
