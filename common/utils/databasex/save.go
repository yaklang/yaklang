package databasex

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
)

// Save provides a way to collect items and save them in batches using a background goroutine.
// It buffers items and periodically passes them to a save function for processing.
type Save[T any] struct {
	saveToDB func([]T) // Function to save items to the database

	buffer *chanx.UnlimitedChan[T] // Channel for buffering items

	config *config

	wg     sync.WaitGroup
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
		log.Infof("Databasex Channel: Save Count in save Loop: %s: need: %v, handled: %v",
			s.config.name,
			s.buffer.Len(),
			len(ts),
		)

		f1 := func() {
			if len(ts) > 0 {
				s.saveToDB(ts)
			}
		}
		ssaprofile.ProfileAdd(true, "save.Save", f1)
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
				if bufferSize > currentSaveSize {
					currentSaveSize = (bufferSize / currentSaveSize) * currentSaveSize
				} else if bufferSize > saveSize {
					currentSaveSize = (bufferSize / saveSize) * saveSize
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
		s.buffer.SafeFeed(item)
	}
}

// Close stops the background goroutine and waits for it to finish.
// It also processes any remaining items in the buffer before returning.
func (s *Save[T]) Close() {
	s.buffer.Close() // Close the buffer
	s.wg.Wait()      // Wait for the background goroutine to finish
}
