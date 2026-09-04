package utils

import "io"

// BytesStripReader 是一个流式过滤器: 在 Read 过程中逐块剔除指定的字节,
// 不全量缓冲整个流, 其余字节的相对顺序保持不变。
// 典型用法: 去掉流中的换行和制表符 NewBytesStripReader(r, '\n', '\r', '\t')。
type BytesStripReader struct {
	src   io.Reader
	skip  [256]bool
	chunk [4096]byte
	start int
	end   int
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
	for r.start >= r.end {
		n, err := r.src.Read(r.chunk[:])
		r.start, r.end = 0, 0
		for _, c := range r.chunk[:n] {
			if r.skip[c] {
				continue
			}
			r.chunk[r.end] = c
			r.end++
		}
		if r.end > 0 {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	copied := copy(p, r.chunk[r.start:r.end])
	r.start += copied
	return copied, nil
}
