package chunkmaker

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type SimpleChunkMaker struct {
	ctx    context.Context
	cancel context.CancelFunc
	dst    *chanx.UnlimitedChan[Chunk]
}

// NewSimpleStringerChunkMaker NewSimpleChunkMakerEx creates a SimpleChunkMaker from an input channel of fmt.Stringer.
// It reads from the input channel, converts each fmt.Stringer to a BufferChunk,
// is Simple , config chunk size or separator is not used.
func NewSimpleStringerChunkMaker(src chan fmt.Stringer, opts ...Option) (*SimpleChunkMaker, error) {
	cfg := NewConfig(opts...)

	ctx, cancel := cfg.ctx, cfg.cancel
	dst := chanx.NewUnlimitedChan[Chunk](ctx, 1000)
	go func() {
		defer dst.Close()
		var preChunk *BufferChunk
		for {
			select {
			case simpleData, ok := <-src:
				if !ok {
					log.Info("SimpleChunkMaker closed")
					return
				}
				currentChunk := NewBufferChunk([]byte(simpleData.String()))
				currentChunk.prev = preChunk
				dst.SafeFeed(currentChunk)
				preChunk = currentChunk
			case <-ctx.Done():
				return
			}
		}
	}()
	return &SimpleChunkMaker{
		ctx:    ctx,
		cancel: cancel,
		dst:    dst,
	}, nil
}

func NewSimpleChunkMaker[T any](src <-chan T, handle func(T) Chunk, opts ...Option) (*SimpleChunkMaker, error) {
	cfg := NewConfig(opts...)

	ctx, cancel := cfg.ctx, cfg.cancel
	dst := chanx.NewUnlimitedChan[Chunk](ctx, 1000)
	go func() {
		defer dst.Close()
		var preChunk Chunk
		for {
			select {
			case ch, ok := <-src:
				if !ok {
					log.Info("SimpleChunkMaker closed")
					return
				}
				currentChunk := handle(ch)
				currentChunk.SetPreviousChunk(preChunk)
				dst.SafeFeed(currentChunk)
				preChunk = currentChunk
			case <-ctx.Done():
				return
			}
		}
	}()
	return &SimpleChunkMaker{
		ctx:    ctx,
		cancel: cancel,
		dst:    dst,
	}, nil
}

func (i *SimpleChunkMaker) Close() error {
	if i.cancel != nil {
		i.cancel()
	}
	return nil
}

func (i *SimpleChunkMaker) OutputChannel() <-chan Chunk {
	return i.dst.OutputChannel()
}
