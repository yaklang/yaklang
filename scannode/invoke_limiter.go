package scannode

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

const envScanNodeMaxParallel = "SCANNODE_MAX_PARALLEL"

// invokeLimiter is deliberately non-blocking. Slot and resource reservations
// share one mutex so a concurrent admission cannot pass one limit while
// another admission passes the other.
type invokeLimiter struct {
	mu                    sync.Mutex
	maximum               uint32
	active                uint32
	cpuCapacityMillicores uint64
	memoryCapacityBytes   uint64
	cpuReservedMillicores uint64
	memoryReservedBytes   uint64
}

func newInvokeLimiter(maximum uint32) *invokeLimiter {
	return &invokeLimiter{maximum: maximum}
}

func (l *invokeLimiter) activeCount() uint32 {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

func (l *invokeLimiter) capacity() uint32 {
	if l == nil {
		return 0
	}
	return l.maximum
}

// TryAcquire reserves capacity without turning the command consumer into a
// local waiting queue. The returned release function is safe to call more than
// once, which lets every terminal path share the same cleanup code.
func (l *invokeLimiter) TryAcquire() (release func(), acquired bool) {
	return l.TryAcquireResources(0, 0)
}

// SetResourceCapacity configures the local allocatable envelope advertised by
// this Node. Existing reservations remain accounted if the envelope shrinks.
func (l *invokeLimiter) SetResourceCapacity(cpuMillicores, memoryBytes uint64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cpuCapacityMillicores = cpuMillicores
	l.memoryCapacityBytes = memoryBytes
}

// TryAcquireResources reserves a slot and the workload admission charge as one
// operation. A zero/zero request keeps the legacy slot-only behavior.
func (l *invokeLimiter) TryAcquireResources(
	cpuMillicores uint64,
	memoryBytes uint64,
) (release func(), acquired bool) {
	if l == nil {
		return func() {}, true
	}
	resourceAware := cpuMillicores > 0 || memoryBytes > 0
	if resourceAware && (cpuMillicores == 0 || memoryBytes == 0) {
		return nil, false
	}

	l.mu.Lock()
	if l.maximum > 0 && l.active >= l.maximum {
		l.mu.Unlock()
		return nil, false
	}
	if resourceAware &&
		(!fitsLocalResource(l.cpuReservedMillicores, cpuMillicores, l.cpuCapacityMillicores) ||
			!fitsLocalResource(l.memoryReservedBytes, memoryBytes, l.memoryCapacityBytes)) {
		l.mu.Unlock()
		return nil, false
	}
	l.active++
	l.cpuReservedMillicores += cpuMillicores
	l.memoryReservedBytes += memoryBytes
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.active--
			l.cpuReservedMillicores -= cpuMillicores
			l.memoryReservedBytes -= memoryBytes
		})
	}, true
}

func fitsLocalResource(reserved, requested, capacity uint64) bool {
	return capacity > 0 && reserved <= capacity && requested <= capacity-reserved
}

func effectiveMaxRunningJobs(configured uint32) (uint32, error) {
	rawValue, configuredByEnvironment := os.LookupEnv(envScanNodeMaxParallel)
	if !configuredByEnvironment {
		return configured, nil
	}
	raw := strings.TrimSpace(rawValue)
	override, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be an integer between 0 and %d: %q",
			envScanNodeMaxParallel,
			uint64(^uint32(0)),
			rawValue,
		)
	}
	return uint32(override), nil
}

func (s *ScanNode) initInvokeLimiter(maximum uint32) {
	s.invokeLimiter = newInvokeLimiter(maximum)
	if maximum == 0 {
		log.Infof("invoke limiter initialized: total=unlimited")
		return
	}
	log.Infof("invoke limiter initialized: total=%d", maximum)
}
