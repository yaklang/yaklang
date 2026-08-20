package scannode

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
)

const envScanNodeMaxParallel = "SCANNODE_MAX_PARALLEL"

// invokeLimiter is deliberately non-blocking. A nil slots channel means that
// the Node is unbounded, while active still tracks the jobs that actually own
// an execution slot for heartbeat reporting.
type invokeLimiter struct {
	slots   chan struct{}
	maximum uint32
	active  atomic.Uint32
}

func newInvokeLimiter(maximum uint32) *invokeLimiter {
	limiter := &invokeLimiter{maximum: maximum}
	if maximum > 0 {
		limiter.slots = make(chan struct{}, int(maximum))
	}
	return limiter
}

func (l *invokeLimiter) activeCount() uint32 {
	if l == nil {
		return 0
	}
	return l.active.Load()
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
	if l == nil {
		return func() {}, true
	}
	if l.slots != nil {
		select {
		case l.slots <- struct{}{}:
		default:
			return nil, false
		}
	}
	l.active.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			l.active.Add(^uint32(0))
			if l.slots != nil {
				<-l.slots
			}
		})
	}, true
}

func effectiveMaxRunningJobs(configured uint32) uint32 {
	raw := strings.TrimSpace(os.Getenv(envScanNodeMaxParallel))
	if raw == "" {
		return configured
	}
	override, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || override == 0 {
		log.Warnf(
			"ignore invalid %s=%q; using configured max_running_jobs=%d",
			envScanNodeMaxParallel,
			raw,
			configured,
		)
		return configured
	}
	return uint32(override)
}

func (s *ScanNode) initInvokeLimiter(maximum uint32) {
	s.invokeLimiter = newInvokeLimiter(maximum)
	if maximum == 0 {
		log.Infof("invoke limiter initialized: total=unlimited")
		return
	}
	log.Infof("invoke limiter initialized: total=%d", maximum)
}
