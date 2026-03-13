package scannode

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

const (
	envScanNodeMaxParallel = "SCANNODE_MAX_PARALLEL"
)

type invokeLimiter struct {
	total  chan struct{}
	totalN int
}

func (l *invokeLimiter) activeCount() int {
	if l == nil || l.total == nil {
		return 0
	}
	return len(l.total)
}

func (l *invokeLimiter) capacity() int {
	if l == nil {
		return 0
	}
	return l.totalN
}

func (l *invokeLimiter) acquire(ctx context.Context) (release func(), err error) {
	if l == nil {
		return func() {}, nil
	}
	if err := acquireToken(ctx, l.total); err != nil {
		return nil, err
	}
	return func() {
		releaseToken(l.total)
	}, nil
}

func acquireToken(ctx context.Context, ch chan struct{}) error {
	if ch == nil {
		return nil
	}
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseToken(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
	}
}

func readIntEnv(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func (s *ScanNode) initInvokeLimiter() {
	totalN := readIntEnv(envScanNodeMaxParallel, 1)
	s.invokeLimiter = &invokeLimiter{
		total:  make(chan struct{}, totalN),
		totalN: totalN,
	}
	log.Infof("invoke limiter initialized: total=%d", totalN)
}
