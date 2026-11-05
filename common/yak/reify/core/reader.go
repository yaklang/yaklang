package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// ByteReader is the per-input byte view shared by loaders and parsers. It
// bounds every access to the declared input size and applies one byte order to
// multi-byte integer reads. Create one reader per input; call WithOrder after
// the container header determines the target byte order.
type ByteReader struct {
	name  string
	src   io.ReaderAt
	base  uint64
	size  uint64
	order ByteOrder
	owned bool
}

func NewByteReader(name string, src io.ReaderAt, size int64) (*ByteReader, error) {
	if name == "" || src == nil {
		return nil, fmt.Errorf("byte reader requires a name and source")
	}
	if size < 0 {
		return nil, fmt.Errorf("byte reader size must be non-negative")
	}
	return &ByteReader{name: name, src: src, size: uint64(size), order: ByteOrderUnknown}, nil
}

func (r *ByteReader) Name() string { return r.name }

func (r *ByteReader) Size() int64 { return int64(r.size) }

// Kind reports the byte order applied to multi-byte reads.
func (r *ByteReader) Kind() ByteOrder { return r.order }

// Source exposes the borrowed source for format parsers. It does not transfer
// ownership; Artifact.Close only closes sources that implement io.Closer.
func (r *ByteReader) Source() io.ReaderAt { return r.src }

// WithOrder returns a view that shares the source and size. The receiver keeps
// its own order, so a raw view can coexist with an ordered view.
func (r *ByteReader) WithOrder(order ByteOrder) (*ByteReader, error) {
	if order != LittleEndian && order != BigEndian {
		return nil, fmt.Errorf("invalid byte order: %q", order)
	}
	view := *r
	view.order = order
	return &view, nil
}

// Slice returns a borrowed view of [offset, offset+size). The view keeps the
// parent's source and byte order. All reads on the view use offsets relative to
// the slice, and the view never closes the source.
func (r *ByteReader) Slice(offset, size uint64) (*ByteReader, error) {
	if r == nil {
		return nil, fmt.Errorf("byte reader is nil")
	}
	if err := r.CheckRange(offset, size); err != nil {
		return nil, err
	}
	view := *r
	view.base += offset
	view.size = size
	view.owned = false
	return &view, nil
}

// CheckRange verifies that [offset, offset+size) lies within the input.
func (r *ByteReader) CheckRange(offset, size uint64) error {
	if offset > r.size || size > r.size-offset {
		return fmt.Errorf("range at 0x%x of 0x%x bytes exceeds input size 0x%x", offset, size, r.size)
	}
	return nil
}

// WithOwner marks the reader as owning the source. Catalog.OpenFile uses this
// for files it opened; loaders never set it for borrowed inputs.
func (r *ByteReader) WithOwner() *ByteReader {
	view := *r
	view.owned = true
	return &view
}

// Close closes the source only when this reader owns it.
func (r *ByteReader) Close() error {
	if r == nil || !r.owned {
		return nil
	}
	if closer, ok := r.src.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// ReadAt implements io.ReaderAt with the declared size as a hard boundary.
func (r *ByteReader) ReadAt(p []byte, off int64) (int, error) {
	if r == nil {
		return 0, fmt.Errorf("byte reader is nil")
	}
	if off < 0 {
		return 0, fmt.Errorf("negative read offset: %d", off)
	}
	if uint64(off) >= r.size {
		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	absolute := r.base + uint64(off)
	remain := r.size - uint64(off)
	if uint64(len(p)) > remain {
		n, err := r.src.ReadAt(p[:int(remain)], int64(absolute))
		if err == nil || err == io.EOF {
			err = io.EOF
		}
		return n, err
	}
	n, err := r.src.ReadAt(p, int64(absolute))
	if err == nil && n < len(p) {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// Bytes returns a copy of the requested range.
func (r *ByteReader) Bytes(offset, size uint64) ([]byte, error) {
	if err := r.CheckRange(offset, size); err != nil {
		return nil, err
	}
	if size > uint64(math.MaxInt) {
		return nil, fmt.Errorf("requested range is too large")
	}
	raw := make([]byte, int(size))
	if size == 0 {
		return raw, nil
	}
	n, err := r.src.ReadAt(raw, int64(r.base+offset))
	if err != nil && !(err == io.EOF && n == len(raw)) {
		return nil, err
	}
	if n != len(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	return raw, nil
}

func (r *ByteReader) Uint8(offset uint64) (uint8, error) {
	raw, err := r.Bytes(offset, 1)
	if err != nil {
		return 0, err
	}
	return raw[0], nil
}

func (r *ByteReader) Uint16(offset uint64) (uint16, error) {
	raw, err := r.Bytes(offset, 2)
	if err != nil {
		return 0, err
	}
	order, err := r.binaryOrder()
	if err != nil {
		return 0, err
	}
	return order.Uint16(raw), nil
}

func (r *ByteReader) Uint32(offset uint64) (uint32, error) {
	raw, err := r.Bytes(offset, 4)
	if err != nil {
		return 0, err
	}
	order, err := r.binaryOrder()
	if err != nil {
		return 0, err
	}
	return order.Uint32(raw), nil
}

func (r *ByteReader) Uint64(offset uint64) (uint64, error) {
	raw, err := r.Bytes(offset, 8)
	if err != nil {
		return 0, err
	}
	order, err := r.binaryOrder()
	if err != nil {
		return 0, err
	}
	return order.Uint64(raw), nil
}

func (r *ByteReader) binaryOrder() (binary.ByteOrder, error) {
	switch r.order {
	case LittleEndian:
		return binary.LittleEndian, nil
	case BigEndian:
		return binary.BigEndian, nil
	default:
		return nil, fmt.Errorf("byte order is not set")
	}
}
