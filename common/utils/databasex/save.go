package databasex

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
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
	saveSize := s.config.saveSize
	saveTime := s.config.saveTimeout
	timer := time.NewTimer(saveTime)
	items := make([]T, 0, saveSize)
	save := func() {
		if len(items) > 0 {
			s.saveToDB(items)
			items = make([]T, 0, saveSize) // Reset the items slice
		}
	}

	for {
		select {
		case item, ok := <-s.buffer.OutputChannel():
			if !ok {
				save()
				return
			}

			items = append(items, item)

			// If we've reached the SaveSize, save immediately
			if len(items) >= saveSize {
				save()
				// Reset the timer since we just saved
				timer.Reset(saveTime)
			}
		case <-timer.C:
			// Time's up, save whatever we have
			save()
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

const MaxSize = 300

// Close stops the background goroutine and waits for it to finish.
// It also processes any remaining items in the buffer before returning.
func (s *Save[T]) Close() {
	s.buffer.Close() // Close the buffer
	s.wg.Wait()      // Wait for the background goroutine to finish
}
