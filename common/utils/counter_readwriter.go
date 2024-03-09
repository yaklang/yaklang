package utils

import "io"

// CountingReader 是一个包装了 io.Reader 的结构体，用于统计读取的字符数
type CountingReader struct {
	r     io.Reader // 底层的 io.Reader
	count int       // 读取的字符数
}

// NewCountingReader 返回一个新的 CountingReader，它包装了给定的 io.Reader
func NewCountingReader(r io.Reader) *CountingReader {
	return &CountingReader{r: r}
}

// Read 实现了 io.Reader 接口，读取数据的同时统计字符数
func (cr *CountingReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	cr.count += n // 累加读取的字符数
	return
}

// Count 返回到目前为止读取的字符数
func (cr *CountingReader) Count() int {
	return cr.count
}

// CountingWriter 是一个包装了 io.Writer 的结构体，用于统计写入的字符数
type CountingWriter struct {
	w     io.Writer // 底层的 io.Writer
	count int       // 写入的字符数
}

// NewCountingWriter 返回一个新的 CountingWriter，它包装了给定的 io.Writer
func NewCountingWriter(w io.Writer) *CountingWriter {
	return &CountingWriter{w: w}
}

// Write 实现了 io.Writer 接口，写入数据的同时统计字符数
func (cw *CountingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.count += n // 累加写入的字符数
	return
}

// Count 返回到目前为止写入的字符数
func (cw *CountingWriter) Count() int {
	return cw.count
}

// CountingReadWriter 是一个包装了 io.Writer 的结构体，用于统计写入的字符数
type CountingReadWriter struct {
	w     io.ReadWriter // 底层的 io.Writer
	count int           // 写入的字符数
}

// NewCountingReadWriter 返回一个新的 CountingWriter，它包装了给定的 io.Writer
func NewCountingReadWriter(w io.ReadWriter) *CountingReadWriter {
	return &CountingReadWriter{w: w}
}

// Write 实现了 io.Writer 接口，写入数据的同时统计字符数
func (cw *CountingReadWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.count += n // 累加写入的字符数
	return
}

// Read 实现了 io.Reader 接口，读取数据的同时统计字符数
func (cr *CountingReadWriter) Read(p []byte) (n int, err error) {
	n, err = cr.w.Read(p)
	cr.count += n // 累加读取的字符数
	return
}

// Count 返回到目前为止写入的字符数
func (cw *CountingReadWriter) Count() int {
	return cw.count
}
