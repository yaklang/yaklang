package scannode

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/yaklang/yaklang/common/node"
)

type runtimeHostResourceSource interface {
	Snapshot() (logicalCPUs uint64, memoryCapacityBytes uint64, memoryAvailableBytes uint64, err error)
}

type systemRuntimeHostResourceSource struct{}

func (systemRuntimeHostResourceSource) Snapshot() (uint64, uint64, uint64, error) {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, 0, err
	}
	logicalCPUs := runtime.NumCPU()
	if logicalCPUs <= 0 || memory.Total == 0 {
		return 0, 0, 0, fmt.Errorf("host CPU or memory capacity is unavailable")
	}
	return uint64(logicalCPUs), memory.Total, memory.Available, nil
}

type runtimeHostResourceCollector struct {
	source                runtimeHostResourceSource
	reservedCPUMillicores uint64
	reservedMemoryBytes   uint64
	mu                    sync.Mutex
	sampleSequence        uint64
}

func newRuntimeHostResourceCollector(
	source runtimeHostResourceSource,
	reservedCPUMillicores uint64,
	reservedMemoryBytes uint64,
) (*runtimeHostResourceCollector, error) {
	if source == nil {
		source = systemRuntimeHostResourceSource{}
	}
	collector := &runtimeHostResourceCollector{
		source:                source,
		reservedCPUMillicores: reservedCPUMillicores,
		reservedMemoryBytes:   reservedMemoryBytes,
	}
	if _, err := collector.snapshot(false); err != nil {
		return nil, fmt.Errorf("collect Runtime Host capacity: %w", err)
	}
	return collector, nil
}

func (c *runtimeHostResourceCollector) Snapshot() (node.RuntimeHostCapacity, error) {
	return c.snapshot(true)
}

func (c *runtimeHostResourceCollector) ValidateEnvelope(cpuMillicores, memoryBytes uint64) error {
	capacity, err := c.snapshot(false)
	if err != nil {
		return err
	}
	if cpuMillicores > capacity.CPUAllocatableMillicores || memoryBytes > capacity.MemoryAllocatableBytes {
		return fmt.Errorf("runtime container resource envelope exceeds host allocatable capacity")
	}
	return nil
}

func (c *runtimeHostResourceCollector) snapshot(advance bool) (node.RuntimeHostCapacity, error) {
	logicalCPUs, memoryCapacity, memoryAvailable, err := c.source.Snapshot()
	if err != nil {
		return node.RuntimeHostCapacity{}, err
	}
	cpuCapacity := logicalCPUs * 1000
	if c.reservedCPUMillicores >= cpuCapacity {
		return node.RuntimeHostCapacity{}, fmt.Errorf(
			"reserved CPU %dm must be smaller than host capacity %dm",
			c.reservedCPUMillicores, cpuCapacity,
		)
	}
	if c.reservedMemoryBytes >= memoryCapacity {
		return node.RuntimeHostCapacity{}, fmt.Errorf(
			"reserved memory %d must be smaller than host capacity %d",
			c.reservedMemoryBytes, memoryCapacity,
		)
	}
	if memoryAvailable > memoryCapacity {
		memoryAvailable = memoryCapacity
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if advance {
		c.sampleSequence++
	}
	return node.RuntimeHostCapacity{
		Scope:                    "host",
		CPUCapacityMillicores:    cpuCapacity,
		CPUAllocatableMillicores: cpuCapacity - c.reservedCPUMillicores,
		MemoryCapacityBytes:      memoryCapacity,
		MemoryAllocatableBytes:   memoryCapacity - c.reservedMemoryBytes,
		MemoryAvailableBytes:     memoryAvailable,
		SampleSequence:           c.sampleSequence,
	}, nil
}
