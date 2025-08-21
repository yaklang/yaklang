package utils

import (
	"context"
	"math"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
)

// SizedWaitGroup has the same role and close to the
// same API as the Golang sync.WaitGroup but adds a limit of
// the amount of goroutines started concurrently.
type SizedWaitGroup struct {
	ctx               context.Context
	Size              int
	WaitingEventCount atomic.Int64
	current           chan struct{}
	wg                *sync.WaitGroup
}

// New creates a SizedWaitGroup.
// The limit parameter is the maximum amount of
// goroutines which can be started concurrently.
func NewSizedWaitGroup(limit int, ctxs ...context.Context) *SizedWaitGroup {
	size := math.MaxInt32 // 2^32 - 1
	if limit > 0 {
		size = limit
	}
	s := &SizedWaitGroup{
		Size:    size,
		current: make(chan struct{}, size),
		wg:      new(sync.WaitGroup),
	}
	for _, ctx := range ctxs {
		s.SetContext(ctx)
	}

	return s
}

// SetContext sets the context for the SizedWaitGroup.
// ! If Call twice or more, any of the previous context Done will cause the WaitGroup to be SetZero.
func (s *SizedWaitGroup) SetContext(ctx context.Context) {
	s.ctx = ctx
	go func() {
		<-ctx.Done()
		s.SetZero()
	}()
}

func (s *SizedWaitGroup) SetZero() {
	s.Add(0 - int(s.WaitingEventCount.Load()))
}

// Add increments the internal WaitGroup counter.
// It can be blocking if the limit of spawned goroutines
// has been reached. It will stop blocking when Done is
// been called.
//
// See sync.WaitGroup documentation for more information.
func (s *SizedWaitGroup) Add(delta ...int) {
	n := 1
	if len(delta) > 0 {
		n = delta[0]
	}

	err := s.AddWithContext(s.ctx, n)
	if err != nil {
		return
	}
}

// AddWithContext increments the internal WaitGroup counter.
// It can be blocking if the limit of spawned goroutines
// has been reached. It will stop blocking when Done is
// been called, or when the context is canceled. Returns nil on
// success or an error if the context is canceled before the lock
// is acquired.
//
// See sync.WaitGroup documentation for more information.
func (s *SizedWaitGroup) AddWithContext(ctx context.Context, delta ...int) error {
	n := 1
	if len(delta) > 0 {
		n = delta[0]
	}
	selfCtx := s.ctx
	if selfCtx == nil {
		selfCtx = context.Background()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for i := 0; i < n; i++ {
		select {
		case <-selfCtx.Done():
			return selfCtx.Err()
		case <-ctx.Done():
			return ctx.Err()
		case s.current <- struct{}{}:
			select {
			case <-ctx.Done():
				select {
				case <-s.current:
				default:
				}
				return ctx.Err()
			default:
			}
			break
		}
	}

	s.wg.Add(n)
	s.WaitingEventCount.Add(int64(n))
	return nil
}

// Done decrements the SizedWaitGroup counter.
// See sync.WaitGroup documentation for more information.
func (s *SizedWaitGroup) Done() {
	defer func() {
		if r := recover(); r != nil {
			if errMsg := InterfaceToString(r); errMsg == "sync: negative WaitGroup counter" {
				log.Error(errMsg)
			} else {
				panic(r)
			}
		}
	}()
	<-s.current
	s.wg.Done()
	s.WaitingEventCount.Add(-1)
}

// Wait blocks until the SizedWaitGroup counter is zero.
// See sync.WaitGroup documentation for more information.
func (s *SizedWaitGroup) Wait() {
	s.wg.Wait()
}
