package chunkmaker

import (
	"fmt"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type ChunkMaker struct {
	_loopOnce sync.Once

	config *Config
	src    *chanx.UnlimitedChan[Chunk]
	dst    *chanx.UnlimitedChan[Chunk]
}

func NewChunkMakerEx(
	input *chanx.UnlimitedChan[Chunk],
	c *Config,
) (*ChunkMaker, error) {
	cm := &ChunkMaker{
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

func NewChunkMaker(dst io.Reader, opts ...Option) (*ChunkMaker, error) {
	c := NewConfig(opts...)
	if c.ChunkSize <= 0 {
		return nil, fmt.Errorf("NewChunkMaker: ChunkSize must be positive, got %d", c.ChunkSize)
	}
	inputSrc := NewChunkChannelFromReader(c.ctx, dst)
	return NewChunkMakerEx(inputSrc, c)
}

func (cm *ChunkMaker) Write(p []byte) (n int, err error) {
	cm.src.SafeFeed(NewBufferChunk(p))
	return len(p), nil
}

func (cm *ChunkMaker) Close() error {
	cm.src.Close()
	return nil
}

func (cm *ChunkMaker) OutputChannel() <-chan Chunk {
	return cm.dst.OutputChannel()
}
