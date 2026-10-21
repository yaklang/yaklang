package pcaputil

import (
	"bytes"
	"encoding/binary"
	"math"
	"unicode/utf8"
)

// The selected Sparkplug3 array encoding follows fixed normative prose and
// the corresponding Java codec, not inconsistent PDF examples or Python bugs.
// Shape validation allocates no element graph and never trusts a declaration
// to size an allocation. The caller charges aggregate elements and graph cost
// before calling sparkArrayValue.
func sparkArrayShape(w []byte, typ uint64) (int, string) {
	width := 1
	switch typ {
	case 22, 26:
	case 23, 27:
		width = 2
	case 24, 28, 30:
		width = 4
	case 25, 29, 31, 34:
		width = 8
	case 32:
		if len(w) < 4 {
			return 0, "Boolean array requires a four-byte element count"
		}
		n := uint64(binary.LittleEndian.Uint32(w))
		if (n+7)/8 != uint64(len(w)-4) {
			return 0, "Boolean array data length differs from element count"
		}
		// Unused low bits in the final octet carry no element and are not rejected.
		return int(n), ""
	case 33:
		count := 0
		for len(w) > 0 {
			end := bytes.IndexByte(w, 0)
			if end < 0 || !utf8.Valid(w[:end]) {
				return 0, "String array requires NUL-terminated UTF-8 elements"
			}
			count++
			w = w[end+1:]
		}
		return count, ""
	default:
		return 0, "datatype needs an independent structured profile"
	}
	if len(w)%width != 0 {
		return 0, "Array byte length differs from element width"
	}
	if typ == 34 {
		for i := 0; i < len(w); i += 8 {
			if binary.LittleEndian.Uint64(w[i:]) > math.MaxInt64 {
				return 0, "DateTime array values outside nonnegative int64 need an independent structured profile"
			}
		}
	}
	return len(w) / width, ""
}

func sparkArrayValue(w []byte, typ uint64) ([]any, string) {
	n, why := sparkArrayShape(w, typ)
	if why != "" {
		return nil, why
	}
	values := make([]any, 0, n)
	if typ == 32 {
		for i := 0; i < n; i++ {
			values = append(values, w[4+i/8]&(0x80>>uint(i%8)) != 0)
		}
		return values, ""
	}
	if typ == 33 {
		for len(w) > 0 {
			end := bytes.IndexByte(w, 0)
			values = append(values, string(w[:end]))
			w = w[end+1:]
		}
		return values, ""
	}
	width := 1
	switch typ {
	case 23, 27:
		width = 2
	case 24, 28, 30:
		width = 4
	case 25, 29, 31, 34:
		width = 8
	}
	for i := 0; i < len(w); i += width {
		b := w[i:]
		switch typ {
		case 22:
			values = append(values, int64(int8(b[0])))
		case 23:
			values = append(values, int64(int16(binary.LittleEndian.Uint16(b))))
		case 24:
			values = append(values, int64(int32(binary.LittleEndian.Uint32(b))))
		case 25:
			values = append(values, int64(binary.LittleEndian.Uint64(b)))
		case 26:
			values = append(values, uint64(b[0]))
		case 27:
			values = append(values, uint64(binary.LittleEndian.Uint16(b)))
		case 28:
			values = append(values, uint64(binary.LittleEndian.Uint32(b)))
		case 29, 34:
			values = append(values, binary.LittleEndian.Uint64(b))
		case 30:
			values = append(values, math.Float32frombits(binary.LittleEndian.Uint32(b)))
		case 31:
			values = append(values, math.Float64frombits(binary.LittleEndian.Uint64(b)))
		}
	}
	return values, ""
}

// Charge every selected array in one message before allocating any typed
// element slice. Omitted types resolve only against this flow's definitions;
// this resource pass cannot create a Birth, association or output value.
func (m *binMQTT) sparkArrayGraph(p map[string]any, verb, device string, wire []byte, maxDepth, maxFields int, reserve func(int64) error) error {
	ms, _ := p["metrics"].([]map[string]any)
	var elements, cost int64
	for _, metric := range ms {
		if metric["is_null"] == true || metric["metadata"] != nil || metric["properties"] != nil {
			continue
		}
		typ, typed := metric["datatype"].(uint64)
		if !typed {
			if verb != "NDATA" && verb != "DDATA" && verb != "NCMD" && verb != "DCMD" {
				continue
			}
			definitions := m.spark.metrics
			aliases := m.spark.aliases
			if device != "" {
				d := m.spark.devices[device]
				if d == nil {
					continue
				}
				definitions, aliases = d.metrics, d.aliases
			}
			name, _ := metric["name"].(string)
			if alias, ok := metric["alias"].(uint64); ok {
				name = aliases[alias]
			}
			def, ok := definitions[name]
			if !ok {
				continue
			}
			typ = def.typ
		}
		if typ < 22 || typ > 34 {
			continue
		}
		raw, ok := metric["bytes_value"].([]byte)
		if !ok {
			continue
		}
		n, why := sparkArrayShape(raw, typ)
		if why != "" {
			continue
		} // semantic refusal never allocates a typed graph
		if int64(n) > int64(maxFields)-elements {
			return protocolError(ErrResourceExceeded, "Sparkplug array elements")
		}
		elements += int64(n)
		cost += 512 + 256*int64(n) + 8*int64(len(raw))
	}
	if cost == 0 {
		return nil
	}
	dataSetCost, fields, err := sparkPreflightWithCount(wire, maxDepth, maxFields)
	if err != nil {
		return err
	}
	if elements > int64(maxFields)-int64(fields) {
		return protocolError(ErrResourceExceeded, "Sparkplug array elements")
	}
	return reserve(m.retainedBytes() + dataSetCost + cost)
}
