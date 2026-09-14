package stream_parser

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
)

// IPCompPayload distinguishes decoded DEFLATE data from data whose algorithm
// requires a negotiated association. Bytes are returned as metadata only.
type IPCompPayload struct {
	Decoded bool
	Bytes   []byte
}

func decodeIPCompPayload(cpi int, wire []byte) (*IPCompPayload, error) {
	const limit = 1 << 20
	if len(wire) == 0 || len(wire) > limit {
		return nil, fmt.Errorf("ipcomp: compressed payload size is outside bounds")
	}
	if cpi != 2 {
		return &IPCompPayload{Bytes: bytes.Clone(wire)}, nil
	}
	// RFC 2394 uses a complete raw DEFLATE stream, without a zlib wrapper.
	// bytes.Reader also implements ReadByte, keeping the inflater from reading
	// past its final block so trailing compressed-stream bytes remain visible.
	source := bytes.NewReader(wire)
	reader := flate.NewReader(source)
	defer reader.Close()
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("ipcomp: invalid DEFLATE payload: %w", err)
	}
	if len(payload) > limit {
		return nil, fmt.Errorf("ipcomp: decompressed payload exceeds limit")
	}
	if source.Len() != 0 {
		return nil, fmt.Errorf("ipcomp: trailing bytes after DEFLATE stream")
	}
	return &IPCompPayload{Decoded: true, Bytes: payload}, nil
}
