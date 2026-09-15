package scannode

import (
	"fmt"
	"github.com/yaklang/yaklang/common/node"
)

// ApplyResourcePolicy serializes heartbeat snapshots with a complete policy
// replacement. Holding the collector and admission locks prevents either
// runtime from observing partially updated admission ceilings.
func (s *ScanNode) ApplyResourcePolicy(policy node.ResourcePolicy) error {
	s.resourcePolicyMu.Lock()
	defer s.resourcePolicyMu.Unlock()
	c := s.hostResources
	if c == nil || s.invokeLimiter == nil {
		return fmt.Errorf("host resource policy unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	oldCPU, oldMemory := c.reservedCPUMillicores, c.reservedMemoryBytes
	c.reservedCPUMillicores, c.reservedMemoryBytes = policy.SystemReservedCPUMillicores, policy.SystemReservedMemoryBytes
	capacity, err := c.snapshotLocked(false)
	if err != nil {
		c.reservedCPUMillicores, c.reservedMemoryBytes = oldCPU, oldMemory
		return err
	}
	l := s.invokeLimiter
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maximum = policy.MaxRunningJobs
	l.cpuCapacityMillicores = capacity.CPUAllocatableMillicores
	l.memoryCapacityBytes = capacity.MemoryAllocatableBytes
	s.maxRunningJobs = policy.MaxRunningJobs
	return nil
}
