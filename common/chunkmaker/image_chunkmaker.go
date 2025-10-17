package chunkmaker

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/imageutils"
)

func NewImageChunkMakerFromFile(targetFile string, opts ...Option) (*SimpleChunkMaker, error) {
	cfg := NewConfig(opts...)
	return NewImageChunkMakerFromFileEx(targetFile, cfg)
}

func NewImageChunkMakerFromFileEx(targetFile string, cfg *Config) (*SimpleChunkMaker, error) {
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
				currentChunk := NewBufferChunkEx(img.RawImage, img.MIMEType, "")
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
