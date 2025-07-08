package databasex

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Fetch[T any] struct {
	fetchFromDB func() []T
	buffer      *chanx.UnlimitedChan[T]
	cfg         *config
	wg          sync.WaitGroup
	size        int
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewFetch[T any](
	fetchFromDB func() []T,
	opt ...Option,
) *Fetch[T] {
	cfg := NewConfig(opt...)
	return NewFetchWithConfig(fetchFromDB, cfg)
}
func NewFetchWithConfig[T any](
	fetchFromDB func() []T,
	cfg *config,
) *Fetch[T] {
	ctx, cancel := context.WithCancel(cfg.ctx)
	f := &Fetch[T]{
		fetchFromDB: fetchFromDB,
		buffer:      chanx.NewUnlimitedChan[T](cfg.ctx, cfg.bufferSize),
		size:        cfg.bufferSize,
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		wg:          sync.WaitGroup{},
	}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.fillBuffer()
	}()
	return f
}

func (f *Fetch[T]) fillBuffer() {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
			items := f.fetchFromDB()
			log.Errorf("Fetch Count in fetch buffer : %v with fetchItem: %v",
				f.buffer.Len(),
				len(items),
			)
			log.Errorf("Fetch %s len: %d", f.cfg.name, len(items))
			for index, item := range items {
				_ = index
				if utils.IsNil(item) {
					log.Errorf("BUG: item is nil in Fetch.fillBuffer")
					continue
				}
				f.buffer.SafeFeed(item)
			}
		}
	}
}

func (f *Fetch[T]) Fetch() (T, error) {
	var zero T
	if f.buffer.Len() == 0 {
		log.Errorf("Fetch size length %T: len: %d", zero, f.buffer.Len())
	}

	item := <-f.buffer.OutputChannel()
	if utils.IsNil(item) {
		return item, utils.Errorf("item is nil in Fetch.Fetch")
	}
	return item, nil
}

// Close stops the background goroutine and closes the buffer channel.
func (f *Fetch[T]) Close(delete ...func([]T)) {
	// stop the background goroutine
	f.cancel()
	f.wg.Wait()

	// close the buffer channel
	f.buffer.Close()

	// drain the rest of the buffer
	if len(delete) > 0 {
		items := make([]T, 0, f.buffer.Len())
		for {
			item, ok := <-f.buffer.OutputChannel()
			if !ok {
				break
			}
			items = append(items, item)
		}
		delete[0](items)
	}
}
