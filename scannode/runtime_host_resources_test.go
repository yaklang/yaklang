package scannode

import (
	"errors"
	"testing"
)

type runtimeHostResourceSourceStub struct {
	cpus      uint64
	total     uint64
	available uint64
	err       error
}

func (s runtimeHostResourceSourceStub) Snapshot() (uint64, uint64, uint64, error) {
	return s.cpus, s.total, s.available, s.err
}

func TestRuntimeHostResourceCollectorAppliesSystemReserve(t *testing.T) {
	t.Parallel()

	collector, err := newRuntimeHostResourceCollector(
		runtimeHostResourceSourceStub{cpus: 8, total: 16 << 30, available: 12 << 30},
		500,
		1<<30,
	)
	if err != nil {
		t.Fatalf("newRuntimeHostResourceCollector() error = %v", err)
	}
	first, err := collector.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	second, err := collector.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() second error = %v", err)
	}
	if first.Scope != "host" || first.CPUCapacityMillicores != 8000 ||
		first.CPUAllocatableMillicores != 7500 ||
		first.MemoryCapacityBytes != 16<<30 ||
		first.MemoryAllocatableBytes != 15<<30 ||
		first.MemoryAvailableBytes != 12<<30 {
		t.Fatalf("Snapshot() = %+v", first)
	}
	if first.SampleSequence != 1 || second.SampleSequence != 2 {
		t.Fatalf("sample sequences = %d, %d", first.SampleSequence, second.SampleSequence)
	}
	if err := collector.ValidateEnvelope(7500, 15<<30); err != nil {
		t.Fatalf("ValidateEnvelope() exact fit = %v", err)
	}
	if err := collector.ValidateEnvelope(7501, 1); err == nil {
		t.Fatal("ValidateEnvelope() accepted CPU above allocatable")
	}
}

func TestRuntimeHostResourceCollectorFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      runtimeHostResourceSource
		reservedCPU uint64
		reservedMem uint64
	}{
		{name: "source failure", source: runtimeHostResourceSourceStub{err: errors.New("unavailable")}},
		{name: "cpu reserve exhausts host", source: runtimeHostResourceSourceStub{cpus: 1, total: 2 << 30}, reservedCPU: 1000},
		{name: "memory reserve exhausts host", source: runtimeHostResourceSourceStub{cpus: 2, total: 1 << 30}, reservedMem: 1 << 30},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := newRuntimeHostResourceCollector(test.source, test.reservedCPU, test.reservedMem); err == nil {
				t.Fatal("expected collector construction to fail")
			}
		})
	}
}

func TestRuntimeHostResourceCollectorClampsAvailableMemory(t *testing.T) {
	t.Parallel()

	collector, err := newRuntimeHostResourceCollector(
		runtimeHostResourceSourceStub{cpus: 2, total: 2 << 30, available: 3 << 30},
		0,
		0,
	)
	if err != nil {
		t.Fatalf("newRuntimeHostResourceCollector() error = %v", err)
	}
	capacity, err := collector.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if capacity.MemoryAvailableBytes != capacity.MemoryCapacityBytes {
		t.Fatalf("available memory was not clamped: %+v", capacity)
	}
}
