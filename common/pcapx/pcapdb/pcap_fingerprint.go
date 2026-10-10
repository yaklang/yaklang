package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

const (
	FullHashLimit         = 20 << 20
	FingerprintVersion    = 1
	fingerprintSamples    = 8
	fingerprintSampleSize = 1 << 20
)

type Fingerprint struct {
	Hash    string `json:"hash"`
	Mode    string `json:"mode"`
	Version int    `json:"version"`
	Size    int64  `json:"size"`
}

func FingerprintFile(filename string) (*Fingerprint, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return fingerprintFile(context.Background(), f)
}

func fingerprintFile(ctx context.Context, f *os.File) (*Fingerprint, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("pcapdb: input must be a regular file")
	}
	result := &Fingerprint{Version: FingerprintVersion, Size: info.Size(), Mode: "full-sha256"}
	h := sha256.New()
	if info.Size() <= FullHashLimit {
		var n int64
		if n, err = copyContext(ctx, h, io.NewSectionReader(f, 0, info.Size())); err != nil {
			return nil, err
		}
		if n != info.Size() {
			return nil, io.ErrUnexpectedEOF
		}
	} else {
		result.Mode = "sampled-sha256"
		io.WriteString(h, "pcapdb-sampled-sha256-v1\x00")
		writeHashInt(h, uint64(info.Size()))
		for i := int64(0); i < fingerprintSamples; i++ {
			// Split the division to avoid overflowing on very large files.
			span := info.Size() - fingerprintSampleSize
			offset := span/(fingerprintSamples-1)*i + span%(fingerprintSamples-1)*i/(fingerprintSamples-1)
			writeHashInt(h, uint64(offset))
			writeHashInt(h, fingerprintSampleSize)
			var n int64
			if n, err = copyContext(ctx, h, io.NewSectionReader(f, offset, fingerprintSampleSize)); err != nil {
				return nil, err
			}
			if n != fingerprintSampleSize {
				return nil, io.ErrUnexpectedEOF
			}
		}
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("pcapdb: source changed during fingerprinting")
	}
	result.Hash = hex.EncodeToString(h.Sum(nil))
	return result, nil
}

func writeHashInt(h hash.Hash, value uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], value)
	h.Write(b[:])
}

func hashFile(ctx context.Context, f *os.File, size int64) (string, error) {
	h := sha256.New()
	n, err := copyContext(ctx, h, io.NewSectionReader(f, 0, size))
	if err != nil {
		return "", err
	}
	if n != size {
		return "", io.ErrUnexpectedEOF
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyContext bounds memory and observes cancellation between file reads.
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 256<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			written, err := dst.Write(buffer[:n])
			total += int64(written)
			if err != nil {
				return total, err
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
