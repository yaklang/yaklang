package amd64

import (
	"encoding/binary"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type ehFrameCIE struct {
	offset      int
	codeAlign   uint64
	dataAlign   int64
	returnReg   uint64
	encoding    byte
	hasEncoding bool
}

func (s *amd64BoundaryScanner) addEHFrame() {
	data := s.hints.EHFrame
	if len(data) == 0 {
		return
	}
	base := s.hints.EHFrameAddr
	cies := make(map[int]ehFrameCIE)
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
			cie, ok := parseIntCIE(data, entryStart, idOffset, header)
			if ok {
				cies[entryStart] = cie
			}
			offset = entryEnd
			continue
		}
		// In .eh_frame the CIE pointer is relative to its own field.
		cieOffset := idOffset - int(id)
		cie, ok := cies[cieOffset]
		if !ok {
			cieOffset = entryStart - int(id)
			cie, ok = cies[cieOffset]
		}
		if !ok || !cie.hasEncoding {
			offset = entryEnd
			continue
		}
		pos := idOffset + 4
		location, next, ok := readEHFrameEncoded(data, pos, cie.encoding, base+uint64(pos), base)
		if !ok {
			offset = entryEnd
			continue
		}
		pos = next
		rangeValue, _, ok := readEHFrameEncoded(data, pos, cie.encoding&0x0f, 0, base)
		if !ok || rangeValue == 0 {
			offset = entryEnd
			continue
		}
		s.add(location, uint64(rangeValue), "", core.BoundaryEHFrame, priorityEHFrame)
		offset = entryEnd
	}
}

func parseIntCIE(data []byte, entryStart, idOffset, header int) (ehFrameCIE, bool) {
	pos := idOffset + 4
	if pos >= len(data) {
		return ehFrameCIE{}, false
	}
	version := data[pos]
	pos++
	augEnd := pos
	for augEnd < len(data) && data[augEnd] != 0 {
		augEnd++
	}
	if augEnd >= len(data) {
		return ehFrameCIE{}, false
	}
	augmentation := string(data[pos:augEnd])
	pos = augEnd + 1
	codeAlign, pos, ok := readULEB(data, pos)
	if !ok {
		return ehFrameCIE{}, false
	}
	dataAlign, pos, ok := readSLEB(data, pos)
	if !ok {
		return ehFrameCIE{}, false
	}
	var returnReg uint64
	if version == 1 {
		if pos >= len(data) {
			return ehFrameCIE{}, false
		}
		returnReg = uint64(data[pos])
		pos++
	} else {
		returnReg, pos, ok = readULEB(data, pos)
		if !ok {
			return ehFrameCIE{}, false
		}
	}
	cie := ehFrameCIE{offset: entryStart, codeAlign: codeAlign, dataAlign: dataAlign, returnReg: returnReg}
	if len(augmentation) > 0 && augmentation[0] == 'z' {
		length, next, ok := readULEB(data, pos)
		if !ok || next+int(length) > len(data) {
			return ehFrameCIE{}, false
		}
		pos = next
		end := pos + int(length)
		for index := 1; index < len(augmentation) && pos < end; index++ {
			switch augmentation[index] {
			case 'R':
				if pos >= end {
					return ehFrameCIE{}, false
				}
				cie.encoding = data[pos]
				cie.hasEncoding = true
				pos++
			case 'P':
				if pos >= end {
					return ehFrameCIE{}, false
				}
				encoding := data[pos]
				pos++
				_, next, ok := readEHFrameEncoded(data, pos, encoding, 0, 0)
				if !ok || next > end {
					return ehFrameCIE{}, false
				}
				pos = next
			case 'L':
				pos++
			case 'S', 'B':
				// No payload.
			default:
				pos = end
			}
		}
	}
	if !cie.hasEncoding {
		cie.encoding = 0
	}
	return cie, true
}

func readEHFrameEncoded(data []byte, pos int, encoding byte, fieldAddress, base uint64) (uint64, int, bool) {
	format := encoding & 0x0f
	var value uint64
	switch format {
	case 0x00: // absptr
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
	case 0x04:
		if pos+8 > len(data) {
			return 0, pos, false
		}
		value = binary.LittleEndian.Uint64(data[pos:])
		pos += 8
	case 0x09:
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
	case 0x0c:
		if pos+8 > len(data) {
			return 0, pos, false
		}
		value = binary.LittleEndian.Uint64(data[pos:])
		pos += 8
	case 0xff:
		return 0, pos, true
	default:
		return 0, pos, false
	}
	switch encoding & 0x70 {
	case 0x10: // pcrel
		value += fieldAddress
	case 0x20: // textrel
		value += base
	case 0x30: // datarel
		value += base
	case 0x40: // funcrel
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
	var current byte
	for pos < len(data) {
		current = data[pos]
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
