package pcaputil

import (
	"fmt"
	"sync"

	"github.com/quic-go/qpack"
	"golang.org/x/net/http2/hpack"
)

// qpackConn is RFC 9204 session state. Encoder streams from direction D
// fill table[D]; HEADERS from D are decoded against that table. SETTINGS
// QPACK_MAX_TABLE_CAPACITY from D limits the peer encoder (1-D).
type qpackConn struct {
	table  [2]qpackTable
	maxCap [2]uint64
}

type qpackEntry struct {
	name, value string
	absolute    uint64
}

type qpackTable struct {
	entries                     []qpackEntry
	insertCount, capacity, size uint64
}

var qpackStaticOnce sync.Once
var qpackStaticTable [99]qpack.HeaderField

func qpackStatic(index uint64) (qpackEntry, error) {
	if index >= 99 {
		return qpackEntry{}, fmt.Errorf("qpack: invalid static table index")
	}
	qpackStaticOnce.Do(func() {
		for i := range qpackStaticTable {
			wire := []byte{0, 0, byte(0xc0 | i)}
			if i >= 63 {
				wire = []byte{0, 0, 0xff, byte(i - 63)}
			}
			fields, err := qpack.NewDecoder(nil).DecodeFull(wire)
			if err != nil || len(fields) != 1 {
				panic("qpack: static table decoder contract changed")
			}
			qpackStaticTable[i] = fields[0]
		}
	})
	f := qpackStaticTable[index]
	return qpackEntry{name: f.Name, value: f.Value}, nil
}

func (t *qpackTable) evict(limit uint64) {
	for t.size > limit && len(t.entries) > 0 {
		e := t.entries[0]
		t.size -= uint64(len(e.name) + len(e.value) + 32)
		t.entries = t.entries[1:]
	}
}

func (t *qpackTable) insert(e qpackEntry, maxItems int) error {
	size := uint64(len(e.name) + len(e.value) + 32)
	if size > t.capacity {
		return protocolError(ErrResourceExceeded, "qpack: entry exceeds dynamic table capacity")
	}
	if int(t.insertCount) >= maxItems {
		return protocolError(ErrResourceExceeded, "qpack: insert count exceeds budget")
	}
	t.evict(t.capacity - size)
	e.absolute = t.insertCount
	t.insertCount++
	t.size += size
	t.entries = append(t.entries, e)
	return nil
}

func (t *qpackTable) absolute(index uint64) (qpackEntry, error) {
	if len(t.entries) == 0 || index < t.entries[0].absolute || index >= t.insertCount {
		return qpackEntry{}, fmt.Errorf("qpack: absent or evicted dynamic table index")
	}
	return t.entries[index-t.entries[0].absolute], nil
}

func (t *qpackTable) relative(index uint64) (qpackEntry, error) {
	if index >= t.insertCount {
		return qpackEntry{}, fmt.Errorf("qpack: dynamic relative index underflow")
	}
	return t.absolute(t.insertCount - index - 1)
}

func qpackPrefixed(b []byte, prefix uint) (uint64, int, error) {
	if len(b) == 0 || prefix == 0 || prefix > 8 {
		return 0, 1, quicMoreError{1}
	}
	mask := uint64(1<<prefix - 1)
	v := uint64(b[0]) & mask
	n := 1
	if v < mask {
		return v, 1, nil
	}
	for shift := uint(0); ; shift += 7 {
		if n >= len(b) {
			return 0, n + 1, quicMoreError{n + 1}
		}
		if shift >= 63 {
			return 0, n, fmt.Errorf("qpack: integer exceeds 62 bits")
		}
		c := b[n]
		n++
		v += uint64(c&127) << shift
		if c&128 == 0 {
			return v, n, nil
		}
	}
}

func qpackString(b []byte, prefix uint) (string, int, error) {
	if len(b) == 0 {
		return "", 1, quicMoreError{1}
	}
	huffman := b[0]&(1<<prefix) != 0
	nlen, n, err := qpackPrefixed(b, prefix)
	if err != nil {
		return "", n, err
	}
	if nlen > 1<<16 {
		return "", n, protocolError(ErrResourceExceeded, "qpack: string exceeds 64 KiB")
	}
	if uint64(len(b)-n) < nlen {
		return "", n + int(nlen), quicMoreError{n + int(nlen)}
	}
	raw := b[n : n+int(nlen)]
	n += int(nlen)
	if !huffman {
		return string(raw), n, nil
	}
	s, err := hpack.HuffmanDecodeToString(raw)
	if err != nil {
		return "", n, fmt.Errorf("qpack: invalid Huffman string: %w", err)
	}
	return s, n, nil
}

func (c *qpackConn) applyEncoder(dir int, buf []byte, maxItems int) (int, []map[string]any, error) {
	if dir != 0 && dir != 1 {
		dir = 0
	}
	t := &c.table[dir]
	max := c.maxCap[dir]
	if len(buf) > 0 && max == 0 {
		return 0, nil, protocolError(ErrContextRequired, "qpack: encoder instructions require observed peer maximum capacity")
	}
	off := 0
	var inst []map[string]any
	for off < len(buf) {
		b := buf[off]
		item := map[string]any{}
		var e qpackEntry
		n := 0
		switch {
		case b&0x80 != 0:
			item["Instruction"] = "Insert With Name Reference"
			idx, k, err := qpackPrefixed(buf[off:], 6)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n = k
			if b&0x40 != 0 {
				e, err = qpackStatic(idx)
			} else {
				e, err = t.relative(idx)
			}
			if err != nil {
				return off, inst, err
			}
			val, k, err := qpackString(buf[off+n:], 7)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n += k
			e.value = val
		case b&0x40 != 0:
			item["Instruction"] = "Insert With Literal Name"
			name, k, err := qpackString(buf[off:], 5)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n = k
			val, k, err := qpackString(buf[off+n:], 7)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n += k
			e = qpackEntry{name: name, value: val}
		case b&0x20 != 0:
			item["Instruction"] = "Set Dynamic Table Capacity"
			capv, k, err := qpackPrefixed(buf[off:], 5)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n = k
			if capv > max {
				return off, inst, protocolError(ErrResourceExceeded, "qpack: capacity exceeds observed peer maximum")
			}
			t.capacity = capv
			t.evict(capv)
			item["Capacity"] = capv
		default:
			item["Instruction"] = "Duplicate"
			idx, k, err := qpackPrefixed(buf[off:], 5)
			if err != nil {
				if quicNeedMore(err) {
					return off, inst, nil
				}
				return off, inst, err
			}
			n = k
			e, err = t.relative(idx)
			if err != nil {
				return off, inst, err
			}
		}
		if b&0xe0 != 0x20 {
			if err := t.insert(e, maxItems); err != nil {
				return off, inst, err
			}
			item["Name"] = e.name
			item["Value"] = e.value
			item["Absolute Index"] = t.insertCount - 1
		}
		inst = append(inst, item)
		off += n
		if len(inst) >= maxItems {
			return off, inst, protocolError(ErrResourceExceeded, "qpack: encoder instruction budget exceeded")
		}
	}
	return off, inst, nil
}

func (c *qpackConn) applyDecoder(buf []byte, maxItems int) (int, []map[string]any, error) {
	off := 0
	var inst []map[string]any
	for off < len(buf) {
		b := buf[off]
		prefix, name := uint(6), "Insert Count Increment"
		if b&0x80 != 0 {
			prefix, name = 7, "Section Acknowledgment"
		} else if b&0x40 != 0 {
			name = "Stream Cancellation"
		}
		v, n, err := qpackPrefixed(buf[off:], prefix)
		if err != nil {
			if quicNeedMore(err) {
				return off, inst, nil
			}
			return off, inst, err
		}
		if name == "Insert Count Increment" && v == 0 {
			return off, inst, protocolError(ErrMalformedMessage, "qpack: zero Insert Count Increment")
		}
		inst = append(inst, map[string]any{"Instruction": name, "Value": v})
		off += n
		if len(inst) >= maxItems {
			return off, inst, protocolError(ErrResourceExceeded, "qpack: decoder instruction budget exceeded")
		}
	}
	return off, inst, nil
}

func (c *qpackConn) decodeSection(dir int, payload []byte) ([]map[string]any, map[string]any, error) {
	if dir != 0 && dir != 1 {
		dir = 0
	}
	t := &c.table[dir]
	if len(payload) == 0 {
		return nil, nil, fmt.Errorf("qpack: empty field section")
	}
	encoded, n, err := qpackPrefixed(payload, 8)
	if err != nil {
		return nil, nil, err
	}
	off := n
	required := uint64(0)
	maxCap := c.maxCap[dir]
	if maxCap == 0 {
		maxCap = t.capacity
	}
	if encoded != 0 {
		entries := maxCap / 32
		full := 2 * entries
		if full == 0 || encoded > full {
			return nil, nil, fmt.Errorf("qpack: invalid encoded Required Insert Count")
		}
		maxValue := t.insertCount + entries
		required = maxValue/full*full + encoded - 1
		if required > maxValue {
			if required <= full {
				return nil, nil, fmt.Errorf("qpack: invalid Required Insert Count wrap")
			}
			required -= full
		}
		if required == 0 {
			return nil, nil, fmt.Errorf("qpack: zero Required Insert Count must encode zero")
		}
		if required > t.insertCount {
			return nil, nil, protocolError(ErrContextRequired, "qpack: blocked field section; encoder snapshot lacks required insertions")
		}
	}
	if off >= len(payload) {
		return nil, nil, fmt.Errorf("qpack: missing Base")
	}
	sign := payload[off]&128 != 0
	delta, n, err := qpackPrefixed(payload[off:], 7)
	if err != nil {
		return nil, nil, err
	}
	off += n
	base := required + delta
	if sign {
		if delta >= required {
			return nil, nil, fmt.Errorf("qpack: negative Base")
		}
		base = required - delta - 1
	}
	dynamic := func(index uint64, post bool) (qpackEntry, error) {
		var abs uint64
		if post {
			abs = base + index
		} else {
			if index >= base {
				return qpackEntry{}, fmt.Errorf("qpack: relative index underflow")
			}
			abs = base - index - 1
		}
		if abs >= required {
			return qpackEntry{}, fmt.Errorf("qpack: dynamic reference exceeds Required Insert Count")
		}
		return t.absolute(abs)
	}
	var headers []map[string]any
	largest := uint64(0)
	note := func(abs uint64) {
		if abs+1 > largest {
			largest = abs + 1
		}
	}
	for off < len(payload) {
		b := payload[off]
		var e qpackEntry
		never := false
		switch {
		case b&0x80 != 0:
			idx, k, er := qpackPrefixed(payload[off:], 6)
			if er != nil {
				return nil, nil, er
			}
			off += k
			if b&0x40 != 0 {
				e, err = qpackStatic(idx)
			} else {
				e, err = dynamic(idx, false)
				if err == nil {
					note(base - idx - 1)
				}
			}
		case b&0xc0 == 0x40:
			never = b&0x20 != 0
			idx, k, er := qpackPrefixed(payload[off:], 4)
			if er != nil {
				return nil, nil, er
			}
			off += k
			if b&0x10 != 0 {
				e, err = qpackStatic(idx)
			} else {
				e, err = dynamic(idx, false)
				if err == nil {
					note(base - idx - 1)
				}
			}
			if err == nil {
				val, k, er := qpackString(payload[off:], 7)
				if er != nil {
					return nil, nil, er
				}
				off += k
				e.value = val
			}
		case b&0xe0 == 0x20:
			never = b&0x10 != 0
			name, k, er := qpackString(payload[off:], 3)
			if er != nil {
				return nil, nil, er
			}
			off += k
			val, k, er := qpackString(payload[off:], 7)
			if er != nil {
				return nil, nil, er
			}
			off += k
			e = qpackEntry{name: name, value: val}
		case b&0xf0 == 0x10:
			idx, k, er := qpackPrefixed(payload[off:], 4)
			if er != nil {
				return nil, nil, er
			}
			off += k
			e, err = dynamic(idx, true)
			if err == nil {
				note(base + idx)
			}
		default:
			never = b&8 != 0
			idx, k, er := qpackPrefixed(payload[off:], 3)
			if er != nil {
				return nil, nil, er
			}
			off += k
			e, err = dynamic(idx, true)
			if err == nil {
				note(base + idx)
				val, k, er := qpackString(payload[off:], 7)
				if er != nil {
					return nil, nil, er
				}
				off += k
				e.value = val
			}
		}
		if err != nil {
			return nil, nil, err
		}
		headers = append(headers, map[string]any{"Name": e.name, "Value": e.value, "Never Index": never})
	}
	if largest != required {
		return nil, nil, fmt.Errorf("qpack: Required Insert Count differs from largest dynamic reference")
	}
	meta := map[string]any{"Required Insert Count": required, "Base": base, "Header Count": len(headers)}
	return headers, meta, nil
}
