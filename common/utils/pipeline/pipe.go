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
	in         *chanx.UnlimitedChan[T]
	out        *chanx.UnlimitedChan[U]
	boundedIn  chan T
	boundedOut chan U
	errMu      sync.Mutex
	err        error
	swg        *utils.SizedWaitGroup
	handler    func(item T, store *utils.SafeMap[any]) (U, error)
	initWorker func() *utils.SafeMap[any]
}

func NewSimplePipe[T, U any](ctx context.Context, in <-chan T, handler func(item T) (U, error)) *Pipe[T, U] {
	ctx = normalizeContext(ctx)
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

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizePipeSize(initBufSize int) int {
	if initBufSize > 0 {
		return initBufSize
	}
	return defaultPipeSize
}

func normalizeBoundedPipeSize(bufSize int) int {
	if bufSize < 0 {
		return 0
	}
	return bufSize
}

func normalizeConcurrency(concurrency ...int) int {
	if len(concurrency) > 0 && concurrency[0] > 0 {
		return concurrency[0]
	}
	return 10
}

// NewPipe 创建一个基本的 Pipe，兼容不需要 store 的老代码
func NewPipe[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T) (U, error),
	concurrency ...int,
) *Pipe[T, U] {
	// 包装老的 handler，使其兼容新的签名
	wrappedHandler := func(item T, store *utils.SafeMap[any]) (U, error) {
		return handler(item)
	}
	return NewPipeWithInit(ctx, initBufSize, wrappedHandler, nil, concurrency...)
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
	return NewBoundedPipeWithStore(ctx, bufSize, wrappedHandler, nil, concurrency...)
}

// NewPipeWithStore 创建一个带有 worker 初始化函数的 Pipe
// initWorker 会在每个 worker 协程启动时执行一次，用于初始化协程本地存储
// handler 的第二个参数会接收到 initWorker 返回的 store
func NewPipeWithStore[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	// 包装 handler，适配可变参数签名
	return NewPipeWithInit(ctx, initBufSize, handler, initWorker, concurrency...)
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

// NewPipeWithInit 创建一个带有 worker 初始化函数的 Pipe（内部使用）
// initWorker 会在每个 worker 协程启动时执行一次，用于初始化协程本地存储
func NewPipeWithInit[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	return newPipeWithInit(ctx, initBufSize, false, handler, initWorker, concurrency...)
}

func newPipeWithInit[T, U any](
	ctx context.Context,
	initBufSize int,
	bounded bool,
	handler func(item T, store *utils.SafeMap[any]) (U, error),
	initWorker func() *utils.SafeMap[any],
	concurrency ...int,
) *Pipe[T, U] {
	ctx = normalizeContext(ctx)
	pipeSize := normalizePipeSize(initBufSize)
	ret := &Pipe[T, U]{
		ctx:        ctx,
		handler:    handler,
		initWorker: initWorker,
	}
	if bounded {
		pipeSize = normalizeBoundedPipeSize(initBufSize)
		ret.boundedIn = make(chan T, pipeSize)
		ret.boundedOut = make(chan U, pipeSize)
	} else {
		ret.in = chanx.NewUnlimitedChan[T](ctx, pipeSize)
		ret.out = chanx.NewUnlimitedChan[U](ctx, pipeSize)
	}
	con := normalizeConcurrency(concurrency...)
	ret.swg = utils.NewSizedWaitGroup(con)

	// 启动固定数量的消费者协程
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
	return false
}

func (p *Pipe[T, U]) FeedSlice(items []T) {
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

func (p *Pipe[T, U]) FeedChannel(ch <-chan T) {
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

func (p *Pipe[T, U]) Feed(item T) {
	p.safeFeedIn(item)
}

func (p *Pipe[T, U]) Out() <-chan U {
	if p.boundedOut != nil {
		return p.boundedOut
	}
	return p.out.OutputChannel()
}

func (p *Pipe[T, U]) inputChannel() <-chan T {
	if p.boundedIn != nil {
		return p.boundedIn
	}
	return p.in.OutputChannel()
}

func (p *Pipe[T, U]) safeFeedIn(item T) {
	defer func() {
		_ = recover()
	}()
	if p.boundedIn == nil {
		p.in.SafeFeed(item)
		return
	}
	select {
	case <-p.ctx.Done():
	case p.boundedIn <- item:
	}
}

func (p *Pipe[T, U]) safeFeedOut(item U) {
	defer func() {
		_ = recover()
	}()
	if p.boundedOut == nil {
		p.out.SafeFeed(item)
		return
	}
	select {
	case <-p.ctx.Done():
	case p.boundedOut <- item:
	}
}

// worker 是固定的消费者协程，持续从输入通道读取数据并处理
func (p *Pipe[T, U]) worker() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Pipe worker panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// 初始化协程本地存储
	var store *utils.SafeMap[any]
	if p.initWorker != nil {
		store = p.initWorker()
	}

	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.inputChannel():
			if !ok {
				return
			}

			var result U
			var err error

			result, err = p.handler(item, store)

			if err == nil {
				p.safeFeedOut(result)
			} else {
				p.errMu.Lock()
				p.err = utils.JoinErrors(p.err, err)
				p.errMu.Unlock()
			}
		}
	}
}

func (p *Pipe[T, U]) Close() {
	if p.boundedIn != nil {
		close(p.boundedIn)
	} else {
		p.in.Close()
	}
	p.swg.Wait() // wait for all processing goroutines to finish
	if p.boundedOut != nil {
		close(p.boundedOut)
	} else {
		p.out.Close()
	}
}

func (p *Pipe[T, U]) Error() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}
