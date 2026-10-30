//go:build hids && linux

package enrich

import (
	"context"
	"io"
	"sync"
)

type artifactContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r artifactContextReader) Read(buf []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(buf)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}

type artifactContextReaderAt struct {
	ctx       context.Context
	reader    io.ReaderAt
	mu        sync.Mutex
	remaining int64
}

func (r *artifactContextReaderAt) ReadAt(buf []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(buf)) > r.remaining {
		return 0, ErrArtifactMetadataLimit
	}
	n, err := r.reader.ReadAt(buf, offset)
	r.remaining -= int64(n)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}
