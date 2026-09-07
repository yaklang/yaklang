package scannode

import (
	"github.com/yaklang/yaklang/common/node"
	"sync"
	"testing"
)

func TestResourcePolicyShrinksAdmissionWithoutCancellingReservations(t *testing.T) {
	collector, err := newRuntimeHostResourceCollector(runtimeHostResourceSourceStub{cpus: 4, total: 8192, available: 4096}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &ScanNode{hostResources: collector, invokeLimiter: newInvokeLimiter(4), manager: newTaskManager(), maxRunningJobs: 4}
	s.invokeLimiter.SetResourceCapacity(4000, 8192)
	release1, ok := s.invokeLimiter.TryAcquireResources(1000, 1024)
	if !ok {
		t.Fatal("first admission failed")
	}
	defer release1()
	release2, ok := s.invokeLimiter.TryAcquireResources(1000, 1024)
	if !ok {
		t.Fatal("second admission failed")
	}
	defer release2()
	if err := s.ApplyResourcePolicy(node.ResourcePolicy{SystemReservedCPUMillicores: 3000, SystemReservedMemoryBytes: 7168, MaxRunningJobs: 1}); err != nil {
		t.Fatal(err)
	}
	if s.invokeLimiter.activeCount() != 2 {
		t.Fatal("running jobs changed")
	}
	if _, ok := s.invokeLimiter.TryAcquire(); ok {
		t.Fatal("new slot admitted above reduced maximum")
	}
	snapshot := s.Snapshot()
	if snapshot.MaxRunningJobs != 1 || snapshot.RuntimeHostCapacity.CPUAllocatableMillicores != 1000 || snapshot.RuntimeHostCapacity.MemoryAllocatableBytes != 1024 {
		t.Fatalf("snapshot=%+v capacity=%+v", snapshot, snapshot.RuntimeHostCapacity)
	}
	if err := s.ApplyResourcePolicy(node.ResourcePolicy{SystemReservedCPUMillicores: 4000}); err == nil {
		t.Fatal("accepted exhausted CPU")
	}
	if err := s.ApplyResourcePolicy(node.ResourcePolicy{SystemReservedMemoryBytes: 8192}); err == nil {
		t.Fatal("accepted exhausted memory")
	}
	if s.Snapshot().MaxRunningJobs != 1 {
		t.Fatal("invalid policy changed maximum")
	}
	release1()
	release2()
	if err := s.ApplyResourcePolicy(node.ResourcePolicy{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		release, ok := s.invokeLimiter.TryAcquire()
		if !ok {
			t.Fatal("zero maximum must be unlimited")
		}
		defer release()
	}
	if s.Snapshot().MaxRunningJobs != 0 || s.Snapshot().RuntimeHostCapacity.CPUAllocatableMillicores != 4000 {
		t.Fatal("explicit zero not applied")
	}
}

func TestResourcePolicyConcurrentSnapshotsAndAdmission(t *testing.T) {
	collector, err := newRuntimeHostResourceCollector(runtimeHostResourceSourceStub{cpus: 4, total: 8192, available: 4096}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &ScanNode{hostResources: collector, invokeLimiter: newInvokeLimiter(4), manager: newTaskManager()}
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				switch worker {
				case 0:
					if err := s.ApplyResourcePolicy(node.ResourcePolicy{SystemReservedCPUMillicores: uint64(i%2) * 1000, MaxRunningJobs: uint32(i % 3)}); err != nil {
						t.Error(err)
					}
				case 1:
					_ = s.Snapshot()
					_, _ = collector.Snapshot()
					_ = collector.ValidateEnvelope(1000, 1024)
				case 2:
					if release, ok := s.invokeLimiter.TryAcquireResources(1000, 1024); ok {
						release()
					}
					_ = s.invokeLimiter.capacity()
				}
			}
		}(worker)
	}
	wg.Wait()
	if s.invokeLimiter.activeCount() != 0 {
		t.Fatal("leaked active reservations")
	}
}
