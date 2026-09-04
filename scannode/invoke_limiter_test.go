package scannode

import (
	"os"
	"strings"
	"sync"
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

func TestInvokeLimiterResourceAdmissionIsAtomicAndReleaseIsIdempotent(t *testing.T) {
	limiter := newInvokeLimiter(4)
	limiter.SetResourceCapacity(2000, 4<<30)

	first, acquired := limiter.TryAcquireResources(1500, 3<<30)
	if !acquired {
		t.Fatal("first resource request was not acquired")
	}
	if _, acquired := limiter.TryAcquireResources(1000, 2<<30); acquired {
		t.Fatal("resource-overcommitting request was acquired")
	}
	first()
	first()
	second, acquired := limiter.TryAcquireResources(1000, 2<<30)
	if !acquired {
		t.Fatal("resource request was not admitted after idempotent release")
	}
	second()
}

func TestInvokeLimiterConcurrentResourceAdmissionDoesNotOvercommit(t *testing.T) {
	limiter := newInvokeLimiter(4)
	limiter.SetResourceCapacity(1000, 1<<30)
	start := make(chan struct{})
	type result struct {
		release  func()
		acquired bool
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			release, acquired := limiter.TryAcquireResources(1000, 1<<30)
			results <- result{release: release, acquired: acquired}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	acquired := 0
	for item := range results {
		if !item.acquired {
			continue
		}
		acquired++
		item.release()
	}
	if acquired != 1 {
		t.Fatalf("concurrent resource admissions = %d, want exactly 1", acquired)
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

func TestScanNodeSnapshotReportsHostCapacityWithoutAIRuntimeHost(t *testing.T) {
	collector, err := newRuntimeHostResourceCollector(
		runtimeHostResourceSourceStub{cpus: 8, total: 16 << 30, available: 12 << 30},
		500,
		1<<30,
	)
	if err != nil {
		t.Fatalf("new resource collector: %v", err)
	}
	limiter := newInvokeLimiter(4)
	node := &ScanNode{
		manager:        newTaskManager(),
		invokeLimiter:  limiter,
		maxRunningJobs: 4,
		hostResources:  collector,
	}

	snapshot := node.Snapshot()
	capacity := snapshot.RuntimeHostCapacity
	if capacity == nil || capacity.CPUAllocatableMillicores != 7500 ||
		capacity.MemoryAllocatableBytes != 15<<30 || capacity.SampleSequence != 1 {
		t.Fatalf("unexpected host capacity: %#v", capacity)
	}
	if _, acquired := limiter.TryAcquireResources(8000, 2<<30); acquired {
		t.Fatal("local limiter ignored the reported allocatable CPU")
	}
}
