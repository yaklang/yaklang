package stream_parser

import (
	"fmt"
	"sync"

	"github.com/quic-go/qpack"
	"golang.org/x/net/http2/hpack"
)

// RFC 9204 §§3–4. Replays one caller-supplied, complete encoder prefix into
// a local table. No cross-call table, pending-stream queue, acknowledgments,
// eviction-reference lifecycle or QUIC connection state is fabricated.
// Existing qpack supplies ONLY RFC Appendix A's fixed table; dynamic decoding
// below is independent. Existing hpack supplies the shared Huffman alphabet.
var http3StaticOnce sync.Once
var http3StaticTable [99]qpack.HeaderField

func http3QPACKStatic(index uint64) (qpack.HeaderField, error) {
	if index >= 99 {
		return qpack.HeaderField{}, fmt.Errorf("qpack: invalid static table index")
	}
	http3StaticOnce.Do(func() {
		for i := range http3StaticTable {
			wire := []byte{0, 0, byte(0xc0 | i)}
			if i >= 63 {
				wire = []byte{0, 0, 0xff, byte(i - 63)}
			}
			fields, err := qpack.NewDecoder(nil).DecodeFull(wire)
			if err != nil || len(fields) != 1 {
				panic("qpack: installed static table decoder contract changed")
			}
			http3StaticTable[i] = fields[0]
		}
	})
	return http3StaticTable[index], nil
}

func (c *http3Cursor) prefixed(name string, prefix uint) (uint64, http3Field, error) {
	start := c.at
	if c.at >= c.end || prefix == 0 || prefix > 8 {
		return 0, http3Field{}, fmt.Errorf("qpack: truncated %s", name)
	}
	mask := uint64(1<<prefix - 1)
	v := uint64(c.wire[c.at]) & mask
	c.at++
	if v == mask {
		for shift := uint(0); ; shift += 7 {
			if c.at >= c.end {
				return 0, http3Field{}, fmt.Errorf("qpack: truncated %s continuation", name)
			}
			if shift >= 63 {
				return 0, http3Field{}, fmt.Errorf("qpack: integer exceeds 62-bit/10-byte profile")
			}
			b := c.wire[c.at]
			c.at++
			part := uint64(b & 127)
			if part > (http3MaxInteger-v)>>shift {
				return 0, http3Field{}, fmt.Errorf("qpack: integer exceeds 62 bits")
			}
			v += part << shift
			if b&128 == 0 {
				break
			}
		}
	}
	f := http3Group(name, start*8+8-int(prefix), c.at*8, http3Leaf("Prefix Value", "uint8", start*8+8-int(prefix), start*8+8))
	if c.at > start+1 {
		f.Children = append(f.Children, http3Leaf("Continuation Bytes", "raw", (start+1)*8, c.at*8))
	}
	f.Info = map[string]any{"Decoded Value": v}
	return v, f, nil
}

func (c *http3Cursor) text(name string, prefix uint, coordinate string) (string, http3Field, map[string]any, error) {
	start := c.at
	if start >= c.end {
		return "", http3Field{}, nil, fmt.Errorf("qpack: truncated %s string", name)
	}
	huffman := c.wire[start]&(1<<prefix) != 0
	length, lf, err := c.prefixed("Encoded Length", prefix)
	if err != nil {
		return "", http3Field{}, nil, err
	}
	if length > http3MaxString || length > uint64(c.end-c.at) {
		return "", http3Field{}, nil, fmt.Errorf("qpack: string length exceeds boundary/65536-byte profile")
	}
	encodedStart := c.at
	c.at += int(length)
	value := string(c.wire[encodedStart:c.at])
	if huffman {
		value, err = hpack.HuffmanDecodeToString(c.wire[encodedStart:c.at])
		if err != nil {
			return "", http3Field{}, nil, fmt.Errorf("qpack: invalid Huffman string: %w", err)
		}
	}
	if len(value) > http3MaxString {
		return "", http3Field{}, nil, fmt.Errorf("qpack: decoded string exceeds 65536-byte profile")
	}
	f := http3Group(name, start*8+7-int(prefix), c.at*8, http3Leaf("Huffman", "uint8", start*8+7-int(prefix), start*8+8-int(prefix)), lf, http3Leaf("Encoded String", "raw", encodedStart*8, c.at*8))
	f.Info = map[string]any{"Decoded String": value, "Huffman Encoded": huffman}
	source := map[string]any{"Coordinate System": coordinate, "Encoded String Bit Span": [2]uint64{uint64(encodedStart) * 8, uint64(c.at) * 8}, "Huffman Encoded": huffman}
	return value, f, source, nil
}

type http3QPACKEntry struct {
	name, value             string
	nameSource, valueSource map[string]any
	absolute                uint64
}
type http3QPACKTable struct {
	entries                          []http3QPACKEntry
	insertCount, capacity, size, max uint64
}

func (t *http3QPACKTable) evict(limit uint64) {
	for t.size > limit && len(t.entries) > 0 {
		e := t.entries[0]
		t.size -= uint64(len(e.name) + len(e.value) + 32)
		t.entries[0] = http3QPACKEntry{}
		t.entries = t.entries[1:]
	}
}
func (t *http3QPACKTable) insert(e http3QPACKEntry) error {
	size := uint64(len(e.name) + len(e.value) + 32)
	if size > t.capacity {
		return fmt.Errorf("qpack: entry exceeds current dynamic table capacity")
	}
	if t.insertCount >= http3MaxItems {
		return fmt.Errorf("qpack: insert count exceeds 4096-entry replay profile")
	}
	t.evict(t.capacity - size)
	e.absolute = t.insertCount
	t.insertCount++
	t.size += size
	t.entries = append(t.entries, e)
	return nil
}
func (t *http3QPACKTable) absolute(index uint64) (http3QPACKEntry, error) {
	if len(t.entries) == 0 || index < t.entries[0].absolute || index >= t.insertCount {
		return http3QPACKEntry{}, fmt.Errorf("qpack: absent or evicted dynamic table index")
	}
	return t.entries[index-t.entries[0].absolute], nil
}
func (t *http3QPACKTable) relative(index uint64) (http3QPACKEntry, error) {
	if index >= t.insertCount {
		return http3QPACKEntry{}, fmt.Errorf("qpack: dynamic relative index underflow")
	}
	return t.absolute(t.insertCount - index - 1)
}
func http3QPACKStaticEntry(index uint64) (http3QPACKEntry, error) {
	f, err := http3QPACKStatic(index)
	if err != nil {
		return http3QPACKEntry{}, err
	}
	source := map[string]any{"Coordinate System": "RFC9204-static-table", "Static Index": index}
	return http3QPACKEntry{name: f.Name, value: f.Value, nameSource: source, valueSource: source}, nil
}

func http3QPACKEncoder(wire []byte, start int, max uint64) (*http3QPACKTable, []http3Field, error) {
	t := &http3QPACKTable{max: max}
	if len(wire) > http3MaxBytes || max > http3MaxBytes {
		return nil, nil, fmt.Errorf("qpack: encoder/table exceeds implementation profile")
	}
	if start < len(wire) && max == 0 {
		return nil, nil, fmt.Errorf("qpack: encoder instructions require observed nonzero peer maximum capacity")
	}
	c := http3Cursor{wire: wire, at: start, end: len(wire)}
	fields := []http3Field{}
	for c.at < c.end {
		if len(fields) == http3MaxItems {
			return nil, nil, fmt.Errorf("qpack: encoder instruction count exceeds profile")
		}
		s := c.at
		b := wire[s]
		f := http3Field{Name: fmt.Sprintf("Encoder Instruction %d", len(fields)), Start: s * 8, Info: map[string]any{}}
		var e http3QPACKEntry
		switch {
		case b&0x80 != 0:
			f.Info["Instruction"] = "insert with name reference"
			f.Children = append(f.Children, http3Leaf("Opcode", "uint8", s*8, s*8+1), http3Leaf("Static Table", "uint8", s*8+1, s*8+2))
			index, ix, err := c.prefixed("Name Index", 6)
			if err != nil {
				return nil, nil, err
			}
			f.Children = append(f.Children, ix)
			if b&0x40 != 0 {
				e, err = http3QPACKStaticEntry(index)
			} else {
				e, err = t.relative(index)
			}
			if err != nil {
				return nil, nil, err
			}
			value, vf, source, err := c.text("Value", 7, "qpack-encoder-stream-relative-bits")
			if err != nil {
				return nil, nil, err
			}
			f.Children = append(f.Children, vf)
			e.value = value
			e.valueSource = source
		case b&0x40 != 0:
			f.Info["Instruction"] = "insert with literal name"
			f.Children = append(f.Children, http3Leaf("Opcode", "uint8", s*8, s*8+2))
			name, nf, ns, err := c.text("Name", 5, "qpack-encoder-stream-relative-bits")
			if err != nil {
				return nil, nil, err
			}
			value, vf, vs, err := c.text("Value", 7, "qpack-encoder-stream-relative-bits")
			if err != nil {
				return nil, nil, err
			}
			f.Children = append(f.Children, nf, vf)
			e = http3QPACKEntry{name: name, value: value, nameSource: ns, valueSource: vs}
		case b&0x20 != 0:
			f.Info["Instruction"] = "set dynamic table capacity"
			f.Children = append(f.Children, http3Leaf("Opcode", "uint8", s*8, s*8+3))
			capacity, cf, err := c.prefixed("Capacity", 5)
			if err != nil {
				return nil, nil, err
			}
			f.Children = append(f.Children, cf)
			if capacity > max {
				return nil, nil, fmt.Errorf("qpack: capacity exceeds observed peer maximum")
			}
			t.capacity = capacity
			t.evict(capacity)
			f.Info["Capacity"] = capacity
		default:
			f.Info["Instruction"] = "duplicate"
			f.Children = append(f.Children, http3Leaf("Opcode", "uint8", s*8, s*8+3))
			index, ix, err := c.prefixed("Relative Index", 5)
			if err != nil {
				return nil, nil, err
			}
			f.Children = append(f.Children, ix)
			e, err = t.relative(index)
			if err != nil {
				return nil, nil, err
			}
		}
		if b&0xe0 != 0x20 {
			if err := t.insert(e); err != nil {
				return nil, nil, err
			}
			f.Info["Absolute Index"] = t.insertCount - 1
			f.Info["Name"] = e.name
			f.Info["Value"] = e.value
			f.Info["Name Source"] = e.nameSource
			f.Info["Value Source"] = e.valueSource
		}
		f.End = c.at * 8
		fields = append(fields, f)
	}
	return t, fields, nil
}

func http3QPACKSnapshot(wire []byte, max uint64) (*http3QPACKTable, error) {
	if len(wire) == 0 {
		return &http3QPACKTable{max: max}, nil
	}
	c := http3Cursor{wire: wire, end: len(wire)}
	kind, _, err := c.integer("Encoder Stream Type")
	if err != nil {
		return nil, err
	}
	if kind != 2 {
		return nil, fmt.Errorf("qpack: supplied snapshot must begin with encoder stream type 2")
	}
	t, _, err := http3QPACKEncoder(wire, c.at, max)
	return t, err
}

func http3QPACKDecoder(wire []byte, start int) ([]http3Field, error) {
	c := http3Cursor{wire: wire, at: start, end: len(wire)}
	fields := []http3Field{}
	for c.at < c.end {
		if len(fields) == http3MaxItems {
			return nil, fmt.Errorf("qpack: decoder instruction count exceeds profile")
		}
		s := c.at
		b := wire[s]
		prefix, opcodeBits, name := uint(6), 2, "Insert Count Increment"
		if b&0x80 != 0 {
			prefix, opcodeBits, name = 7, 1, "Section Acknowledgment"
		} else if b&0x40 != 0 {
			name = "Stream Cancellation"
		}
		v, vf, err := c.prefixed("Instruction Value", prefix)
		if err != nil {
			return nil, err
		}
		if name == "Insert Count Increment" && v == 0 {
			return nil, fmt.Errorf("qpack: zero Insert Count Increment")
		}
		f := http3Group(fmt.Sprintf("Decoder Instruction %d", len(fields)), s*8, c.at*8, http3Leaf("Opcode", "uint8", s*8, s*8+opcodeBits), vf)
		f.Info = map[string]any{"Instruction": name, "Value": v}
		fields = append(fields, f)
	}
	return fields, nil
}

func http3QPACKSection(wire []byte, start, end int, t *http3QPACKTable) (http3Field, []map[string]any, error) {
	c := http3Cursor{wire: wire, at: start, end: end}
	f := http3Field{Name: "QPACK Field Section", Start: start * 8, End: end * 8}
	fail := func(err error) (http3Field, []map[string]any, error) { return http3Field{}, nil, err }
	encoded, rf, err := c.prefixed("Encoded Required Insert Count", 8)
	if err != nil {
		return fail(err)
	}
	f.Children = append(f.Children, rf)
	required := uint64(0)
	if encoded != 0 {
		entries := t.max / 32
		full := 2 * entries
		if full == 0 || encoded > full {
			return fail(fmt.Errorf("qpack: invalid encoded Required Insert Count"))
		}
		maxValue := t.insertCount + entries
		required = maxValue/full*full + encoded - 1
		if required > maxValue {
			if required <= full {
				return fail(fmt.Errorf("qpack: invalid Required Insert Count wrap"))
			}
			required -= full
		}
		if required == 0 {
			return fail(fmt.Errorf("qpack: zero Required Insert Count must encode zero"))
		}
		if required > t.insertCount {
			return fail(fmt.Errorf("qpack: blocked field section; encoder snapshot lacks required insertions"))
		}
	}
	if c.at >= end {
		return fail(fmt.Errorf("qpack: missing Base"))
	}
	sign := wire[c.at]&128 != 0
	s := c.at
	delta, df, err := c.prefixed("Delta Base", 7)
	if err != nil {
		return fail(err)
	}
	f.Children = append(f.Children, http3Leaf("Base Sign", "uint8", s*8, s*8+1), df)
	base := required + delta
	if sign {
		if delta >= required {
			return fail(fmt.Errorf("qpack: negative Base"))
		}
		base = required - delta - 1
	} else if base > http3MaxInteger {
		return fail(fmt.Errorf("qpack: Base exceeds 62 bits"))
	}
	headers := []map[string]any{}
	decodedBytes, largest := 0, uint64(0)
	dynamic := func(index uint64, post bool) (http3QPACKEntry, error) {
		var abs uint64
		if post {
			if index > http3MaxInteger-base {
				return http3QPACKEntry{}, fmt.Errorf("qpack: post-base index overflow")
			}
			abs = base + index
		} else {
			if index >= base {
				return http3QPACKEntry{}, fmt.Errorf("qpack: relative index underflow")
			}
			abs = base - index - 1
		}
		if abs >= required {
			return http3QPACKEntry{}, fmt.Errorf("qpack: dynamic reference exceeds Required Insert Count")
		}
		if abs+1 > largest {
			largest = abs + 1
		}
		return t.absolute(abs)
	}
	for c.at < end {
		if len(headers) == http3MaxItems {
			return fail(fmt.Errorf("qpack: header count exceeds profile"))
		}
		s := c.at
		b := wire[s]
		hf := http3Field{Name: fmt.Sprintf("Field Line %d", len(headers)), Start: s * 8, Info: map[string]any{}}
		var e http3QPACKEntry
		never := false
		switch {
		case b&0x80 != 0:
			hf.Info["Representation"] = "indexed"
			hf.Children = append(hf.Children, http3Leaf("Opcode", "uint8", s*8, s*8+1), http3Leaf("Static Table", "uint8", s*8+1, s*8+2))
			index, ix, er := c.prefixed("Index", 6)
			if er != nil {
				return fail(er)
			}
			hf.Children = append(hf.Children, ix)
			if b&0x40 != 0 {
				e, err = http3QPACKStaticEntry(index)
			} else {
				e, err = dynamic(index, false)
			}
		case b&0xc0 == 0x40:
			hf.Info["Representation"] = "literal with name reference"
			never = b&0x20 != 0
			hf.Children = append(hf.Children, http3Leaf("Opcode", "uint8", s*8, s*8+2), http3Leaf("Never Index", "uint8", s*8+2, s*8+3), http3Leaf("Static Table", "uint8", s*8+3, s*8+4))
			index, ix, er := c.prefixed("Name Index", 4)
			if er != nil {
				return fail(er)
			}
			hf.Children = append(hf.Children, ix)
			if b&0x10 != 0 {
				e, err = http3QPACKStaticEntry(index)
			} else {
				e, err = dynamic(index, false)
			}
			if err == nil {
				var vf http3Field
				e.value, vf, e.valueSource, err = c.text("Value", 7, "decrypted-stream-relative-bits")
				hf.Children = append(hf.Children, vf)
			}
		case b&0xe0 == 0x20:
			hf.Info["Representation"] = "literal with literal name"
			never = b&0x10 != 0
			hf.Children = append(hf.Children, http3Leaf("Opcode", "uint8", s*8, s*8+3), http3Leaf("Never Index", "uint8", s*8+3, s*8+4))
			var nf, vf http3Field
			e.name, nf, e.nameSource, err = c.text("Name", 3, "decrypted-stream-relative-bits")
			hf.Children = append(hf.Children, nf)
			if err == nil {
				e.value, vf, e.valueSource, err = c.text("Value", 7, "decrypted-stream-relative-bits")
				hf.Children = append(hf.Children, vf)
			}
		case b&0xf0 == 0x10:
			hf.Info["Representation"] = "indexed post-base"
			hf.Children = append(hf.Children, http3Leaf("Opcode", "uint8", s*8, s*8+4))
			index, ix, er := c.prefixed("Post Base Index", 4)
			if er != nil {
				return fail(er)
			}
			hf.Children = append(hf.Children, ix)
			e, err = dynamic(index, true)
		default:
			hf.Info["Representation"] = "literal post-base name"
			never = b&8 != 0
			hf.Children = append(hf.Children, http3Leaf("Opcode", "uint8", s*8, s*8+4), http3Leaf("Never Index", "uint8", s*8+4, s*8+5))
			index, ix, er := c.prefixed("Post Base Name Index", 3)
			if er != nil {
				return fail(er)
			}
			hf.Children = append(hf.Children, ix)
			e, err = dynamic(index, true)
			if err == nil {
				var vf http3Field
				e.value, vf, e.valueSource, err = c.text("Value", 7, "decrypted-stream-relative-bits")
				hf.Children = append(hf.Children, vf)
			}
		}
		if err != nil {
			return fail(err)
		}
		decodedBytes += len(e.name) + len(e.value) + 32
		if decodedBytes > http3MaxBytes {
			return fail(fmt.Errorf("qpack: decoded field section exceeds 1MiB profile"))
		}
		hf.End = c.at * 8
		hf.Info["Name"] = e.name
		hf.Info["Value"] = e.value
		hf.Info["Never Index"] = never
		hf.Info["Name Source"] = e.nameSource
		hf.Info["Value Source"] = e.valueSource
		f.Children = append(f.Children, hf)
		headers = append(headers, map[string]any{"Name": e.name, "Value": e.value, "Never Index": never, "Representation Bit Span": [2]uint64{uint64(s) * 8, uint64(c.at) * 8}, "Name Source": e.nameSource, "Value Source": e.valueSource})
	}
	if largest != required {
		return fail(fmt.Errorf("qpack: Required Insert Count differs from largest dynamic reference"))
	}
	f.Info = map[string]any{"Required Insert Count": required, "Base": base, "Header Count": len(headers), "Decoded Field Section Bytes": decodedBytes}
	return f, headers, nil
}
