package chunkmaker

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"sync"
)

type MergerChunkMaker struct {
	ctx    context.Context
	cancel context.CancelFunc

	dst *chanx.UnlimitedChan[Chunk]
	wg  sync.WaitGroup
}

func (m *MergerChunkMaker) AddInput(input <-chan Chunk) {
	if m.ctx.Err() != nil {
		return
	}
	m.wg.Add(1)
	go m.forward(input)
}

func (m *MergerChunkMaker) forward(input <-chan Chunk) {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case val, ok := <-input:
			if !ok {
				return
			}
			m.dst.SafeFeed(val)
		}
	}
}

func (m *MergerChunkMaker) OutputChannel() <-chan Chunk {
	return m.dst.OutputChannel()
}

func (m *MergerChunkMaker) Close() error {
	m.cancel()
	m.wg.Wait()
	m.dst.Close()
	return nil
}

func NewMergerChunkMaker(ctx context.Context) *MergerChunkMaker {
	subCtx, cancel := context.WithCancel(ctx)

	return &MergerChunkMaker{
		ctx:    subCtx,
		cancel: cancel,
		dst:    chanx.NewUnlimitedChan[Chunk](subCtx, 1000),
	}
}
