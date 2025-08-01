package chunkmaker

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/imageutils"
)

type ImageChunkMaker struct {
	ctx    context.Context
	cancel context.CancelFunc
	dst    *chanx.UnlimitedChan[Chunk]
}

func (i *ImageChunkMaker) Close() error {
	if i.cancel != nil {
		i.cancel()
	}
	return nil
}

func (i *ImageChunkMaker) OutputChannel() <-chan Chunk {
	return i.dst.OutputChannel()
}

func NewImageChunkMakerFromFile(targetFile string, opts ...Option) (*ImageChunkMaker, error) {
	cfg := NewConfig(opts...)
	return NewImageChunkMakerFromFileEx(targetFile, cfg)
}

func NewImageChunkMakerFromFileEx(targetFile string, cfg *Config) (*ImageChunkMaker, error) {
	ctx, cancel := cfg.ctx, cfg.cancel
	imageChan, err := imageutils.ExtractImageFromFile(targetFile, imageutils.WithCtx(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	dst := chanx.NewUnlimitedChan[Chunk](ctx, 1000)
	go func() {
		defer dst.Close()
		var preChunk *BufferChunk
		for {
			select {
			case img, ok := <-imageChan:
				if !ok {
					log.Info("ImageChunkMaker closed")
					return
				}
				currentChunk := NewBufferChunkWithMIMEType(img.RawImage, img.MIMEType)
				currentChunk.prev = preChunk
				dst.SafeFeed(currentChunk)
				preChunk = currentChunk
			case <-ctx.Done():
				return
			}
		}
	}()
	return &ImageChunkMaker{
		ctx:    ctx,
		cancel: cancel,
		dst:    dst,
	}, nil
}
