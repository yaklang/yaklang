package stream_parser

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	zlibJSONMaxCompressed = 1 << 16
	zlibJSONMaxDecoded    = 1 << 20
	zlibJSONMaxDepth      = 128
	zlibJSONMaxValues     = 65536 // includes each object member name
)

type zlibJSONField struct {
	Name, Type string
	Start, End int // unchanged record byte offsets, never decompressed offsets
	Children   []zlibJSONField
	Info       map[string]any
}

func decodeZlibJSONRecord(wire []byte) (fields []zlibJSONField, info map[string]any, err error) {
	defer func() {
		if err != nil {
			fields, info = nil, nil
		}
	}()
	fail := func(reason string) error { return fmt.Errorf("zlib-json: %s", reason) }
	if len(wire) < 12 || len(wire) > 4+zlibJSONMaxCompressed {
		return nil, nil, fail("record boundary must be 12..65540 bytes")
	}
	if uint64(binary.BigEndian.Uint32(wire[:4])) != uint64(len(wire)-4) {
		return nil, nil, fail("compressed length does not match exact record boundary")
	}
	cmf, flg := wire[4], wire[5]
	if cmf&15 != 8 || cmf>>4 > 7 || binary.BigEndian.Uint16(wire[4:6])%31 != 0 {
		return nil, nil, fail("invalid RFC1950 CMF or FLG header")
	}
	if flg&0x20 != 0 {
		return nil, nil, fail("preset dictionary is unsupported in this profile")
	}
	// Go's zlib reader validates CINFO <= 7 but its DEFLATE decoder always
	// has a 32KiB window. It does not enforce smaller advertised windows.
	// Do not label those streams fully validated without a distance audit.
	if cmf>>4 != 7 {
		return nil, nil, fail("unsupported smaller-window zlib profile; 32KiB required")
	}
	// bytes.Reader implements io.ByteReader: flate cannot buffer past the
	// member into a second stream or a suffix and hide that trailing input.
	compressed := bytes.NewReader(wire[4:])
	inflater, err := zlib.NewReader(compressed)
	if err != nil {
		return nil, nil, fail("invalid zlib member header")
	}
	defer inflater.Close()
	decoded, err := io.ReadAll(io.LimitReader(inflater, zlibJSONMaxDecoded+1))
	if err != nil {
		if errors.Is(err, zlib.ErrChecksum) {
			return nil, nil, fail("Adler32 checksum mismatch")
		}
		return nil, nil, fail("invalid or truncated DEFLATE member")
	}
	if len(decoded) > zlibJSONMaxDecoded {
		return nil, nil, fail("decoded JSON exceeds 1MiB limit")
	}
	// Checksum verification requires reaching zlib EOF, not merely Close.
	var probe [1]byte
	n, readErr := inflater.Read(probe[:])
	if n != 0 || readErr != io.EOF {
		return nil, nil, fail("zlib member did not reach verified EOF")
	}
	if compressed.Len() != 0 {
		return nil, nil, fail("trailing bytes or concatenated zlib member")
	}
	if !utf8.Valid(decoded) {
		return nil, nil, fail("decoded JSON is not valid UTF-8")
	}
	document, count, err := zlibJSONDocument(decoded)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(decoded)
	leaf := func(name, typ string, start, end int) zlibJSONField {
		return zlibJSONField{Name: name, Type: typ, Start: start, End: end}
	}
	fields = []zlibJSONField{
		leaf("Compressed Length", "uint32", 0, 4),
		{Name: "Zlib Member", Start: 4, End: len(wire), Children: []zlibJSONField{
			leaf("CMF", "uint8", 4, 5), leaf("FLG", "uint8", 5, 6),
			leaf("DEFLATE Bytes", "raw", 6, len(wire)-4), leaf("Adler32", "uint32", len(wire)-4, len(wire)),
		}},
	}
	info = map[string]any{
		"Profile":            "4-byte BE length and one RFC1950 32KiB-window zlib member containing RFC8259 JSON",
		"Compression Method": uint8(cmf & 15), "Compression Info": uint8(cmf >> 4), "Window Bytes": 32768,
		"FCHECK": uint8(flg & 31), "FDICT": false, "FLEVEL": uint8(flg >> 6),
		"Adler32 Verified": true, "Compressed Bytes Consumed": len(wire) - 4,
		"Maximum Compressed Bytes": zlibJSONMaxCompressed, "Maximum Decoded Bytes": zlibJSONMaxDecoded,
		"Maximum JSON Depth": zlibJSONMaxDepth, "Maximum JSON Values And Names": zlibJSONMaxValues,
		"Decoded Byte Length": len(decoded), "Decoded SHA256": fmt.Sprintf("%x", sum),
		"Decoded Bytes": decoded, "Decoded Document": document, "JSON Values And Names": count,
		"Decoded Source":                 "RFC1950 decompression of this record's Zlib Member",
		"Decoded Values Have Wire Spans": false, "Object Members": "ordered; duplicate names preserved",
		"Number Representation":         "json.Number; original decimal text without float conversion",
		"String Representation":         "Go encoding/json; unpaired UTF16 escapes become U+FFFD; original decoded bytes preserved",
		"Application Identity Inferred": false, "Application Semantics Decoded": false,
		"TCP Reassembled": false, "Structured Generation Supported": false,
	}
	return fields, info, nil
}

// This separate value tree is metadata, NOT a base.Node tree over the wire
// buffer. Ordered members preserve repeated names instead of silently choosing
// first or last; any JSON root kind is allowed. Original decoded bytes remain
// available for spelling, whitespace and unpaired-surrogate fidelity.
func zlibJSONDocument(decoded []byte) (map[string]any, int, error) {
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.UseNumber()
	count := 0
	invalid := func(err error) error {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return fmt.Errorf("zlib-json: invalid JSON syntax at decoded byte %d", syntax.Offset)
		}
		// Do not reflect document strings, member names or values in errors.
		return fmt.Errorf("zlib-json: invalid or incomplete JSON document")
	}
	budget := func() error {
		count++
		if count > zlibJSONMaxValues {
			return fmt.Errorf("zlib-json: JSON exceeds 65536-value-and-name limit")
		}
		return nil
	}
	var value func(int) (map[string]any, error)
	value = func(depth int) (map[string]any, error) {
		if err := budget(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, invalid(err)
		}
		delim, container := token.(json.Delim)
		if container {
			if depth >= zlibJSONMaxDepth {
				return nil, fmt.Errorf("zlib-json: JSON exceeds 128-container depth limit")
			}
			switch delim {
			case '{':
				members := []any{}
				for decoder.More() {
					if err := budget(); err != nil {
						return nil, err
					}
					key, err := decoder.Token()
					if err != nil {
						return nil, invalid(err)
					}
					name, ok := key.(string)
					if !ok {
						return nil, invalid(nil)
					}
					child, err := value(depth + 1)
					if err != nil {
						return nil, err
					}
					members = append(members, map[string]any{"Name": name, "Value": child})
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim('}') {
					return nil, invalid(err)
				}
				return map[string]any{"Kind": "object", "Members": members}, nil
			case '[':
				items := []any{}
				for decoder.More() {
					child, err := value(depth + 1)
					if err != nil {
						return nil, err
					}
					items = append(items, child)
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim(']') {
					return nil, invalid(err)
				}
				return map[string]any{"Kind": "array", "Items": items}, nil
			default:
				return nil, invalid(nil)
			}
		}
		switch v := token.(type) {
		case string:
			return map[string]any{"Kind": "string", "Value": v}, nil
		case json.Number:
			return map[string]any{"Kind": "number", "Value": v}, nil
		case bool:
			return map[string]any{"Kind": "boolean", "Value": v}, nil
		case nil:
			return map[string]any{"Kind": "null", "Value": nil}, nil
		default:
			return nil, invalid(nil)
		}
	}
	document, err := value(0)
	if err != nil {
		return nil, 0, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, 0, fmt.Errorf("zlib-json: unconsumed data after JSON value")
	}
	return document, count, nil
}
