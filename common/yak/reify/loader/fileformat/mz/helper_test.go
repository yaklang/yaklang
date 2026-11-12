package mz

import "io"

// byteReaderAt adapts a byte slice to io.ReaderAt.
func byteReaderAt(data []byte) io.ReaderAt { return sliceAt(data) }

type sliceAt []byte

func (r sliceAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
