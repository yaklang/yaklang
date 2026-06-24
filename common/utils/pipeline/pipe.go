package pipeline

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Pipe[T, U any] struct {
	ctx        context.Context
	in         chanx.PipeIO[T]
	out        chanx.PipeIO[U]
	errMu      sync.Mutex
	err        error
	swg        *utils.SizedWaitGroup
	handler    func(item T, store *utils.SafeMap[any]) (U, error)
	initWorker func() *utils.SafeMap[any]
}

func NewSimplePipe[T, U any](ctx context.Context, in <-chan T, handler func(item T) (U, error)) *Pipe[T, U] {
	if ctx == nil {
		ctx = context.Background()
	}
	ret := &Pipe[T, U]{
		ctx: ctx,
		out: chanx.NewUnlimitedChan[U](ctx, defaultPipeSize),
		handler: func(item T, store *utils.SafeMap[any]) (U, error) {
			return handler(item)
		},
	}
	go func() {
		defer ret.out.Close()
		for {
			select {
			case <-ret.ctx.Done():
				return
			case item, ok := <-in:
				if !ok {
					return
				}
				outResult, err := ret.handler(item, nil)
				if err != nil {
					log.Errorf("failed to handle item: %v", err)
					return
				}
				ret.out.SafeFeed(outResult)
			}
		}
	}()
	return ret
}

const defaultPipeSize = 200

// NewPipe 创建一个基本的 Pipe，兼容不需要 store 的老代码
func NewPipe[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T) (U, error),
	concurrency ...int,
) *Pipe[T, U] {
	wrappedHandler := func(item T, store *utils.SafeMap[any]) (U, error) {
		return handler(item)
	}
	return newPipeWithInit(ctx, initBufSize, false, wrappedHandler, nil, concurrency...)
}

// NewBoundedPipe creates a Pipe backed by fixed-size channels. It is useful for
// pipelines that carry large objects and need backpressure instead of the
// default unbounded buffering behavior. A bufSize of 0 creates unbuffered
// channels.
func NewBoundedPipe[T, U any](
	ctx context.Context,
	bufSize int,
	handler func(item T) (U, error),
	concurrency ...int,
) *Pipe[T, U] {
	wrappedHandler := func(item T, store *utils.SafeMap[any]) (U, error) {
		return handler(item)
	}
	return newPipeWithInit(ctx, bufSize, true, wrappedHandler, nil, concurrency...)
}

// NewPipeWithStore 创建一个带有 worker 初始化函数的 Pipe
func NewPipeWithStore[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	return newPipeWithInit(ctx, initBufSize, false, handler, initWorker, concurrency...)
}

// NewBoundedPipeWithStore is NewPipeWithStore plus fixed input/output channel
// capacity. Existing NewPipe* constructors intentionally keep their historical
// unbounded channel semantics. A bufSize of 0 creates unbuffered channels.
func NewBoundedPipeWithStore[T, U any](
	ctx context.Context,
	bufSize int,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	return newPipeWithInit(ctx, bufSize, true, handler, initWorker, concurrency...)
}

func newPipeWithInit[T, U any](
	ctx context.Context,
	initBufSize int,
	bounded bool,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	if ctx == nil {
		ctx = context.Background()
	}
	con := 10
	if len(concurrency) > 0 && concurrency[0] > 0 {
		con = concurrency[0]
	}
	bufSize := initBufSize
	if !bounded && bufSize <= 0 {
		bufSize = defaultPipeSize
	}
	if bounded && bufSize < 0 {
		bufSize = 0
	}

	ret := &Pipe[T, U]{
		ctx:        ctx,
		handler:    handler,
		initWorker: initWorker,
	}
	if bounded {
		ret.in = chanx.NewLimitedChan[T](ctx, bufSize)
		ret.out = chanx.NewLimitedChan[U](ctx, bufSize)
	} else {
		ret.in = chanx.NewUnlimitedChan[T](ctx, bufSize)
		ret.out = chanx.NewUnlimitedChan[U](ctx, bufSize)
	}
	ret.swg = utils.NewSizedWaitGroup(con)

	for i := 0; i < con; i++ {
		ret.swg.Add(1)
		go func() {
			defer ret.swg.Done()
			ret.worker()
		}()
	}

	return ret
}

func (p *Pipe[T, U]) IsContextCancel() bool {
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

func (p *Pipe[T, U]) FeedSlice(items []T) {
	go func() {
		defer p.Close()
		for _, item := range items {
			if p.IsContextCancel() {
				return
			}
			p.in.SafeFeed(item)
		}
	}()
}

func (p *Pipe[T, U]) FeedChannel(ch <-chan T) {
	go func() {
		defer p.Close()
		for item := range ch {
			if p.IsContextCancel() {
				return
			}
			p.in.SafeFeed(item)
		}
	}()
}

func (p *Pipe[T, U]) Feed(item T) {
	p.in.SafeFeed(item)
}

func (p *Pipe[T, U]) Out() <-chan U {
	return p.out.OutputChannel()
}

func (p *Pipe[T, U]) worker() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Pipe worker panic: %v", r)
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
		case item, ok := <-p.in.OutputChannel():
			if !ok {
				return
			}

			result, err := p.handler(item, store)
			if err == nil {
				p.out.SafeFeed(result)
			} else {
				p.errMu.Lock()
				p.err = utils.JoinErrors(p.err, err)
				p.errMu.Unlock()
			}
		}
	}
}

func (p *Pipe[T, U]) Close() {
	p.in.Close()
	p.swg.Wait()
	p.out.Close()
}

func (p *Pipe[T, U]) Error() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}
