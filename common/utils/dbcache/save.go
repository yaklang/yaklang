package dbcache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

// Save provides a way to collect items and save them in batches using a background goroutine.
// It buffers items and periodically passes them to a save function for processing.
type Save[T any] struct {
	saveToDB func([]T) // Function to save items to the database

	buffer *chanx.UnlimitedChan[T] // Channel for buffering items

	config *config

	wg      *sync.WaitGroup
	saveWG  *sync.WaitGroup
	saveSem chan struct{}
	ctx     context.Context    // Context for cancellation
	cancel  context.CancelFunc // Function to cancel the context

	metrics saveMetrics
}

type SaveStats struct {
	Pending           int64
	MaxPending        int64
	BatchCount        uint64
	BatchItemsTotal   uint64
	MaxBatchSize      int64
	EnqueueBlockTotal time.Duration
	MaxEnqueueBlock   time.Duration
	SaveTimeTotal     time.Duration
	MaxSaveTime       time.Duration
}

func (s SaveStats) AvgBatchSize() float64 {
	if s.BatchCount == 0 {
		return 0
	}
	return float64(s.BatchItemsTotal) / float64(s.BatchCount)
}

type saveMetrics struct {
	pending           atomic.Int64
	maxPending        atomic.Int64
	batchCount        atomic.Uint64
	batchItemsTotal   atomic.Uint64
	maxBatchSize      atomic.Int64
	enqueueBlockTotal atomic.Int64
	maxEnqueueBlock   atomic.Int64
	saveTimeTotal     atomic.Int64
	maxSaveTime       atomic.Int64
}

func (m *saveMetrics) recordEnqueue(blockCost time.Duration) {
	pending := m.pending.Add(1)
	updateAtomicMaxInt64(&m.maxPending, pending)
	if blockCost > 0 {
		m.enqueueBlockTotal.Add(int64(blockCost))
		updateAtomicMaxInt64(&m.maxEnqueueBlock, int64(blockCost))
	}
}

func (m *saveMetrics) recordBatch(size int, saveCost time.Duration) {
	if size <= 0 {
		return
	}
	m.pending.Add(-int64(size))
	m.batchCount.Add(1)
	m.batchItemsTotal.Add(uint64(size))
	updateAtomicMaxInt64(&m.maxBatchSize, int64(size))
	if saveCost > 0 {
		m.saveTimeTotal.Add(int64(saveCost))
		updateAtomicMaxInt64(&m.maxSaveTime, int64(saveCost))
	}
}

func (m *saveMetrics) snapshot() SaveStats {
	return SaveStats{
		Pending:           m.pending.Load(),
		MaxPending:        m.maxPending.Load(),
		BatchCount:        m.batchCount.Load(),
		BatchItemsTotal:   m.batchItemsTotal.Load(),
		MaxBatchSize:      m.maxBatchSize.Load(),
		EnqueueBlockTotal: time.Duration(m.enqueueBlockTotal.Load()),
		MaxEnqueueBlock:   time.Duration(m.maxEnqueueBlock.Load()),
		SaveTimeTotal:     time.Duration(m.saveTimeTotal.Load()),
		MaxSaveTime:       time.Duration(m.maxSaveTime.Load()),
	}
}

func updateAtomicMaxInt64(target *atomic.Int64, value int64) {
	for {
		current := target.Load()
		if value <= current {
			return
		}
		if target.CompareAndSwap(current, value) {
			return
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

// NewSave creates a new Saver with the specified buffer size and save function.
// It starts a background goroutine to process items from the buffer.
func NewSave[T any](
	saveToDB func([]T),
	opt ...Option,
) *Save[T] {
	cfg := NewConfig(opt...)
	return NewSaveWithConfig(saveToDB, cfg)
}

func NewSaveWithConfig[T any](
	saveToDB func([]T),
	cfg *config,
) *Save[T] {
	if utils.IsNil(saveToDB) {
		return nil
	}
	ctx, cancel := context.WithCancel(cfg.ctx)
	s := &Save[T]{
		saveToDB: saveToDB,
		buffer:   chanx.NewUnlimitedChan[T](ctx, cfg.bufferSize),
		ctx:      ctx,
		cancel:   cancel,
		config:   cfg,
		wg:       &sync.WaitGroup{},
		saveWG:   &sync.WaitGroup{},
	}
	if cfg.saveParallelism > 1 {
		s.saveSem = make(chan struct{}, cfg.saveParallelism)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processBuffer()
	}()

	return s
}

// const SaveSize = 2000
// const SaveTime = 1 * time.Second

// processBuffer runs in a background goroutine and periodically processes items from the buffer.
func (s *Save[T]) processBuffer() {
	saveTime := s.config.saveTimeout
	timer := time.NewTimer(saveTime)
	defer timer.Stop()

	save := func(ts []T) {
		if len(ts) == 0 {
			return
		}
		s.dispatchSave(ts)
	}

	currentSaveSize := s.config.saveSize
	items := make([]T, 0, currentSaveSize)
	for {
		saveSize := s.config.saveSize
		select {
		case <-s.ctx.Done():
			save(items)
			return
		case item, ok := <-s.buffer.OutputChannel():
			if !ok {
				save(items)
				return
			}

			items = append(items, item)

			// If we've reached the SaveSize, save immediately
			if len(items) >= currentSaveSize {
				save(items)
				bufferSize := s.buffer.Len()
				if bufferSize > currentSaveSize*2 {
					currentSaveSize *= 10
				} else if bufferSize > currentSaveSize {
					currentSaveSize *= 5
				} else if bufferSize > saveSize {
					// currentSaveSize = currentSaveSize
					// pass
				} else {
					currentSaveSize = saveSize
				}

				items = make([]T, 0, currentSaveSize)
				resetTimer(timer, saveTime)
			}
		case <-timer.C:
			// Time's up, save whatever we have
			save(items)
			items = make([]T, 0, saveSize)
			resetTimer(timer, saveTime)
		}
	}
}

// Save adds an item to the buffer for saving.
// It will be processed by the background goroutine.
func (s *Save[T]) Save(item T) {
	if !utils.IsNil(item) {
		queued := false
		start := time.Now()
		func() {
			defer func() {
				if r := recover(); r != nil {
					utils.Errorf("Save item panic: %v", r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			s.buffer.FeedBlock(item)
			queued = true
		}()
		if !queued {
			return
		}
		blockCost := time.Since(start)
		s.metrics.recordEnqueue(blockCost)
		if blockCost > time.Second {
			log.Errorf("dbcache save blocked %s: item(%v) cost:%v",
				s.config.name, item, blockCost,
			)
		}
	}
}

// Close stops the background goroutine and waits for it to finish.
// It also processes any remaining items in the buffer before returning.
func (s *Save[T]) Close() {
	if s == nil {
		return
	}
	s.buffer.Close() // Close the buffer
	s.wg.Wait()      // Wait for the background goroutine to finish
	s.saveWG.Wait()
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Save[T]) Stats() SaveStats {
	if s == nil {
		return SaveStats{}
	}
	return s.metrics.snapshot()
}

func (s *Save[T]) dispatchSave(ts []T) {
	if len(ts) == 0 {
		return
	}
	if s.saveSem == nil {
		s.runSave(ts)
		return
	}

	s.saveSem <- struct{}{}
	s.saveWG.Add(1)
	go func(items []T) {
		defer func() {
			<-s.saveSem
			s.saveWG.Done()
		}()
		s.runSave(items)
	}(ts)
}

func (s *Save[T]) runSave(ts []T) {
	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("dbcache batch save panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		s.metrics.recordBatch(len(ts), time.Since(start))
	}()
	s.saveToDB(ts)
}
