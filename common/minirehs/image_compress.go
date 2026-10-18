package minirehs

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"

	"github.com/klauspost/compress/zstd"
)

const imageCompressionWindow = 1 << 20

// The envelope keeps cache identity readable without decompression. Its payload
// is a complete raw v1 image, so the native MVS blob and raw layout stay intact.
func compressCompilationImage(raw []byte) ([]byte, error) {
	w, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault),
		zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(imageCompressionWindow))
	if err != nil {
		return nil, err
	}
	defer w.Close()
	b := append([]byte(nil), raw[:imageHeaderSize]...)
	binary.LittleEndian.PutUint32(b[12:], binary.LittleEndian.Uint32(b[12:])|imageCompressed)
	b = putU32(b, uint32(len(raw)))
	b = w.EncodeAll(raw, b)
	// Small or incompressible images should never grow from the envelope.
	if len(b)+4 >= len(raw) {
		return raw, nil
	}
	return putU32(b, crc32.ChecksumIEEE(b)), nil
}

func decompressCompilationImage(data []byte, rawSize int) ([]byte, error) {
	r, err := zstd.NewReader(bytes.NewReader(data[imageHeaderSize+4:len(data)-4]),
		zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(imageCompressionWindow),
		zstd.WithDecoderMaxMemory(imageCompressionWindow), zstd.WithDecodeBuffersBelow(0))
	if err != nil {
		return nil, ErrCorruptImage
	}
	defer r.Close()
	// A forged declared size must not eagerly allocate gigabytes. Reserve at most
	// 32 MiB, also bounded by compressed input size, then grow only on actual output.
	// Leave MinRead spare to avoid Buffer.ReadFrom doubling at the final EOF.
	capacity := int64(rawSize) + bytes.MinRead
	if limit := int64(len(data)) * 32; capacity > limit {
		capacity = limit
	}
	if capacity > 32<<20 {
		capacity = 32 << 20
	}
	out := bytes.NewBuffer(make([]byte, 0, int(capacity)))
	_, err = out.ReadFrom(io.LimitReader(r, int64(rawSize)+1))
	if err != nil || out.Len() != rawSize {
		return nil, ErrCorruptImage
	}
	return out.Bytes(), nil
}
