package utils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"sync"
)

type Result[O any] struct {
	Index  int
	Output O
	Err    error
}

type ParallelProcessConfig struct {
	Concurrency    int
	StartCallback  func()
	FinishCallback func()

	StartTask func()
	DeferTask func()
}

func newParallelProcessConfig(opts ...ParallelProcessOption) *ParallelProcessConfig {
	cfg := &ParallelProcessConfig{
		Concurrency:    20, // 默认并发数
		StartCallback:  func() {},
		FinishCallback: func() {},
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

type ParallelProcessOption func(*ParallelProcessConfig)

func WithParallelProcessConcurrency(concurrency int) ParallelProcessOption {
	return func(cfg *ParallelProcessConfig) {
		if concurrency > 0 {
			cfg.Concurrency = concurrency
		}
	}
}

func WithParallelProcessStartTask(h func()) ParallelProcessOption {
	return func(cfg *ParallelProcessConfig) {
		cfg.StartTask = h
	}
}

func WithParallelProcessDeferTask(h func()) ParallelProcessOption {
	return func(cfg *ParallelProcessConfig) {
		cfg.DeferTask = h
	}
}

func WithParallelProcessStartCallback(callback func()) ParallelProcessOption {
	return func(cfg *ParallelProcessConfig) {
		if callback != nil {
			cfg.StartCallback = callback
		}
	}
}

func WithParallelProcessFinishCallback(callback func()) ParallelProcessOption {
	return func(cfg *ParallelProcessConfig) {
		if callback != nil {
			cfg.FinishCallback = callback
		}
	}
}

func OrderedParallelProcess[I any, O any](
	ctx context.Context,
	inputCh <-chan I,
	processFunc func(I) (O, error),
	opts ...ParallelProcessOption,
) <-chan Result[O] {
	resultsCh := chanx.NewUnlimitedChan[Result[O]](ctx, 100)
	finalOutputCh := chanx.NewUnlimitedChan[Result[O]](ctx, 100)

	config := newParallelProcessConfig(opts...)
	swg := NewSizedWaitGroup(config.Concurrency)
	startOnce := sync.Once{}
	go func() {
		index := 0
		for data := range inputCh {
			startOnce.Do(config.StartCallback)
			swg.Add(1)
			currentIndex := index
			if config.StartTask != nil {
				config.StartTask()
			}
			go func() {
				defer swg.Done()
				defer func() {
					if config.DeferTask != nil {
						config.DeferTask()
					}
				}()
				output, err := processFunc(data)
				// 将处理结果（包含原始索引）发送到 resultsCh
				resultsCh.SafeFeed(Result[O]{
					Index:  currentIndex,
					Output: output,
					Err:    err,
				})
			}()
			index++
		}
		swg.Wait()
		resultsCh.Close()
	}()

	go func() {
		defer finalOutputCh.Close()
		defer func() {
			if config.FinishCallback != nil {
				config.FinishCallback()
			}
		}()
		buffer := make(map[int]Result[O])
		nextIndex := 0

		for result := range resultsCh.OutputChannel() {
			buffer[result.Index] = result
			for {
				res, ok := buffer[nextIndex]
				if !ok {
					break
				}
				finalOutputCh.SafeFeed(res)
				delete(buffer, nextIndex)
				nextIndex++
			}
		}

		if len(buffer) > 0 {
			log.Errorf("Some results were not processed in order, remaining items: %v", buffer)
		}
	}()
	return finalOutputCh.OutputChannel()
}

func OrderedParallelProcessSkipError[I any, O any](
	ctx context.Context,
	inputCh <-chan I,
	processFunc func(I) (O, error),
	opts ...ParallelProcessOption,
) <-chan O {
	outputChan := chanx.NewUnlimitedChan[O](ctx, 100)
	go func() {
		defer outputChan.Close()
		for res := range OrderedParallelProcess[I, O](ctx, inputCh, processFunc, opts...) {
			if res.Err != nil {
				log.Errorf("Error processing item at index %d: %v", res.Index, res.Err)
				continue
			}
			yield := res.Output
			outputChan.SafeFeed(yield)
		}
	}()
	return outputChan.OutputChannel()
}
