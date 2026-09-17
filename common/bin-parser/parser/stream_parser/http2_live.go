package stream_parser

import (
	"fmt"
	"golang.org/x/net/http2/hpack"
)

// HTTP2FrameLayout reuses the exact public rule's syntax validation. Fragment
// is borrowed from wire; consumers must not retain it beyond that input.
type HTTP2FrameLayout struct {
	Type, Flags      byte
	Stream, Promised uint32
	Fragment         []byte
}

func InspectHTTP2Frame(wire []byte) (HTTP2FrameLayout, error) {
	f, err := inspectHTTP2WireFrame(wire, 0, false)
	if err != nil {
		return HTTP2FrameLayout{}, err
	}
	if f.end != len(wire) {
		return HTTP2FrameLayout{}, fmt.Errorf("http2: trailing frame bytes")
	}
	out := HTTP2FrameLayout{Type: f.typ, Flags: f.flags, Stream: f.stream, Promised: f.promised}
	if f.fragmentStart >= 0 {
		out.Fragment = wire[f.fragmentStart:f.fragmentEnd]
	}
	return out, nil
}

// HTTP2HeaderDecoder is one ordered plaintext direction's HPACK dictionary.
// The owner serializes calls and discards the decoder after any error.
type HTTP2HeaderDecoder struct {
	d        *hpack.Decoder
	headers  []map[string]any
	bytes    int
	err      error
	capacity uint32
	reduce   bool
	minimum  uint32
}

func NewHTTP2HeaderDecoder() *HTTP2HeaderDecoder {
	x := &HTTP2HeaderDecoder{capacity: 4096}
	x.d = hpack.NewDecoder(4096, func(h hpack.HeaderField) {
		x.bytes += len(h.Name) + len(h.Value) + 32
		if len(x.headers) >= http2FieldsMaxHeaders || x.bytes > http2FieldsMaxBytes {
			x.err = fmt.Errorf("http2: expanded header limit")
			x.d.SetEmitEnabled(false)
			return
		}
		x.headers = append(x.headers, map[string]any{"Name": h.Name, "Value": h.Value, "Sensitive": h.Sensitive})
	})
	x.d.SetMaxStringLength(http2FieldsMaxString)
	return x
}

func (x *HTTP2HeaderDecoder) SetAllowedTableSize(n uint32) {
	x.d.SetAllowedMaxDynamicTableSize(n)
	x.RequireTableReduction(n)
}

// RequireTableReduction tracks the smallest acknowledged setting even when a
// subsequent pending SETTINGS already permits the table to grow again.
func (x *HTTP2HeaderDecoder) RequireTableReduction(n uint32) {
	if n < x.capacity && (!x.reduce || n < x.minimum) {
		x.reduce = true
		x.minimum = n
	}
}

func hpackUpdateSize(block []byte) uint32 {
	v := uint64(block[0] & 31)
	if v == 31 {
		for at, shift := 1, uint(0); at < len(block); at, shift = at+1, shift+7 {
			v += uint64(block[at]&127) << shift
		}
	}
	return uint32(v)
}

func (x *HTTP2HeaderDecoder) Decode(block []byte) ([]map[string]any, error) {
	updates, err := http2HPACKLeadingUpdates(block)
	if err != nil {
		return nil, err
	}
	if x.reduce && len(block) > 0 && len(updates) == 0 {
		return nil, fmt.Errorf("http2: missing HPACK table reduction after SETTINGS ACK")
	}
	if x.reduce && len(updates) > 0 && hpackUpdateSize(block[:updates[0]]) > x.minimum {
		return nil, fmt.Errorf("http2: HPACK update omitted minimum acknowledged table size")
	}
	x.headers, x.bytes, x.err = nil, 0, nil
	x.d.SetEmitEnabled(true)
	if err := decodeHTTP2HPACKScanned(x.d, block, updates); err != nil {
		x.headers = nil
		return nil, err
	}
	result, err := x.headers, x.err
	if err == nil && len(updates) > 0 {
		start := 0
		for _, end := range updates {
			v := uint64(block[start] & 31)
			if v == 31 {
				for at, shift := start+1, uint(0); at < end; at, shift = at+1, shift+7 {
					v += uint64(block[at]&127) << shift
				}
			}
			x.capacity = uint32(v)
			start = end
		}
		x.reduce = false
	}
	x.headers = nil
	return result, err
}
