package utils

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
