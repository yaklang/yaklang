package chunkmaker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
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

	var timer *time.Ticker
	if cm.config.enableTimeTrigger {
		timer = time.NewTicker(cm.config.timeTriggerInterval)
	} else {
		timer = time.NewTicker(time.Second)
	}

	flushAll := func() {
		bufferChunk.FlushAllChunkSizeTo(cm.dst, cm.config.ChunkSize)
	}

	for {
		closeOnce.Do(func() {
			close(i)
		})
		select {
		case <-timer.C:
			log.Infof("time trigger, current buffer size: %d", bufferChunk.BytesSize())
			if !cm.config.enableTimeTrigger {
				continue
			}
			flushAll()
		case result, ok := <-inputChan:
			if !ok {
				log.Infof("start to flush all chunks, current buffer size: %d", bufferChunk.BytesSize())
				flushAll()
				return
			}
			bufferChunk.Write(result.Data())
			log.Infof("start to write %v", spew.Sdump(string(result.Data())))
			bufferChunk.FlushFullChunkSizeTo(cm.dst, cm.config.ChunkSize)
		}
	}
}
