package jsonextractor

import (
	"fmt"
	"io"
)

// autoPeekReader 是一个可以预读的 Reader
type autoPeekReader struct {
	reader    io.Reader // 底层 reader
	buffer    []byte    // 预读缓冲区
	bufferPos int       // 缓冲区当前位置
}

// newAutoPeekReader 创建一个新的 autoPeekReader
func newAutoPeekReader(r io.Reader) *autoPeekReader {
	pr := &autoPeekReader{
		reader:    r,
		buffer:    make([]byte, 0),
		bufferPos: 0,
	}
	// 初始化时预读一个字节
	pr.ensureBuffer(1)
	return pr
}

// ensureBuffer 确保缓冲区至少有 n 个字节可读
func (pr *autoPeekReader) ensureBuffer(n int) error {
	available := len(pr.buffer) - pr.bufferPos
	if available >= n {
		return nil
	}

	// 需要读取更多数据
	needed := n - available
	buf := make([]byte, needed)
	read, err := pr.reader.Read(buf)
	if read > 0 {
		pr.buffer = append(pr.buffer, buf[:read]...)
	}
	return err
}

// Read 实现 io.Reader 接口
func (pr *autoPeekReader) Read(p []byte) (n int, err error) {
	// 先用缓冲区的数据
	if pr.bufferPos < len(pr.buffer) {
		n = copy(p, pr.buffer[pr.bufferPos:])
		pr.bufferPos += n

		// 尝试预读，但忽略错误
		pr.ensureBuffer(1)
		return n, nil
	}

	// 缓冲区已用完，直接从底层读取
	pr.buffer = nil
	pr.bufferPos = 0
	n, err = pr.reader.Read(p)

	// 如果成功读取，尝试预读下一个字节
	if n > 0 {
		pr.ensureBuffer(1)
	}

	return n, err
}

// ReadByte 读取并返回下一个字节
func (pr *autoPeekReader) ReadByte() (byte, error) {
	// 确保缓冲区至少有一个字节
	if err := pr.ensureBuffer(1); err != nil && len(pr.buffer) <= pr.bufferPos {
		return 0, err
	}

	// 没有足够的字节
	if pr.bufferPos >= len(pr.buffer) {
		return 0, io.EOF
	}

	// 读取当前字节
	b := pr.buffer[pr.bufferPos]
	pr.bufferPos++

	// 预读下一个字节
	pr.ensureBuffer(1)

	return b, nil
}

// Peek 返回下一个字节但不消费
func (pr *autoPeekReader) Peek() (byte, error) {
	res, err := pr.PeekN(1)
	if err != nil {
		return 0, err
	}
	if len(res) < 1 {
		return 0, fmt.Errorf("invalid peek , want %d but got %d", 1, len(res))
	}
	return res[0], nil
}

// Peek 返回下n个字节但不消费
func (pr *autoPeekReader) PeekN(n int) ([]byte, error) {
	if err := pr.ensureBuffer(n); err != nil && len(pr.buffer) <= pr.bufferPos {
		return nil, err
	}

	if pr.bufferPos >= len(pr.buffer) {
		return nil, io.EOF
	}

	return pr.buffer[pr.bufferPos : pr.bufferPos+n], nil
}

// Buffered 返回缓冲区中当前可读的所有内容
func (pr *autoPeekReader) Buffered() []byte {
	if pr.bufferPos >= len(pr.buffer) {
		return []byte{}
	}
	return pr.buffer[pr.bufferPos:]
}
