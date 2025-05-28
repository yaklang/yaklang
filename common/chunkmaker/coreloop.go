package chunkmaker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/chanx" // Import chanx
)

func (cm *ChunkMaker) loop(i chan struct{}) {
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
	processAndLinkChunks := func(tempOutputChan *chanx.UnlimitedChan[Chunk]) {
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

	flushBufferToTempChannel := func(flushAllData bool) {
		// Create a temporary channel for Flush...To methods
		// The context for this temp channel should ideally be derived from cm.config.ctx
		// or a new one if appropriate for its lifecycle.
		tempDst := chanx.NewUnlimitedChan[Chunk](cm.config.ctx, 100) // Buffer size can be adjusted

		go func() { // Run flushing in a new goroutine to avoid deadlocks
			defer tempDst.Close() // Close tempDst when flushing is done
			if flushAllData {
				bufferChunk.FlushAllChunkSizeTo(tempDst, cm.config.chunkSize)
			} else {
				bufferChunk.FlushFullChunkSizeTo(tempDst, cm.config.chunkSize)
			}
		}()
		processAndLinkChunks(tempDst)
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
					flushBufferToTempChannel(true)
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
