package utils

import "io"

// BytesStripReader 是一个流式过滤器: 在 Read 过程中逐块剔除指定的字节,
// 不全量缓冲整个流, 其余字节的相对顺序保持不变。
// 典型用法: 去掉流中的换行和制表符 NewBytesStripReader(r, '\n', '\r', '\t')。
type BytesStripReader struct {
	src  io.Reader
	skip [256]bool
}

// NewBytesStripReader 包装一个 io.Reader, 流式剔除 chars 中指定的所有字节。
func NewBytesStripReader(r io.Reader, chars ...byte) *BytesStripReader {
	s := &BytesStripReader{src: r}
	for _, c := range chars {
		s.skip[c] = true
	}
	return s
}

// Read implements the io.Reader interface for BytesStripReader.
func (r *BytesStripReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		n, err := r.src.Read(p)
		kept := 0
		for _, c := range p[:n] {
			if r.skip[c] {
				continue
			}
			p[kept] = c
			kept++
		}
		// Filter in the caller's buffer so data and its error stay together.
		// Only keep reading when actual input was consumed and fully stripped;
		// a source returning (0, nil) must not cause an internal busy loop.
		if kept > 0 || err != nil || n == 0 {
			return kept, err
		}
	}
}
