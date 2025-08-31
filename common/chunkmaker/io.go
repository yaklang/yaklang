package chunkmaker

import (
	"context"
	"io"
	"os"

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
	// Create a copy of the data to avoid sharing the underlying buffer
	data := make([]byte, len(p))
	copy(data, p)
	chunk := NewBufferChunk(data)
	w.dst.SafeFeed(chunk)
	return len(p), nil
}

func (w *SimpleChunkWriter) Close() error {
	w.dst.Close()
	return nil
}

func NewChunkChannelFromReader(ctx context.Context, r io.ReadCloser) *chanx.UnlimitedChan[Chunk] {
	dst := chanx.NewUnlimitedChan[Chunk](ctx, 1000)
	writer := NewSimpleChunkWriter(dst)
	go func() {
		defer writer.Close()
		defer r.Close()
		io.Copy(writer, r)
	}()
	return dst
}

func NewChunkChannelFromFilename(ctx context.Context, filename string) *chanx.UnlimitedChan[Chunk] {
	fp, err := os.Open(filename)
	if err != nil {
		return nil
	}
	return NewChunkChannelFromReader(ctx, fp)
}
