package stream_parser

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/golang/snappy"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const (
	mongoOpCompressed = 2012
	mongoOpMsg        = 2013
	mongoMaxBytes     = 1 << 20
)

type mongoReader struct {
	wire   []byte
	at     int
	fields []tlsCertificateField
	err    error
}

func parseMongoDBFields(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	return parseExactByteFieldTreeWithEndian(node, process, decodeMongoDBFields, "mongodb-fields", "little")
}

func decodeMongoDBFields(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 16 || len(wire) > mongoMaxBytes {
		return nil, nil, fmt.Errorf("mongodb-fields: 16..1048576 bytes required")
	}
	r := &mongoReader{wire: wire}
	length := r.u32("Message Length")
	if r.err != nil || length != uint32(len(wire)) {
		return nil, nil, fmt.Errorf("mongodb-fields: length mismatch")
	}
	req := r.u32("Request ID")
	respTo := r.u32("Response To")
	op := r.u32("Op Code")
	info := map[string]any{
		"Request ID": uint64(req), "Response To": uint64(respTo),
		"Op Code": uint64(op), "Opcode Name": mongoOpcodeName(op),
		"Context Level": "observed",
	}
	switch op {
	case mongoOpMsg:
		r.opMsg(info, len(wire))
	case mongoOpCompressed:
		r.opCompressed(info)
	default:
		return nil, nil, fmt.Errorf("mongodb-fields: unsupported opcode %d", op)
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.at != len(wire) {
		return nil, nil, fmt.Errorf("mongodb-fields: trailing bytes")
	}
	return r.fields, info, nil
}

func (r *mongoReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.at+n > len(r.wire) {
		r.err = fmt.Errorf("mongodb-fields: truncated %s", name)
		return nil
	}
	s := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	return r.wire[s:r.at]
}

func (r *mongoReader) u32(name string) uint32 {
	b := r.take(name, "uint32", 4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *mongoReader) u8(name string) byte {
	b := r.take(name, "uint8", 1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *mongoReader) cstring(name string) string {
	if r.err != nil {
		return ""
	}
	s := r.at
	for r.at < len(r.wire) && r.wire[r.at] != 0 {
		r.at++
	}
	if r.at >= len(r.wire) {
		r.err = fmt.Errorf("mongodb-fields: truncated %s", name)
		return ""
	}
	v := string(r.wire[s:r.at])
	r.fields = append(r.fields, tlsCertificateLeaf(name, "string", s, r.at))
	r.at++
	return v
}

func (r *mongoReader) opMsg(info map[string]any, end int) {
	flags := r.u32("Flag Bits")
	info["Flag Bits"] = uint64(flags)
	info["More To Come"] = flags&2 != 0
	if flags&0xfffc != 0 {
		r.err = fmt.Errorf("mongodb-fields: unknown required OP_MSG flag")
		return
	}
	payloadEnd := end
	if flags&1 != 0 {
		if end-r.at < 4 {
			r.err = fmt.Errorf("mongodb-fields: truncated checksum")
			return
		}
		payloadEnd = end - 4
	}
	var sections []map[string]any
	for r.err == nil && r.at < payloadEnd {
		kind := r.u8("Section Kind")
		sec := map[string]any{"Kind": uint64(kind)}
		switch kind {
		case 0:
			r.bsonDoc("Body")
			sec["Kind Name"] = "Body"
		case 1:
			sz := int(r.u32("Sequence Size"))
			start := r.at - 4
			if sz < 5 || start+sz > payloadEnd {
				r.err = fmt.Errorf("mongodb-fields: invalid document sequence size")
				return
			}
			id := r.cstring("Sequence Identifier")
			sec["Kind Name"] = "Document Sequence"
			sec["Sequence Identifier"] = id
			seqEnd := start + sz
			var n int
			for r.err == nil && r.at < seqEnd {
				r.bsonDoc("Sequence Document")
				n++
			}
			if r.at != seqEnd && r.err == nil {
				r.err = fmt.Errorf("mongodb-fields: document sequence boundary mismatch")
				return
			}
			sec["Document Count"] = n
			info["Sequence Identifier"] = id
		default:
			r.err = fmt.Errorf("mongodb-fields: unsupported section kind %d", kind)
			return
		}
		sections = append(sections, sec)
	}
	info["Section Count"] = len(sections)
	info["Sections"] = sections
	if flags&1 != 0 {
		r.take("Checksum", "uint32", 4)
	}
}

func (r *mongoReader) opCompressed(info map[string]any) {
	orig := r.u32("Original Opcode")
	uncomp := r.u32("Uncompressed Size")
	cid := r.u8("Compressor ID")
	info["Original Opcode"] = uint64(orig)
	info["Uncompressed Size"] = uint64(uncomp)
	info["Compressor ID"] = uint64(cid)
	info["Compressor"] = mongoCompressorName(cid)
	payload := r.take("Compressed Message", "raw", len(r.wire)-r.at)
	if r.err != nil {
		return
	}
	if uncomp == 0 || int(uncomp) > mongoMaxBytes {
		r.err = fmt.Errorf("mongodb-fields: uncompressed size out of range")
		return
	}
	raw, err := mongoDecompress(cid, payload, int(uncomp))
	if err != nil {
		r.err = err
		return
	}
	if len(raw) != int(uncomp) {
		r.err = fmt.Errorf("mongodb-fields: uncompressed length mismatch")
		return
	}
	info["Uncompressed Body"] = append([]byte(nil), raw...)
	inner := &mongoReader{wire: raw}
	switch orig {
	case mongoOpMsg:
		inner.opMsg(info, len(raw))
	default:
		r.err = fmt.Errorf("mongodb-fields: compressed opcode %d is not OP_MSG", orig)
		return
	}
	if inner.err != nil {
		r.err = inner.err
		return
	}
	if inner.at != len(raw) {
		r.err = fmt.Errorf("mongodb-fields: inner OP_MSG trailing bytes")
	}
	info["Inner Opcode Name"] = mongoOpcodeName(orig)
}

func (r *mongoReader) bsonDoc(prefix string) {
	start := r.at
	sz := int(r.u32(prefix + " Size"))
	if r.err != nil {
		return
	}
	if sz < 5 || start+sz > len(r.wire) {
		r.err = fmt.Errorf("mongodb-fields: invalid BSON size")
		return
	}
	end := start + sz
	for r.err == nil && r.at < end {
		t := r.u8("Type")
		if t == 0 {
			if r.at != end {
				r.err = fmt.Errorf("mongodb-fields: premature BSON terminator")
			}
			return
		}
		r.cstring("Name")
		r.bsonValue(t)
	}
	if r.err == nil && r.at != end {
		r.err = fmt.Errorf("mongodb-fields: BSON boundary mismatch")
	}
}

func (r *mongoReader) bsonValue(t byte) {
	switch t {
	case 0x01, 0x09, 0x12:
		r.take("Value", "raw", 8)
	case 0x02:
		n := int(r.u32("String Length"))
		if n < 1 {
			r.err = fmt.Errorf("mongodb-fields: invalid string length")
			return
		}
		r.take("Value", "string", n)
	case 0x05:
		n := int(r.u32("Binary Length"))
		r.u8("Subtype")
		if n > 0 {
			r.take("Value", "raw", n)
		}
	case 0x07:
		r.take("Value", "raw", 12)
	case 0x08:
		r.u8("Value")
	case 0x0a:
	case 0x10:
		r.u32("Int32")
	case 0x03, 0x04:
		r.bsonDoc("Embedded")
	default:
		r.err = fmt.Errorf("mongodb-fields: unsupported BSON type 0x%02x", t)
	}
}

func mongoDecompress(id byte, src []byte, want int) ([]byte, error) {
	switch id {
	case 0:
		if len(src) != want {
			return nil, fmt.Errorf("mongodb-fields: noop compressor length mismatch")
		}
		return append([]byte(nil), src...), nil
	case 1:
		out, err := snappy.Decode(nil, src)
		if err != nil {
			return nil, fmt.Errorf("mongodb-fields: snappy: %w", err)
		}
		return out, nil
	case 2:
		zr, err := zlib.NewReader(bytes.NewReader(src))
		if err != nil {
			return nil, fmt.Errorf("mongodb-fields: zlib: %w", err)
		}
		defer zr.Close()
		out, err := io.ReadAll(io.LimitReader(zr, int64(want)+1))
		if err != nil {
			return nil, fmt.Errorf("mongodb-fields: zlib: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("mongodb-fields: unsupported compressor %d", id)
	}
}

func mongoOpcodeName(op uint32) string {
	switch op {
	case mongoOpMsg:
		return "OP_MSG"
	case mongoOpCompressed:
		return "OP_COMPRESSED"
	case 2004:
		return "OP_QUERY"
	}
	return "UNKNOWN"
}

func mongoCompressorName(id byte) string {
	switch id {
	case 0:
		return "noop"
	case 1:
		return "snappy"
	case 2:
		return "zlib"
	case 3:
		return "zstd"
	}
	return "unknown"
}
