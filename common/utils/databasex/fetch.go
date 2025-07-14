package databasex

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Fetch[T any] struct {
	fetchFromDB func(context.Context, int) <-chan T
	buffer      *chanx.UnlimitedChan[T]
	cfg         *config
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewFetch[T any](
	fetchFromDB func(context.Context, int) <-chan T,
	opt ...Option,
) *Fetch[T] {
	cfg := NewConfig(opt...)
	return NewFetchWithConfig(fetchFromDB, cfg)
}
func NewFetchWithConfig[T any](
	fetchFromDB func(context.Context, int) <-chan T,
	cfg *config,
) *Fetch[T] {
	if utils.IsNil(fetchFromDB) {
		return nil
	}
	ctx, cancel := context.WithCancel(cfg.ctx)
	f := &Fetch[T]{
		fetchFromDB: fetchFromDB,
		buffer:      chanx.NewUnlimitedChan[T](cfg.ctx, cfg.bufferSize),
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
	defer func() {
		if r := recover(); r != nil {
			log.Debugf("Databasex Channel: Fetch panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// fetchSize := f.cfg.bufferSize
	currentFetchSize := f.cfg.fetchSize
	for {
		fetchSize := f.cfg.fetchSize
		select {
		case <-f.ctx.Done():
			return
		default:
			if f.buffer.Len() > currentFetchSize*5 {
				time.Sleep(100 * time.Millisecond) // Sleep for a short duration to avoid busy waiting
				continue
			}

			bufferWeight := 1
			bufferLen := f.buffer.Len()
			if bufferLen < fetchSize/2 { // [:0.5]
				bufferWeight = 10
			} else if bufferLen < fetchSize { // [0.5, 1]
				bufferWeight = 5
			}

			currentFetchSize = fetchSize * bufferWeight

			// start := time.Now()
			// _ = start
			// log.Debugf("Databasex Channel: Fetch Count Start in fetch buffer %s: buffer(%v|%v) with fetchItem(%v)", f.cfg.name, bufferLen, bufferWeight, currentFetchSize)
			var ch <-chan T
			ch = f.fetchFromDB(f.ctx, currentFetchSize)
			for item := range ch {
				if utils.IsNil(item) {
					log.Errorf("BUG: item is nil in Fetch.fillBuffer")
					continue
				}
				f.buffer.SafeFeed(item)
			}
			// log.Debugf("Databasex Channel: Fetch Count in fetch buffer %s:  fetchItem(%v): Time(%v)", f.cfg.name, f.buffer.Len(), time.Since(start))
		}
	}
}

func (f *Fetch[T]) Fetch() (T, error) {
	// start := time.Now()
	if f.buffer.Len() == 0 {
		log.Debugf("Databasex Channel: Fetch buffer is empty %s: buffer(%v) with fetchItem", f.cfg.name, f.buffer.Len())
	}
	item := <-f.buffer.OutputChannel()
	// since := time.Since(start)
	// if since > time.Second {
	// 	log.Debugf("Databasex Channel: Fetch too long time  %s: buffer(%v) with fetchItem: Time(%v)", f.cfg.name, f.buffer.Len(), since)
	// }
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
	items := make([]T, 0, f.buffer.Len())

	// drain the rest of the buffer
	if len(delete) > 0 {
		for {
			item, ok := <-f.buffer.OutputChannel()
			if !ok {
				break
			}
			items = append(items, item)
		}
	}
	// log.Debugf("Databasex Channel: Fetch Close: %s, items: %d", f.cfg.name, len(items))
	delete[0](items)
}
