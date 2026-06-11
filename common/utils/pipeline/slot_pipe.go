package pipeline

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type SlotResult[T any] struct {
	Value   T
	once    sync.Once
	release func()
}

func newSlotResult[T any](value T, release func()) *SlotResult[T] {
	return &SlotResult[T]{
		Value:   value,
		release: release,
	}
}

func (r *SlotResult[T]) Release() {
	if r == nil || r.release == nil {
		return
	}
	r.once.Do(r.release)
}

type SlotPipe[T, U any] struct {
	ctx        context.Context
	in         chan T
	out        chan *SlotResult[U]
	errMu      sync.Mutex
	err        error
	swg        *utils.SizedWaitGroup
	handler    func(item T, store *utils.SafeMap[any]) (U, error)
	initWorker func() *utils.SafeMap[any]
	slots      chan struct{}
}

func NewSlotPipe[T, U any](
	ctx context.Context,
	bufSize int,
	slotCount int,
	handler func(item T) (U, error),
	concurrency ...int,
) *SlotPipe[T, U] {
	wrappedHandler := func(item T, store *utils.SafeMap[any]) (U, error) {
		return handler(item)
	}
	return NewSlotPipeWithStore(ctx, bufSize, slotCount, wrappedHandler, nil, concurrency...)
}

func NewSlotPipeWithStore[T, U any](
	ctx context.Context,
	bufSize int,
	slotCount int,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *SlotPipe[T, U] {
	ctx = normalizeContext(ctx)
	con := normalizeConcurrency(concurrency...)
	if slotCount <= 0 {
		slotCount = con
	}
	if slotCount < 1 {
		slotCount = 1
	}
	bufSize = normalizeBoundedPipeSize(bufSize)
	ret := &SlotPipe[T, U]{
		ctx:        ctx,
		in:         make(chan T, bufSize),
		out:        make(chan *SlotResult[U], slotCount),
		handler:    handler,
		initWorker: initWorker,
		slots:      make(chan struct{}, slotCount),
		swg:        utils.NewSizedWaitGroup(con),
	}
	for i := 0; i < con; i++ {
		ret.swg.Add(1)
		go func() {
			defer ret.swg.Done()
			ret.worker()
		}()
	}
	return ret
}

func (p *SlotPipe[T, U]) IsContextCancel() bool {
	if p.ctx == nil {
		return false
	}
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

func (p *SlotPipe[T, U]) FeedSlice(items []T) {
	go func() {
		defer p.Close()
		for _, item := range items {
			if p.IsContextCancel() {
				return
			}
			p.safeFeedIn(item)
		}
	}()
}

func (p *SlotPipe[T, U]) FeedChannel(ch <-chan T) {
	go func() {
		defer p.Close()
		for item := range ch {
			if p.IsContextCancel() {
				return
			}
			p.safeFeedIn(item)
		}
	}()
}

func (p *SlotPipe[T, U]) Feed(item T) {
	p.safeFeedIn(item)
}

func (p *SlotPipe[T, U]) Out() <-chan *SlotResult[U] {
	return p.out
}

func (p *SlotPipe[T, U]) safeFeedIn(item T) {
	defer func() {
		_ = recover()
	}()
	select {
	case <-p.ctx.Done():
	case p.in <- item:
	}
}

func (p *SlotPipe[T, U]) safeFeedOut(item *SlotResult[U]) bool {
	defer func() {
		_ = recover()
	}()
	select {
	case <-p.ctx.Done():
		return false
	case p.out <- item:
		return true
	}
}

func (p *SlotPipe[T, U]) acquireSlot() (func(), bool) {
	select {
	case <-p.ctx.Done():
		return nil, false
	case p.slots <- struct{}{}:
		return func() {
			select {
			case <-p.slots:
			default:
			}
		}, true
	}
}

func (p *SlotPipe[T, U]) worker() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("SlotPipe worker panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	var store *utils.SafeMap[any]
	if p.initWorker != nil {
		store = p.initWorker()
	}

	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.in:
			if !ok {
				return
			}
			if !p.handle(item, store) {
				return
			}
		}
	}
}

func (p *SlotPipe[T, U]) handle(item T, store *utils.SafeMap[any]) (keepGoing bool) {
	release, ok := p.acquireSlot()
	if !ok {
		return false
	}
	releasedToResult := false
	defer func() {
		if r := recover(); r != nil {
			release()
			panic(r)
		}
		if !releasedToResult {
			release()
		}
	}()

	result, err := p.handler(item, store)
	if err != nil {
		p.errMu.Lock()
		p.err = utils.JoinErrors(p.err, err)
		p.errMu.Unlock()
		return true
	}

	slot := newSlotResult(result, release)
	releasedToResult = true
	if !p.safeFeedOut(slot) {
		slot.Release()
		return false
	}
	return true
}

func (p *SlotPipe[T, U]) Close() {
	close(p.in)
	p.swg.Wait()
	close(p.out)
}

func (p *SlotPipe[T, U]) Error() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}
