package aireactdeps

import (
	"io"
	"sync"
)

// ClosableReader 可关闭的Reader包装器 (保留兼容性)
type ClosableReader struct {
	reader io.Reader
	closed chan struct{}
	mu     sync.RWMutex
}

// NewClosableReader 创建新的可关闭Reader
func NewClosableReader(reader io.Reader) *ClosableReader {
	return &ClosableReader{
		reader: reader,
		closed: make(chan struct{}),
	}
}

// Read 实现io.Reader接口
func (cr *ClosableReader) Read(p []byte) (n int, err error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	select {
	case <-cr.closed:
		return 0, io.EOF
	default:
		return cr.reader.Read(p)
	}
}

// Close 关闭Reader
func (cr *ClosableReader) Close() error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	select {
	case <-cr.closed:
		// 已经关闭
	default:
		close(cr.closed)
	}
	return nil
}

// IsClosed 检查是否已关闭
func (cr *ClosableReader) IsClosed() bool {
	select {
	case <-cr.closed:
		return true
	default:
		return false
	}
}
