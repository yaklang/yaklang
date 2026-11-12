package arm

import "io"

// bytesReader adapts a byte slice to io.ReaderAt for the Thumb tests.
func bytesReader(data []byte) io.ReaderAt { return sliceReader(data) }

type sliceReader []byte

func (r sliceReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
