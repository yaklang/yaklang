package chunkmaker

import (
	"sync"
	"sync/atomic"
	"time"
)

func (cm *ChunkMaker) loop(i chan struct{}) {
	closeOnce := sync.Once{}
	src := cm.src
	inputChan := src.OutputChannel()
	bufferChunk := NewBufferChunk(nil)

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
	_ = bufferChunk

	var timerChan <-chan time.Time // Use a nil channel if timer is not enabled
	if cm.config.enableTimeTrigger {
		ticker := time.NewTicker(cm.config.timeTriggerInterval)
		defer ticker.Stop() // Ensure ticker is stopped
		timerChan = ticker.C
	}

	flushAll := func() {
		bufferChunk.FlushAllChunkSizeTo(cm.dst, cm.config.chunkSize)
	}

	for {
		closeOnce.Do(func() {
			close(i)
		})
		select {
		case <-timerChan: // This will block indefinitely if timerChan is nil
			// log.Infof("time trigger, current buffer size: %d", bufferChunk.BytesSize())
			// No need to check cm.config.enableTimeTrigger here, as timerChan would be nil if not enabled
			flushAll()
		case result, ok := <-inputChan:
			if !ok {
				// log.Infof("start to flush all chunks, current buffer size: %d", bufferChunk.BytesSize())
				flushAll()
				return
			}
			bufferChunk.Write(result.Data())
			// log.Infof("start to write %v", spew.Sdump(string(result.Data())))
			bufferChunk.FlushFullChunkSizeTo(cm.dst, cm.config.chunkSize)
		}
	}
}
