package utils

import (
	"context"
	"sync"
	"time"
)

type BatchProcessorConfig[T any] struct {
	size    int
	timeout int
	cb      []func([]T)
}
type BatchProcessorOption[T any] func(*BatchProcessorConfig[T])

func WithBatchProcessorSize[T any](size int) BatchProcessorOption[T] {
	return func(b *BatchProcessorConfig[T]) {
		b.size = size
	}
}
func WithBatchProcessorTimeout[T any](timeout int) BatchProcessorOption[T] {
	return func(b *BatchProcessorConfig[T]) {
		b.timeout = timeout
	}
}
func WithBatchProcessorCallBack[T any](cb func([]T)) BatchProcessorOption[T] {
	return func(b *BatchProcessorConfig[T]) {
		b.cb = append(b.cb, cb)
	}
}

type BatchProcessor[T any] struct {
	dataChannel <-chan T
	config      *BatchProcessorConfig[T]
	ctx         context.Context
	wg          *sync.WaitGroup
}

func (b *BatchProcessor[T]) Start() {
	b.wg.Add(1)
	go func() {
		ticker := time.NewTicker(time.Hour)
		if b.config.timeout > 0 {
			ticker = time.NewTicker(time.Second * time.Duration(b.config.timeout))
		}
		defer b.wg.Done()
		var batch []T
		for {
			select {
			case <-b.ctx.Done():
				//做默认处理
				for _, f := range b.config.cb {
					f(batch)
				}
				return
			case <-ticker.C:
				for _, f := range b.config.cb {
					f(batch)
				}
			case data, ok := <-b.dataChannel:
				if !ok {
					if len(batch) > 0 {
						for _, cb := range b.config.cb {
							cb(batch)
						}
					}
					return
				}
				batch = append(batch, data)
				if len(batch) >= b.config.size {
					for _, cb := range b.config.cb {
						cb(batch)
					}
					batch = []T{}
				}
			}
		}
	}()
}
func (b *BatchProcessor[T]) Wait() {
	b.wg.Wait()
}
func NewBatchProcessor[T any](ctx context.Context, dataChannel <-chan T, opts ...BatchProcessorOption[T]) *BatchProcessor[T] {
	b := &BatchProcessor[T]{
		ctx: ctx,
		config: &BatchProcessorConfig[T]{
			size: 5,
		},
		dataChannel: dataChannel,
		wg:          new(sync.WaitGroup),
	}
	for _, opt := range opts {
		opt(b.config)
	}
	return b
}
