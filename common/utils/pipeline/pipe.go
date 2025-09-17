package pipeline

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Pipe[T, U any] struct {
	ctx     context.Context
	in      *chanx.UnlimitedChan[T]
	out     *chanx.UnlimitedChan[U]
	err     error
	swg     *utils.SizedWaitGroup
	handler func(item T) (U, error)
}

func NewSimplePipe[T, U any](ctx context.Context, in <-chan T, handler func(item T) (U, error)) *Pipe[T, U] {
	ret := &Pipe[T, U]{
		ctx:     ctx,
		out:     chanx.NewUnlimitedChan[U](ctx, defaultPipeSize),
		handler: handler,
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
				outResult, err := ret.handler(item)
				if err != nil {
					log.Errorf("failed to handle item '%s': %v", item, err)
					return
				}
				ret.out.SafeFeed(outResult)
			}
		}
	}()
	return ret
}

const defaultPipeSize = 200

func NewPipe[T, U any](
	ctx context.Context,
	initBufSize int,
	handler func(item T) (U, error),
	concurrency ...int,
) *Pipe[T, U] {
	pipeSize := defaultPipeSize
	if initBufSize > 0 {
		pipeSize = initBufSize
	}
	ret := &Pipe[T, U]{
		ctx:     ctx,
		in:      chanx.NewUnlimitedChan[T](ctx, pipeSize),
		out:     chanx.NewUnlimitedChan[U](ctx, pipeSize),
		handler: handler,
	}
	var con int
	if len(concurrency) > 0 && concurrency[0] > 0 {
		con = concurrency[0]
	} else {
		con = 10 // 默认并发10
	}
	ret.swg = utils.NewSizedWaitGroup(con)
	ret.swg.Add(1)
	go func() {
		defer ret.swg.Done()
		ret.process()
	}()

	return ret
}

func (p *Pipe[T, U]) FeedSlice(items []T) {
	go func() {
		defer p.Close()
		for _, item := range items {
			p.in.SafeFeed(item)
		}
	}()
}

func (p *Pipe[T, U]) FeedChannel(ch <-chan T) {
	go func() {
		defer p.Close()
		for item := range ch {
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

func (p *Pipe[T, U]) process() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.in.OutputChannel():
			if !ok {
				return
			}
			_ = item

			p.swg.Add(1)
			go func(t T) {
				defer p.swg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Pipe process panic: %v", r)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				if result, err := p.handler(t); err == nil {
					p.out.SafeFeed(result)
				} else {
					p.err = utils.JoinErrors(p.err, err)
				}
			}(item)
		}
	}
}

func (p *Pipe[T, U]) Close() {
	p.in.Close()
	p.swg.Wait() // wait for all processing goroutines to finish
	p.out.Close()
}

func (p *Pipe[T, U]) Error() error {
	return p.err
}
