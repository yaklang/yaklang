package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Header and Body contain literal observations. The flat keys retained for
// callers have the same meaning; Effective/Request/Connection keys are context
// and never receive a fabricated wire range.
func smb2HeaderFields(info map[string]any, pdu []byte, resp bool, flags uint32) {
	h := map[string]any{"Wire Field Ranges": map[string]any{}}
	field := func(key string, lo, hi int, value any) { smb2Literal(h, key, lo, hi, value) }
	field("Protocol ID", 0, 4, binary.LittleEndian.Uint32(pdu[:4]))
	field("Structure Size", 4, 6, binary.LittleEndian.Uint16(pdu[4:6]))
	field("Credit Charge", 6, 8, binary.LittleEndian.Uint16(pdu[6:8]))
	if resp {
		field("Status", 8, 12, binary.LittleEndian.Uint32(pdu[8:12]))
	} else {
		field("Channel Sequence", 8, 10, binary.LittleEndian.Uint16(pdu[8:10]))
		field("Reserved", 10, 12, binary.LittleEndian.Uint16(pdu[10:12]))
	}
	field("Command", 12, 14, binary.LittleEndian.Uint16(pdu[12:14]))
	field("Credit", 14, 16, binary.LittleEndian.Uint16(pdu[14:16]))
	field("Flags", 16, 20, flags)
	field("Next Command", 20, 24, binary.LittleEndian.Uint32(pdu[20:24]))
	field("Message ID", 24, 32, binary.LittleEndian.Uint64(pdu[24:32]))
	if flags&smb2FlagAsync != 0 {
		field("Async ID", 32, 40, binary.LittleEndian.Uint64(pdu[32:40]))
	} else {
		field("Process ID", 32, 36, binary.LittleEndian.Uint32(pdu[32:36]))
		field("Tree ID", 36, 40, binary.LittleEndian.Uint32(pdu[36:40]))
	}
	field("Session ID", 40, 48, binary.LittleEndian.Uint64(pdu[40:48]))
	field("Signature", 48, 64, bytes.Clone(pdu[48:64]))
	for key, value := range h {
		info[key] = value
	}
	info["Header"] = cloneSession(h)
}

func smb2Literal(m map[string]any, key string, lo, hi int, value any) {
	ranges, _ := m["Wire Field Ranges"].(map[string]any)
	if ranges == nil {
		ranges = map[string]any{}
		m["Wire Field Ranges"] = ranges
	}
	m[key] = value
	ranges[key] = []any{uint64(lo), uint64(hi)}
}

// Parse helpers use offsets relative to the command's SMB2 header. The walker
// translates them once to the complete owned Direct TCP PDU, including each
// compound command's actual position.
func smb2OffsetRanges(m map[string]any, base int) {
	for key, value := range m {
		if key == "Wire Field Ranges" {
			if ranges, ok := value.(map[string]any); ok {
				for name, value := range ranges {
					if span, ok := value.([]any); ok && len(span) == 2 {
						lo, lok := displayUint(span[0])
						hi, hok := displayUint(span[1])
						if lok && hok {
							ranges[name] = []any{lo + uint64(base), hi + uint64(base)}
						}
					}
				}
			}
			continue
		}
		switch v := value.(type) {
		case map[string]any:
			smb2OffsetRanges(v, base)
		case []map[string]any:
			for _, child := range v {
				smb2OffsetRanges(child, base)
			}
		case []any:
			for _, child := range v {
				if child, ok := child.(map[string]any); ok {
					smb2OffsetRanges(child, base)
				}
			}
		}
	}
}

func smb2EffectiveIDs(c map[string]any) (uint64, uint32) {
	sid, _ := c["Session ID"].(uint64)
	tid, _ := c["Tree ID"].(uint32)
	if effective, ok := c["Effective Session ID"].(uint64); ok {
		sid = effective
	}
	if effective, ok := c["Effective Tree ID"].(uint32); ok {
		tid = effective
	}
	return sid, tid
}

func smb2Body(pdu []byte, minimum int, structure uint16) (map[string]any, error) {
	if len(pdu) < smb2HeaderSize+minimum {
		return nil, fmt.Errorf("smb2: truncated body (requires %d bytes)", minimum)
	}
	if binary.LittleEndian.Uint16(pdu[64:66]) != structure {
		return nil, fmt.Errorf("smb2: body StructureSize must be %d", structure)
	}
	m := map[string]any{}
	smb2Literal(m, "Structure Size", 64, 66, structure)
	return m, nil
}

func smb2Buffer(pdu []byte, offset, length, minimum int) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	if offset < minimum || offset > len(pdu) || length > len(pdu)-offset {
		return nil, fmt.Errorf("smb2: variable buffer outside command")
	}
	return pdu[offset : offset+length], nil
}

// A non-success NEGOTIATE/CREATE/CLOSE has the error layout, not the command's
// success layout. In particular Error.ByteCount is not a selected dialect and
// a long ErrorData blob does not contain a CREATE FileId.
func smb2ErrorFields(info map[string]any, pdu []byte) error {
	body, err := smb2Body(pdu, 8, 9)
	if err != nil {
		return err
	}
	n := uint64(binary.LittleEndian.Uint32(pdu[68:72]))
	if n > uint64(len(pdu)-72) {
		return fmt.Errorf("smb2: error ByteCount exceeds command")
	}
	smb2Literal(body, "Error Context Count", 66, 67, pdu[66])
	smb2Literal(body, "Reserved", 67, 68, pdu[67])
	smb2Literal(body, "Byte Count", 68, 72, uint32(n))
	smb2Literal(body, "Error Data", 72, 72+int(n), bytes.Clone(pdu[72:72+int(n)]))
	info["Body"], info["Error Response"] = body, true
	return nil
}

func smb2NegotiateFields(info map[string]any, pdu []byte, response bool, max int) error {
	if response && info["Status"] != uint32(0) {
		return smb2ErrorFields(info, pdu)
	}
	minimum, structure := 36, uint16(36)
	if response {
		minimum, structure = 64, 65
	}
	body, err := smb2Body(pdu, minimum, structure)
	if err != nil {
		return err
	}
	info["Body"] = body
	b := pdu[64:]
	if !response {
		count := int(binary.LittleEndian.Uint16(b[2:4]))
		if count == 0 || count > (len(b)-36)/2 {
			return fmt.Errorf("smb2: incomplete or empty negotiate dialect list")
		}
		if count > max {
			return protocolError(ErrResourceExceeded, "SMB2 dialect count exceeds collection budget")
		}
		smb2Literal(body, "Dialect Count", 66, 68, uint16(count))
		dialects := make([]uint16, count)
		entries := make([]map[string]any, count)
		for i := range dialects {
			dialects[i] = binary.LittleEndian.Uint16(b[36+2*i:])
			entries[i] = map[string]any{}
			smb2Literal(entries[i], "Dialect", 100+2*i, 102+2*i, dialects[i])
		}
		info["Dialects"], body["Dialects"] = dialects, entries
		for _, dialect := range dialects {
			if dialect == 0x0311 {
				return smb2NegotiateContextFields(body, pdu, int(binary.LittleEndian.Uint16(b[32:34])), uint64(binary.LittleEndian.Uint32(b[28:32])), 100+2*count, max, 0, 0)
			}
		}
		return nil
	}
	dialect := binary.LittleEndian.Uint16(b[4:6])
	smb2Literal(body, "Security Mode", 66, 68, binary.LittleEndian.Uint16(b[2:4]))
	smb2Literal(body, "Dialect", 68, 70, dialect)
	info["Dialect"], info["Version Name"] = dialect, smb2DialectName(dialect)
	smb2Literal(info, "Dialect", 68, 70, dialect)
	offset, length := int(binary.LittleEndian.Uint16(b[56:58])), int(binary.LittleEndian.Uint16(b[58:60]))
	security, err := smb2Buffer(pdu, offset, length, 128)
	if err != nil {
		return err
	}
	smb2Literal(body, "Security Buffer Offset", 120, 122, uint16(offset))
	smb2Literal(body, "Security Buffer Length", 122, 124, uint16(length))
	if length != 0 {
		smb2Literal(body, "Security Buffer", offset, offset+length, bytes.Clone(security))
	}
	// Only the 3.1.1 layout interprets these reserved fields as contexts.
	if dialect == 0x0311 {
		count := int(binary.LittleEndian.Uint16(b[6:8]))
		at := uint64(binary.LittleEndian.Uint32(b[60:64]))
		return smb2NegotiateContextFields(body, pdu, count, at, 128, max, offset, length)
	}
	return nil
}

func smb2NegotiateContextFields(body map[string]any, pdu []byte, count int, at uint64, minimum, max, securityOffset, securityLength int) error {
	if count > max {
		return protocolError(ErrResourceExceeded, "SMB2 negotiate context count exceeds collection budget")
	}
	if count != 0 && (at < uint64(minimum) || at%8 != 0 || at > uint64(len(pdu))) {
		return fmt.Errorf("smb2: invalid negotiate context offset")
	}
	contexts := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		if at > uint64(len(pdu)) || uint64(len(pdu))-at < 8 {
			return fmt.Errorf("smb2: truncated negotiate context header")
		}
		length := uint64(binary.LittleEndian.Uint16(pdu[at+2 : at+4]))
		if length > uint64(len(pdu))-at-8 {
			return fmt.Errorf("smb2: truncated negotiate context data")
		}
		end := at + 8 + length
		if securityLength != 0 && at < uint64(securityOffset+securityLength) && end > uint64(securityOffset) {
			return fmt.Errorf("smb2: negotiate contexts overlap security buffer")
		}
		c := map[string]any{}
		smb2Literal(c, "Type", int(at), int(at+2), binary.LittleEndian.Uint16(pdu[at:at+2]))
		smb2Literal(c, "Data Length", int(at+2), int(at+4), uint16(length))
		smb2Literal(c, "Reserved", int(at+4), int(at+8), binary.LittleEndian.Uint32(pdu[at+4:at+8]))
		smb2Literal(c, "Data", int(at+8), int(end), bytes.Clone(pdu[at+8:end]))
		contexts = append(contexts, c)
		at = (end + 7) &^ 7
	}
	body["Negotiate Contexts"] = contexts
	return nil
}

func smb2CreateFields(info map[string]any, pdu []byte, response bool, max int) error {
	if response && info["Status"] != uint32(0) {
		return smb2ErrorFields(info, pdu)
	}
	minimum, structure := 56, uint16(57)
	if response {
		minimum, structure = 88, 89
	}
	body, err := smb2Body(pdu, minimum, structure)
	if err != nil {
		return err
	}
	info["Body"] = body
	if !response {
		offset, length := int(binary.LittleEndian.Uint16(pdu[108:110])), int(binary.LittleEndian.Uint16(pdu[110:112]))
		name, err := smb2Buffer(pdu, offset, length, 120)
		if err != nil || length%2 != 0 {
			return fmt.Errorf("smb2: invalid CREATE name buffer")
		}
		info["File Name"] = smb2UTF16LE(name)
		if length != 0 {
			smb2Literal(info, "File Name", offset, offset+length, info["File Name"])
			smb2Literal(body, "File Name", offset, offset+length, info["File Name"])
		}
		return smb2CreateContextFields(body, pdu, 112, 120, max)
	}
	fid := hex.EncodeToString(pdu[128:144])
	smb2Literal(info, "FileId", 128, 144, fid)
	smb2Literal(body, "FileId", 128, 144, fid)
	return smb2CreateContextFields(body, pdu, 144, 152, max)
}

// CREATE contexts form a forward-only, command-bounded chain. Validate all
// declared buffers before publishing a successful open; their type-specific
// data remains an explicitly opaque observation here.
func smb2CreateContextFields(body map[string]any, pdu []byte, pairOffset, minimum, max int) error {
	offset := uint64(binary.LittleEndian.Uint32(pdu[pairOffset : pairOffset+4]))
	length := uint64(binary.LittleEndian.Uint32(pdu[pairOffset+4 : pairOffset+8]))
	smb2Literal(body, "Create Contexts Offset", pairOffset, pairOffset+4, uint32(offset))
	smb2Literal(body, "Create Contexts Length", pairOffset+4, pairOffset+8, uint32(length))
	if length == 0 {
		return nil
	}
	if offset < uint64(minimum) || offset%8 != 0 || offset > uint64(len(pdu)) || length > uint64(len(pdu))-offset {
		return fmt.Errorf("smb2: CREATE contexts outside command")
	}
	end, at := offset+length, offset
	var contexts []map[string]any
	for {
		if len(contexts) >= max {
			return protocolError(ErrResourceExceeded, "SMB2 CREATE context count exceeds collection budget")
		}
		if end-at < 16 {
			return fmt.Errorf("smb2: truncated CREATE context header")
		}
		next := uint64(binary.LittleEndian.Uint32(pdu[at : at+4]))
		stop := end
		if next != 0 {
			if next < 16 || next%8 != 0 || next > end-at || end-at-next < 16 {
				return fmt.Errorf("smb2: invalid CREATE context Next")
			}
			stop = at + next
		}
		nameOffset := uint64(binary.LittleEndian.Uint16(pdu[at+4 : at+6]))
		nameLength := uint64(binary.LittleEndian.Uint16(pdu[at+6 : at+8]))
		dataOffset := uint64(binary.LittleEndian.Uint16(pdu[at+10 : at+12]))
		dataLength := uint64(binary.LittleEndian.Uint32(pdu[at+12 : at+16]))
		for _, span := range [][2]uint64{{nameOffset, nameLength}, {dataOffset, dataLength}} {
			if span[1] != 0 && (span[0] < 16 || span[0]%8 != 0 || span[0] > stop-at || span[1] > stop-at-span[0]) {
				return fmt.Errorf("smb2: CREATE context buffer outside entry")
			}
		}
		if nameLength != 0 && dataLength != 0 && nameOffset < dataOffset+dataLength && dataOffset < nameOffset+nameLength {
			return fmt.Errorf("smb2: CREATE context name and data overlap")
		}
		c := map[string]any{}
		smb2Literal(c, "Next", int(at), int(at+4), uint32(next))
		if nameLength != 0 {
			smb2Literal(c, "Name", int(at+nameOffset), int(at+nameOffset+nameLength), bytes.Clone(pdu[at+nameOffset:at+nameOffset+nameLength]))
		}
		if dataLength != 0 {
			smb2Literal(c, "Data", int(at+dataOffset), int(at+dataOffset+dataLength), bytes.Clone(pdu[at+dataOffset:at+dataOffset+dataLength]))
		}
		contexts = append(contexts, c)
		if next == 0 {
			body["Create Contexts"] = contexts
			return nil
		}
		at += next
	}
}

func smb2CloseFields(info map[string]any, pdu []byte, response bool) error {
	if response && info["Status"] != uint32(0) {
		return smb2ErrorFields(info, pdu)
	}
	minimum, structure := 24, uint16(24)
	if response {
		minimum, structure = 60, 60
	}
	body, err := smb2Body(pdu, minimum, structure)
	if err != nil {
		return err
	}
	info["Body"] = body
	smb2Literal(body, "Flags", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	if !response {
		fid := hex.EncodeToString(pdu[72:88])
		smb2Literal(info, "FileId", 72, 88, fid)
		smb2Literal(body, "FileId", 72, 88, fid)
		return nil
	}
	for i, key := range []string{"Creation Time", "Last Access Time", "Last Write Time", "Change Time", "Allocation Size", "End of File"} {
		at := 72 + 8*i
		smb2Literal(body, key, at, at+8, binary.LittleEndian.Uint64(pdu[at:at+8]))
	}
	smb2Literal(body, "File Attributes", 120, 124, binary.LittleEndian.Uint32(pdu[120:124]))
	return nil
}
