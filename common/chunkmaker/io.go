package chunkmaker

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type SimpleChunkWriter struct {
	dst *chanx.UnlimitedChan[Chunk]
}

func NewSimpleChunkWriter(dst *chanx.UnlimitedChan[Chunk]) *SimpleChunkWriter {
	return &SimpleChunkWriter{
		dst: dst,
	}
}

var _ io.WriteCloser = (*SimpleChunkWriter)(nil)

func (w *SimpleChunkWriter) Write(p []byte) (n int, err error) {
	chunk := NewBufferChunk(p)
	w.dst.SafeFeed(chunk)
	return len(p), nil
}

func (w *SimpleChunkWriter) Close() error {
	w.dst.Close()
	return nil
}

func NewChunkChannelFromReader(ctx context.Context, r io.Reader) *chanx.UnlimitedChan[Chunk] {
	dst := chanx.NewUnlimitedChan[Chunk](ctx, 1000)
	writer := NewSimpleChunkWriter(dst)
	go func() {
		defer writer.Close()
		io.Copy(writer, r)
	}()
	return dst
}
