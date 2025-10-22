package asyncdb

import (
	"context"
	"sync"
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

	wg     *sync.WaitGroup
	ctx    context.Context    // Context for cancellation
	cancel context.CancelFunc // Function to cancel the context
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

	save := func(ts []T) {
		if len(ts) == 0 {
			return
		}
		// start := time.Now()
		batchSave(ts, s.saveToDB, s.wg)
		// log.Debugf("Databasex Channel: Save Count in save Loop: %s: need: %v, handled: %v, cost: %v",
		// 	s.config.name,
		// 	s.buffer.Len(),
		// 	len(ts),
		// 	time.Since(start),
		// )
	}

	currentSaveSize := s.config.saveSize
	items := make([]T, 0, currentSaveSize)
	for {
		saveSize := s.config.saveSize
		select {
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
				// Reset the timer since we just saved
				timer.Reset(saveTime)
			}
		case <-timer.C:
			// Time's up, save whatever we have
			save(items)
			items = make([]T, 0, saveSize)
			timer.Reset(saveTime)
		}
	}
}

// Save adds an item to the buffer for saving.
// It will be processed by the background goroutine.
func (s *Save[T]) Save(item T) {
	defer func() {
		if r := recover(); r != nil {
			utils.Errorf("Save item panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	if !utils.IsNil(item) {
		start := time.Now()
		s.buffer.FeedBlock(item)
		if time.Since(start) > time.Second {
			log.Errorf("Databasex Channel: Save Count in Save: %s: item(%v) took too long to save, cost: %v",
				s.config.name, item, time.Since(start),
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
}

func batchSave[T any](data []T, handler func([]T), wg *sync.WaitGroup) {
	if len(data) == 0 {
		return
	}

	wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("Databasex Channel: batchSave panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		defer wg.Done()
		handler(data)
	}()

	// size := defaultBatchSize
	// for i := 0; i < len(data); i += size {
	// 	end := i + size
	// 	if end > len(data) {
	// 		end = len(data)
	// 	}
	// 	wg.Add(1)
	// 	go func(chunk []T) {
	// 		defer wg.Done()
	// 		handler(chunk)
	// 	}(data[i:end])
	// }
}
