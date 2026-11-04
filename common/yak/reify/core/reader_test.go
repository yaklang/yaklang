package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

func newReader(t *testing.T, raw []byte) *ByteReader {
	t.Helper()
	reader, err := NewByteReader("fixture", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestByteReaderConstruction(t *testing.T) {
	if _, err := NewByteReader("", bytes.NewReader(nil), 0); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := NewByteReader("x", nil, 0); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := NewByteReader("x", bytes.NewReader(nil), -1); err == nil {
		t.Fatal("negative size accepted")
	}
	reader := newReader(t, []byte{1, 2, 3})
	if reader.Name() != "fixture" || reader.Size() != 3 || reader.Kind() != ByteOrderUnknown {
		t.Fatalf("unexpected reader metadata: %+v", reader)
	}
}

func TestByteReaderOrderedReads(t *testing.T) {
	raw := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	reader := newReader(t, raw)
	for _, read := range []func() error{
		func() error { _, err := reader.Uint16(0); return err },
		func() error { _, err := reader.Uint32(0); return err },
		func() error { _, err := reader.Uint64(0); return err },
	} {
		if err := read(); err == nil {
			t.Fatal("multi-byte read without byte order accepted")
		}
	}
	little, err := reader.WithOrder(LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := little.Uint16(1); err != nil || value != 0x0302 {
		t.Fatalf("little u16: %#x %v", value, err)
	}
	if value, err := little.Uint32(1); err != nil || value != 0x05040302 {
		t.Fatalf("little u32: %#x %v", value, err)
	}
	if value, err := little.Uint64(0); err != nil || value != 0x0807060504030201 {
		t.Fatalf("little u64: %#x %v", value, err)
	}
	big, err := reader.WithOrder(BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := big.Uint16(1); err != nil || value != 0x0203 {
		t.Fatalf("big u16: %#x %v", value, err)
	}
	if value, err := big.Uint32(1); err != nil || value != 0x02030405 {
		t.Fatalf("big u32: %#x %v", value, err)
	}
	if value, err := big.Uint64(0); err != nil || value != 0x0102030405060708 {
		t.Fatalf("big u64: %#x %v", value, err)
	}
	if reader.Kind() != ByteOrderUnknown {
		t.Fatal("WithOrder changed the receiver")
	}
	if _, err := reader.WithOrder(ByteOrderUnknown); err == nil {
		t.Fatal("unknown byte order accepted")
	}
}

func TestByteReaderBounds(t *testing.T) {
	reader := newReader(t, []byte{1, 2, 3, 4})
	for _, tc := range []struct{ offset, size uint64 }{
		{0, 5}, {4, 1}, {5, 0}, {math.MaxUint64, 1},
	} {
		if err := reader.CheckRange(tc.offset, tc.size); err == nil {
			t.Fatalf("out-of-range %d/%d accepted", tc.offset, tc.size)
		}
	}
	for _, tc := range []struct{ offset, size uint64 }{
		{0, 0}, {4, 0}, {0, 4}, {3, 1},
	} {
		if err := reader.CheckRange(tc.offset, tc.size); err != nil {
			t.Fatalf("valid range %d/%d rejected: %v", tc.offset, tc.size, err)
		}
	}
	if _, err := reader.Bytes(math.MaxUint64, 2); err == nil {
		t.Fatal("overflowing range accepted")
	}
	if _, err := reader.Uint8(4); err == nil {
		t.Fatal("read past the end accepted")
	}
	if _, err := reader.ReadAt(make([]byte, 2), -1); err == nil {
		t.Fatal("negative read offset accepted")
	}
	n, err := reader.ReadAt(make([]byte, 2), 3)
	if n != 1 || !errors.Is(err, io.EOF) {
		t.Fatalf("truncated read: %d %v", n, err)
	}
	n, err = reader.ReadAt(nil, 4)
	if n != 0 || err != nil {
		t.Fatalf("empty read at end: %d %v", n, err)
	}
}

func TestByteReaderBytesAreCopies(t *testing.T) {
	raw := []byte{1, 2, 3, 4}
	reader := newReader(t, raw)
	got, err := reader.Bytes(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	got[0] = 0xff
	if !bytes.Equal(raw, []byte{1, 2, 3, 4}) {
		t.Fatal("Bytes exposed the source buffer")
	}
}

func TestByteOrderOf(t *testing.T) {
	if ByteOrderOf(binary.LittleEndian) != LittleEndian {
		t.Fatal("little endian mapping failed")
	}
	if ByteOrderOf(binary.BigEndian) != BigEndian {
		t.Fatal("big endian mapping failed")
	}
}

type shortReaderAt struct{}

func (shortReaderAt) ReadAt(p []byte, _ int64) (int, error) { return len(p) / 2, nil }

func TestByteReaderNormalizesShortReads(t *testing.T) {
	reader, err := NewByteReader("short", shortReaderAt{}, 4)
	if err != nil {
		t.Fatal(err)
	}
	n, err := reader.ReadAt(make([]byte, 4), 0)
	if n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short read without an error: %d %v", n, err)
	}
}
