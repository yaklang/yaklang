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
	fetchFromDB func(int) []T
	buffer      *chanx.UnlimitedChan[T]
	cfg         *config
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewFetch[T any](
	fetchFromDB func(int) []T,
	opt ...Option,
) *Fetch[T] {
	cfg := NewConfig(opt...)
	return NewFetchWithConfig(fetchFromDB, cfg)
}
func NewFetchWithConfig[T any](
	fetchFromDB func(int) []T,
	cfg *config,
) *Fetch[T] {
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
	// fetchSize := f.cfg.bufferSize
	currentFetchSize := f.cfg.fetchSize
	prevSendBuffer := 0
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
			if bufferLen < fetchSize/2 || prevSendBuffer < fetchSize/2 { // [:0.5]
				bufferWeight = 10
			} else if bufferLen < fetchSize || prevSendBuffer < fetchSize { // [0.5, 1]
				bufferWeight = 5
			} else if prevSendBuffer > bufferLen && prevSendBuffer-bufferLen > fetchSize/2 {
				bufferWeight = 5
			}

			currentFetchSize = fetchSize * bufferWeight

			items := f.fetchFromDB(currentFetchSize)

			log.Errorf(
				"Databasex Channel: Fetch Count in fetch buffer %s: buffer(%v|%v) prevBuf(%v) with fetchItem(%v): %v", f.cfg.name,
				bufferLen, bufferWeight, prevSendBuffer,
				currentFetchSize, len(items),
			)

			prevSendBuffer = f.buffer.Len()

			for _, item := range items {
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
		log.Errorf("Databasex Channel: Fetch size length %T: len: %d ", zero, f.buffer.Len())
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
