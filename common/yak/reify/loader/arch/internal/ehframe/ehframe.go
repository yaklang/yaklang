// Package ehframe parses DWARF call-frame information into function ranges.
// It reads CIE/FDE headers and address ranges only; it does not execute CFI
// instructions. Both the AMD64 and the ARM64 boundary discoverers use it.
package ehframe

import "encoding/binary"

// Entry is one FDE: a function range in image addresses.
type Entry struct {
	Location uint64
	Range    uint64
}

type cie struct {
	encoding    byte
	hasEncoding bool
}

// Entries returns every FDE range in data. base is the image address of
// data[0] and resolves text-relative encodings.
func Entries(data []byte, base uint64) []Entry {
	if len(data) == 0 {
		return nil
	}
	result := make([]Entry, 0, 64)
	cies := make(map[int]cie)
	for offset := 0; offset+4 <= len(data); {
		length := int(binary.LittleEndian.Uint32(data[offset:]))
		entryStart := offset
		header := 4
		if length == 0 {
			break
		}
		if length == 0xffffffff {
			if offset+12 > len(data) {
				break
			}
			length = int(binary.LittleEndian.Uint64(data[offset+4:]))
			header = 12
		}
		if length < 4 || offset+header+length > len(data) {
			break
		}
		idOffset := offset + header
		id := binary.LittleEndian.Uint32(data[idOffset:])
		entryEnd := idOffset + length
		if id == 0 {
			if parsed, ok := parseCIE(data, idOffset); ok {
				cies[entryStart] = parsed
			}
			offset = entryEnd
			continue
		}
		// In .eh_frame the CIE pointer is relative to its own field.
		pointer, ok := cies[idOffset-int(id)]
		if !ok {
			pointer, ok = cies[entryStart-int(id)]
		}
		if !ok || !pointer.hasEncoding {
			offset = entryEnd
			continue
		}
		pos := idOffset + 4
		location, next, ok := readEncoded(data, pos, pointer.encoding, base+uint64(pos), base)
		if !ok {
			offset = entryEnd
			continue
		}
		pos = next
		size, _, ok := readEncoded(data, pos, pointer.encoding&0x0f, 0, base)
		if ok && size != 0 && location != 0 {
			result = append(result, Entry{Location: location, Range: size})
		}
		offset = entryEnd
	}
	return result
}

func parseCIE(data []byte, idOffset int) (cie, bool) {
	pos := idOffset + 4
	if pos >= len(data) {
		return cie{}, false
	}
	version := data[pos]
	pos++
	augEnd := pos
	for augEnd < len(data) && data[augEnd] != 0 {
		augEnd++
	}
	if augEnd >= len(data) {
		return cie{}, false
	}
	augmentation := string(data[pos:augEnd])
	pos = augEnd + 1
	if _, next, ok := readULEB(data, pos); ok {
		pos = next
	} else {
		return cie{}, false
	}
	if _, next, ok := readSLEB(data, pos); ok {
		pos = next
	} else {
		return cie{}, false
	}
	if version == 1 {
		if pos >= len(data) {
			return cie{}, false
		}
		pos++
	} else if _, next, ok := readULEB(data, pos); ok {
		pos = next
	} else {
		return cie{}, false
	}
	entry := cie{}
	if len(augmentation) == 0 || augmentation[0] != 'z' {
		return entry, true
	}
	length, next, ok := readULEB(data, pos)
	if !ok || next+int(length) > len(data) {
		return cie{}, false
	}
	pos = next
	end := pos + int(length)
	for index := 1; index < len(augmentation) && pos < end; index++ {
		switch augmentation[index] {
		case 'R':
			if pos >= end {
				return cie{}, false
			}
			entry.encoding = data[pos]
			entry.hasEncoding = true
			pos++
		case 'P':
			if pos >= end {
				return cie{}, false
			}
			encoding := data[pos]
			pos++
			if _, next, ok := readEncoded(data, pos, encoding, 0, 0); ok && next <= end {
				pos = next
			} else {
				return cie{}, false
			}
		case 'L':
			pos++
		case 'S', 'B':
			// No payload.
		default:
			pos = end
		}
	}
	return entry, true
}

func readEncoded(data []byte, pos int, encoding byte, fieldAddress, base uint64) (uint64, int, bool) {
	var value uint64
	switch encoding & 0x0f {
	case 0x00, 0x04, 0x0c: // absptr, udata8, sdata8
		if pos+8 > len(data) {
			return 0, pos, false
		}
		value = binary.LittleEndian.Uint64(data[pos:])
		pos += 8
	case 0x01: // uleb128
		raw, next, ok := readULEB(data, pos)
		if !ok {
			return 0, pos, false
		}
		value, pos = raw, next
	case 0x02:
		if pos+2 > len(data) {
			return 0, pos, false
		}
		value = uint64(binary.LittleEndian.Uint16(data[pos:]))
		pos += 2
	case 0x03:
		if pos+4 > len(data) {
			return 0, pos, false
		}
		value = uint64(binary.LittleEndian.Uint32(data[pos:]))
		pos += 4
	case 0x09: // sleb128
		raw, next, ok := readSLEB(data, pos)
		if !ok {
			return 0, pos, false
		}
		value, pos = uint64(raw), next
	case 0x0a:
		if pos+2 > len(data) {
			return 0, pos, false
		}
		value = uint64(int64(int16(binary.LittleEndian.Uint16(data[pos:]))))
		pos += 2
	case 0x0b:
		if pos+4 > len(data) {
			return 0, pos, false
		}
		value = uint64(int64(int32(binary.LittleEndian.Uint32(data[pos:]))))
		pos += 4
	case 0x0f:
		return 0, pos, true
	default:
		return 0, pos, false
	}
	switch encoding & 0x70 {
	case 0x10: // pcrel
		value += fieldAddress
	case 0x20, 0x30, 0x40: // textrel, datarel, funcrel
		value += base
	}
	return value, pos, true
}

func readULEB(data []byte, pos int) (uint64, int, bool) {
	var value uint64
	var shift uint
	for pos < len(data) {
		current := data[pos]
		pos++
		value |= uint64(current&0x7f) << shift
		if current&0x80 == 0 {
			return value, pos, true
		}
		shift += 7
		if shift >= 64 {
			return 0, pos, false
		}
	}
	return 0, pos, false
}

func readSLEB(data []byte, pos int) (int64, int, bool) {
	var value int64
	var shift uint
	for pos < len(data) {
		current := data[pos]
		pos++
		value |= int64(current&0x7f) << shift
		shift += 7
		if current&0x80 == 0 {
			if shift < 64 && current&0x40 != 0 {
				value |= -1 << shift
			}
			return value, pos, true
		}
		if shift >= 64 {
			return 0, pos, false
		}
	}
	return 0, pos, false
}
