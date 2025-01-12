package rwendpoint

import "io"

type WireGuardReadWriteCloser interface {
	Read(bufs [][]byte, sizes []int, offset int) (int, error)
	Write(bufs [][]byte, size int) (int, error)
	Close() error
}

type wireGuardReadWriterWrapper struct {
	mtu    uint32
	offset int

	rw WireGuardReadWriteCloser
}

func NewWireGuardReadWriteCloserWrapper(rw WireGuardReadWriteCloser, mtu uint32, offset int) io.ReadWriteCloser {
	return &wireGuardReadWriterWrapper{
		rw:     rw,
		mtu:    mtu,
		offset: offset,
	}
}

func (t *wireGuardReadWriterWrapper) Read(packet []byte) (n int, err error) {
	bufs := [][]byte{packet}
	sizes := []int{0}
	_, err = t.rw.Read(bufs, sizes, t.offset)
	if err != nil {
		return 0, err
	}
	return sizes[0], nil
}

func (t *wireGuardReadWriterWrapper) Write(packet []byte) (n int, err error) {
	bufs := [][]byte{packet}
	_, err = t.rw.Write(bufs, t.offset)
	if err != nil {
		return 0, err
	}
	return len(packet), nil
}

func (t *wireGuardReadWriterWrapper) Close() error {
	return t.rw.Close()
}
