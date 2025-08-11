package chunkmaker

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type TextChunkMaker struct {
	_loopOnce sync.Once

	config *Config
	src    *chanx.UnlimitedChan[Chunk]
	dst    *chanx.UnlimitedChan[Chunk]
}

func NewTextChunkMakerEx(
	input *chanx.UnlimitedChan[Chunk],
	c *Config,
) (*TextChunkMaker, error) {
	cm := &TextChunkMaker{
		src:    input,
		dst:    chanx.NewUnlimitedChan[Chunk](c.ctx, 1000),
		config: c,
	}

	syncChan := make(chan struct{})
	cm._loopOnce.Do(func() {
		go cm.loop(syncChan)
	})
	select {
	case _, ok := <-syncChan:
		if !ok {
			log.Debug("syncChan passed")
		}
	}
	return cm, nil
}

func NewTextChunkMaker(dst io.Reader, opts ...Option) (*TextChunkMaker, error) {
	c := NewConfig(opts...)
	if c.chunkSize <= 0 {
		return nil, fmt.Errorf("NewTextChunkMaker: ChunkSize must be positive, got %d", c.chunkSize)
	}
	if c.enableTimeTrigger && c.timeTriggerInterval <= 0 {
		return nil, fmt.Errorf("NewTextChunkMaker: timeTriggerInterval must be positive when time trigger is enabled, got %v", c.timeTriggerInterval)
	}

	if c.chunkSize <= 0 && !c.enableTimeTrigger {
		return nil, fmt.Errorf("NewTextChunkMaker: ChunkSize must be positive or time trigger must be enabled")
	}
	var rc io.ReadCloser
	r, ok := dst.(io.ReadCloser)
	if ok {
		rc = r
	} else {
		rc = io.NopCloser(dst)
	}
	inputSrc := NewChunkChannelFromReader(c.ctx, rc)
	return NewTextChunkMakerEx(inputSrc, c)
}

func (cm *TextChunkMaker) Write(p []byte) (n int, err error) {
	cm.src.SafeFeed(NewBufferChunk(p))
	return len(p), nil
}

func (cm *TextChunkMaker) Close() error {
	cm.src.Close()
	return nil
}

func (cm *TextChunkMaker) OutputChannel() <-chan Chunk {
	return cm.dst.OutputChannel()
}

func (cm *TextChunkMaker) loop(i chan struct{}) {
	closeOnce := sync.Once{}
	src := cm.src
	inputChan := src.OutputChannel()
	bufferChunk := NewBufferChunk(nil) // This buffer accumulates data from inputChan

	var lastSentChunk Chunk // Variable to keep track of the last chunk sent to cm.dst

	defer func() {
		cm.dst.Close()
	}()

	bufferSize := new(int64)
	delta := func(i int64) int64 {
		return atomic.AddInt64(bufferSize, i)
	}
	currentBuffer := func() int64 {
		return atomic.LoadInt64(bufferSize)
	}
	_ = delta
	_ = currentBuffer
	// _ = bufferChunk // bufferChunk is used

	var timerChan <-chan time.Time
	if cm.config.enableTimeTrigger {
		ticker := time.NewTicker(cm.config.timeTriggerInterval)
		defer ticker.Stop()
		timerChan = ticker.C
	}

	// Helper function to process and link chunks from a temporary channel
	processAndLinkChunks := func(tempOutputChan *chanx.UnlimitedChan[Chunk], haveTheLastChunk bool) {
		for chunkToLink := range tempOutputChan.OutputChannel() {
			if bc, ok := chunkToLink.(*BufferChunk); ok {
				bc.prev = lastSentChunk
				cm.dst.SafeFeed(bc)
				lastSentChunk = bc
			} else {
				// Handle case where chunkToLink is not *BufferChunk, though unlikely
				// based on current NewBufferChunk usage. For safety, send as is or log.
				cm.dst.SafeFeed(chunkToLink)
				lastSentChunk = chunkToLink
			}
		}
	}

	flushBufferToTempChannelEx := func(flushAllData bool, haveTheLastChunk bool) {
		// Create a temporary channel for Flush...To methods
		// The context for this temp channel should ideally be derived from cm.config.ctx
		// or a new one if appropriate for its lifecycle.
		tempDst := chanx.NewUnlimitedChan[Chunk](cm.config.ctx, 100) // Buffer size can be adjusted

		go func() { // Run flushing in a new goroutine to avoid deadlocks
			defer tempDst.Close() // Close tempDst when flushing is done
			if flushAllData {
				bufferChunk.FlushAllChunkSizeTo(tempDst, cm.config.chunkSize, cm.config.separator, haveTheLastChunk)
			} else {
				bufferChunk.FlushFullChunkSizeTo(tempDst, cm.config.chunkSize, cm.config.separator)
			}
		}()
		processAndLinkChunks(tempDst, haveTheLastChunk)
	}
	flushBufferToTempChannel := func(flushAllData bool) {
		flushBufferToTempChannelEx(flushAllData, false)
	}

	for {
		closeOnce.Do(func() {
			close(i)
		})
		select {
		case <-timerChan:
			if bufferChunk.BytesSize() > 0 { // Only flush if there's data
				flushBufferToTempChannel(true) // Time trigger should flush all remaining data
			}
		case result, ok := <-inputChan:
			if !ok {
				if bufferChunk.BytesSize() > 0 { // Flush any remaining data on close
					flushBufferToTempChannelEx(true, true)
				}
				return
			}
			// It's assumed result itself doesn't need linking here,
			// as it's raw data to be added to bufferChunk.
			// If result itself is a Chunk that needs linking, the logic would differ.
			bufferChunk.Write(result.Data())
			flushBufferToTempChannel(false) // Flush only full chunks after new data
		}
	}
}
