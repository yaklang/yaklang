package pipeline

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Pipe[T, U any] struct {
	ctx     context.Context
	in      *chanx.UnlimitedChan[T]
	out     *chanx.UnlimitedChan[U]
	feedWG  sync.WaitGroup
	wg      sync.WaitGroup
	handler func(item T) (U, error)
}

const defaultPipeSize = 200

func NewPipe[T, U any](ctx context.Context, initBufSize int, handler func(item T) (U, error)) *Pipe[T, U] {
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

	ret.wg.Add(1)
	go func() {
		defer ret.wg.Done()
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

			p.wg.Add(1)
			go func(t T) {
				defer p.wg.Done()
				if result, err := p.handler(t); err == nil {
					p.out.SafeFeed(result)
				}
			}(item)
		}
	}
}

func (p *Pipe[T, U]) Close() {
	p.in.Close()
	p.wg.Wait() // wait for all processing goroutines to finish
	p.out.Close()
}
