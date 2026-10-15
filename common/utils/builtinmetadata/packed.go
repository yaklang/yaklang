package builtinmetadata

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
)

// A small manifest and a separate data block let Hash verify the complete
// artifact and script set without decompressing or parsing any metadata entry.
func Encode(b *Bundle) ([]byte, error) {
	header := *b
	header.Entries = nil
	index, err := compress(&header)
	if err != nil {
		return nil, err
	}
	data, err := compress(b.Entries)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 8, 8+len(index)+len(data))
	copy(out, "YBM3")
	binary.BigEndian.PutUint32(out[4:], uint32(len(index)))
	out = append(out, index...)
	return append(out, data...), nil
}

func compress(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	w := brotli.NewWriterLevel(&out, 11)
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(data)), (128<<20)+1))
	if err == nil && len(raw) > 128<<20 {
		err = fmt.Errorf("metadata exceeds size limit")
	}
	return raw, err
}

func decodeIndex(data []byte, kind, expectedHash string) (*Bundle, []byte, error) {
	if expectedHash != "" && Digest(data) != expectedHash {
		return nil, nil, fmt.Errorf("%s metadata hash mismatch", kind)
	}
	if len(data) < 9 || string(data[:4]) != "YBM3" {
		return nil, nil, fmt.Errorf("invalid metadata container")
	}
	size := uint64(binary.BigEndian.Uint32(data[4:8]))
	if size == 0 || size >= uint64(len(data)-8) {
		return nil, nil, fmt.Errorf("invalid metadata index size")
	}
	end := 8 + int(size)
	raw, err := decompress(data[8:end])
	if err != nil {
		return nil, nil, err
	}
	b, err := Parse(raw, kind)
	if err != nil {
		return nil, nil, err
	}
	if b.Entries != nil {
		return nil, nil, fmt.Errorf("metadata index contains eager entries")
	}
	return b, data[end:], nil
}

func (b *Bundle) decodeEntries(data []byte) error {
	raw, err := decompress(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &b.Entries); err != nil {
		return err
	}
	return nil
}

// Decode is the full build-time validator. Runtime hash checks use decodeIndex;
// the entry data block is loaded only on the first Lookup for that script set.
func Decode(data []byte, kind, expectedHash string) (*Bundle, error) {
	b, entries, err := decodeIndex(data, kind, expectedHash)
	if err == nil {
		err = b.decodeEntries(entries)
	}
	if err == nil {
		err = b.validateEntries()
	}
	return b, err
}
