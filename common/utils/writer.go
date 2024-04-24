package utils

import "io"

type CustomWriter struct {
	write func(p []byte) (n int, err error)
}

func (c *CustomWriter) Write(p []byte) (n int, err error) {
	return c.write(p)
}

func NewWriter(f func(p []byte) (n int, err error)) *CustomWriter {
	return &CustomWriter{
		write: f,
	}
}

type onlyWriter struct {
	io.Writer
}

func RealTimeCopy(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1)
	return io.CopyBuffer(onlyWriter{dst}, src, buf)
}
