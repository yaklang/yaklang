package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
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
		smb2Literal(body, "Security Mode", 68, 70, binary.LittleEndian.Uint16(b[4:6]))
		smb2Literal(body, "Reserved", 70, 72, binary.LittleEndian.Uint16(b[6:8]))
		smb2Literal(body, "Capabilities", 72, 76, binary.LittleEndian.Uint32(b[8:12]))
		smb2Literal(body, "Client Guid", 76, 92, bytes.Clone(b[12:28]))
		count := int(binary.LittleEndian.Uint16(b[2:4]))
		if count == 0 || count > (len(b)-36)/2 {
			return fmt.Errorf("smb2: incomplete or empty negotiate dialect list")
		}
		if count > max {
			return smb2CollectionLimit("SMB2 dialect count exceeds collection budget")
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
		modern := false
		for _, dialect := range dialects {
			if dialect == 0x0311 {
				modern = true
			}
		}
		// MS-SMB2 2.2.3: the 8 bytes at offset 28 are contexts only when 0x0311 is offered.
		if modern {
			smb2Literal(body, "Negotiate Context Offset", 92, 96, binary.LittleEndian.Uint32(b[28:32]))
			smb2Literal(body, "Negotiate Context Count", 96, 98, binary.LittleEndian.Uint16(b[32:34]))
			smb2Literal(body, "Negotiate Context Reserved", 98, 100, binary.LittleEndian.Uint16(b[34:36]))
			return smb2NegotiateContextFields(body, pdu, int(binary.LittleEndian.Uint16(b[32:34])), uint64(binary.LittleEndian.Uint32(b[28:32])), 100+2*count, max, 0, 0)
		}
		smb2Literal(body, "Client Start Time", 92, 100, binary.LittleEndian.Uint64(b[28:36]))
		return nil
	}
	dialect := binary.LittleEndian.Uint16(b[4:6])
	smb2Literal(body, "Security Mode", 66, 68, binary.LittleEndian.Uint16(b[2:4]))
	smb2Literal(body, "Dialect", 68, 70, dialect)
	if dialect == 0x0311 {
		smb2Literal(body, "Negotiate Context Count", 70, 72, binary.LittleEndian.Uint16(b[6:8]))
	} else {
		smb2Literal(body, "Reserved", 70, 72, binary.LittleEndian.Uint16(b[6:8]))
	}
	smb2Literal(body, "Server Guid", 72, 88, bytes.Clone(b[8:24]))
	smb2Literal(body, "Capabilities", 88, 92, binary.LittleEndian.Uint32(b[24:28]))
	smb2Literal(body, "Max Transact Size", 92, 96, binary.LittleEndian.Uint32(b[28:32]))
	smb2Literal(body, "Max Read Size", 96, 100, binary.LittleEndian.Uint32(b[32:36]))
	smb2Literal(body, "Max Write Size", 100, 104, binary.LittleEndian.Uint32(b[36:40]))
	smb2Literal(body, "System Time", 104, 112, binary.LittleEndian.Uint64(b[40:48]))
	smb2Literal(body, "Server Start Time", 112, 120, binary.LittleEndian.Uint64(b[48:56]))
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
		smb2Literal(body, "Negotiate Context Offset", 124, 128, binary.LittleEndian.Uint32(b[60:64]))
		count := int(binary.LittleEndian.Uint16(b[6:8]))
		at := uint64(binary.LittleEndian.Uint32(b[60:64]))
		return smb2NegotiateContextFields(body, pdu, count, at, 128, max, offset, length)
	}
	smb2Literal(body, "Reserved2", 124, 128, binary.LittleEndian.Uint32(b[60:64]))
	return nil
}

func smb2NegotiateContextFields(body map[string]any, pdu []byte, count int, at uint64, minimum, max, securityOffset, securityLength int) error {
	if count > max {
		return smb2CollectionLimit("SMB2 negotiate context count exceeds collection budget")
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
		smb2TypeNegotiateContext(c, int(at+8))
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
		smb2Literal(body, "Security Flags", 66, 67, pdu[66])
		smb2Literal(body, "Requested Oplock Level", 67, 68, pdu[67])
		smb2Literal(body, "Impersonation Level", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
		smb2Literal(body, "Smb Create Flags", 72, 80, binary.LittleEndian.Uint64(pdu[72:80]))
		smb2Literal(body, "Reserved", 80, 88, binary.LittleEndian.Uint64(pdu[80:88]))
		smb2Literal(body, "Desired Access", 88, 92, binary.LittleEndian.Uint32(pdu[88:92]))
		smb2Literal(body, "File Attributes", 92, 96, binary.LittleEndian.Uint32(pdu[92:96]))
		smb2Literal(body, "Share Access", 96, 100, binary.LittleEndian.Uint32(pdu[96:100]))
		smb2Literal(body, "Create Disposition", 100, 104, binary.LittleEndian.Uint32(pdu[100:104]))
		smb2Literal(body, "Create Options", 104, 108, binary.LittleEndian.Uint32(pdu[104:108]))
		smb2Literal(body, "Name Offset", 108, 110, binary.LittleEndian.Uint16(pdu[108:110]))
		smb2Literal(body, "Name Length", 110, 112, binary.LittleEndian.Uint16(pdu[110:112]))
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
		return smb2CreateContextFields(body, pdu, 112, 120, max, false)
	}
	smb2Literal(body, "Oplock Level", 66, 67, pdu[66])
	smb2Literal(body, "Flags", 67, 68, pdu[67])
	smb2Literal(body, "Create Action", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	for i, key := range []string{"Creation Time", "Last Access Time", "Last Write Time", "Change Time"} {
		at := 72 + 8*i
		smb2Literal(body, key, at, at+8, binary.LittleEndian.Uint64(pdu[at:at+8]))
	}
	for i, key := range []string{"Allocation Size", "End Of File"} {
		at := 104 + 8*i
		size := int64(binary.LittleEndian.Uint64(pdu[at : at+8]))
		if size < 0 {
			return fmt.Errorf("smb2: CREATE %s is a negative LARGE_INTEGER", key)
		}
		smb2Literal(body, key, at, at+8, size)
	}
	smb2Literal(body, "File Attributes", 120, 124, binary.LittleEndian.Uint32(pdu[120:124]))
	smb2Literal(body, "Reserved2", 124, 128, binary.LittleEndian.Uint32(pdu[124:128]))
	fid := hex.EncodeToString(pdu[128:144])
	smb2Literal(info, "FileId", 128, 144, fid)
	smb2Literal(body, "FileId", 128, 144, fid)
	return smb2CreateContextFields(body, pdu, 144, 152, max, true)
}

// CREATE contexts form a forward-only, command-bounded chain. Validate all
// declared buffers before publishing a successful open. Only the observed
// DH2Q, MxAc, QFid and RqLs layouts from MS-SMB2 2.2.13.2 are typed.
func smb2CreateContextFields(body map[string]any, pdu []byte, pairOffset, minimum, max int, response bool) error {
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
			return smb2CollectionLimit("SMB2 CREATE context count exceeds collection budget")
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
		smb2TypeCreateContext(c, int(at+dataOffset), response)
		contexts = append(contexts, c)
		if next == 0 {
			body["Create Contexts"] = contexts
			return nil
		}
		at += next
	}
}

func smb2ContextData(c map[string]any) ([]byte, int, bool) {
	data, _ := c["Data"].([]byte)
	if len(data) == 0 {
		return nil, 0, true
	}
	ranges, _ := c["Wire Field Ranges"].(map[string]any)
	span, _ := ranges["Data"].([]any)
	if len(span) != 2 {
		return data, 0, false
	}
	base, ok := displayUint(span[0])
	if !ok || uint64(int(base)) != base {
		return data, 0, false
	}
	return data, int(base), true
}

func smb2TypeCreateContext(c map[string]any, dataAt int, response bool) {
	name, _ := c["Name"].([]byte)
	data, base, ok := smb2ContextData(c)
	if !ok {
		return
	}
	if base == 0 && len(data) != 0 {
		base = dataAt
	}
	opaque := func(reason string) {
		c["Data Status"] = "opaque: " + reason
		log.Debugf("smb2: CREATE context %q left opaque: %s", name, reason)
	}
	switch string(name) {
	case "MxAc":
		// MS-SMB2 2.2.13.2.5 request is one FILETIME. 2.2.13.2.6 response is status plus mask.
		if response && len(data) == 8 {
			smb2Literal(c, "Query Status", base, base+4, binary.LittleEndian.Uint32(data[0:4]))
			smb2Literal(c, "Maximal Access", base+4, base+8, binary.LittleEndian.Uint32(data[4:8]))
			return
		}
		if !response && len(data) == 8 {
			smb2Literal(c, "Timestamp", base, base+8, binary.LittleEndian.Uint64(data[0:8]))
			return
		}
		opaque("MxAc length is not the 8-byte request or response layout")
	case "QFid":
		// MS-SMB2 2.2.13.2: the request passes no data. The response is 32 bytes.
		if !response && len(data) == 0 {
			c["Data Status"] = "request has no QFid buffer"
			return
		}
		if response && len(data) == 32 {
			smb2Literal(c, "Disk File Id", base, base+8, binary.LittleEndian.Uint64(data[0:8]))
			smb2Literal(c, "Volume Id", base+8, base+16, binary.LittleEndian.Uint64(data[8:16]))
			smb2Literal(c, "Reserved", base+16, base+32, bytes.Clone(data[16:32]))
			return
		}
		opaque("QFid length is not an empty request or a 32-byte response")
	case "RqLs":
		// MS-SMB2 2.2.13.2.8 is 32 bytes. 2.2.13.2.10 lease v2 is 52 bytes. Request and response share the layout.
		if len(data) != 32 && len(data) != 52 {
			opaque("RqLs length is not 32 or 52")
			return
		}
		smb2Literal(c, "Lease Key", base, base+16, bytes.Clone(data[0:16]))
		smb2Literal(c, "Lease State", base+16, base+20, binary.LittleEndian.Uint32(data[16:20]))
		smb2Literal(c, "Lease Flags", base+20, base+24, binary.LittleEndian.Uint32(data[20:24]))
		smb2Literal(c, "Lease Duration", base+24, base+32, binary.LittleEndian.Uint64(data[24:32]))
		if len(data) == 52 {
			smb2Literal(c, "Parent Lease Key", base+32, base+48, bytes.Clone(data[32:48]))
			smb2Literal(c, "Epoch", base+48, base+50, binary.LittleEndian.Uint16(data[48:50]))
			smb2Literal(c, "Reserved", base+50, base+52, binary.LittleEndian.Uint16(data[50:52]))
		}
	case "DH2Q":
		// MS-SMB2 2.2.13.2.11 request is 32 bytes. 2.2.13.2.12 response is 8 bytes.
		if !response && len(data) == 32 {
			smb2Literal(c, "Timeout", base, base+4, binary.LittleEndian.Uint32(data[0:4]))
			smb2Literal(c, "Flags", base+4, base+8, binary.LittleEndian.Uint32(data[4:8]))
			smb2Literal(c, "Reserved", base+8, base+16, bytes.Clone(data[8:16]))
			smb2Literal(c, "Create Guid", base+16, base+32, bytes.Clone(data[16:32]))
			return
		}
		if response && len(data) == 8 {
			smb2Literal(c, "Timeout", base, base+4, binary.LittleEndian.Uint32(data[0:4]))
			smb2Literal(c, "Flags", base+4, base+8, binary.LittleEndian.Uint32(data[4:8]))
			return
		}
		opaque("DH2Q length is not the 32-byte request or 8-byte response")
	default:
		if len(data) != 0 {
			opaque("CREATE context tag is not a selected layout")
		}
	}
}

func smb2TypeNegotiateContext(c map[string]any, dataAt int) {
	kind, _ := c["Type"].(uint16)
	data, base, ok := smb2ContextData(c)
	if !ok {
		return
	}
	if base == 0 && len(data) != 0 {
		base = dataAt
	}
	opaque := func(reason string) {
		c["Data Status"] = "opaque: " + reason
		log.Debugf("smb2: negotiate context type %d left opaque: %s", kind, reason)
	}
	switch kind {
	case 1: // MS-SMB2 2.2.3.1.1 PREAUTH
		if len(data) < 4 {
			opaque("PREAUTH header is truncated")
			return
		}
		count := int(binary.LittleEndian.Uint16(data[0:2]))
		salt := int(binary.LittleEndian.Uint16(data[2:4]))
		if count < 1 || salt < 0 || 4+count*2+salt != len(data) {
			opaque("PREAUTH hash count or salt length does not fill this context")
			return
		}
		smb2Literal(c, "Hash Algorithm Count", base, base+2, uint16(count))
		smb2Literal(c, "Salt Length", base+2, base+4, uint16(salt))
		smb2Literal(c, "Hash Algorithms", base+4, base+4+count*2, bytes.Clone(data[4:4+count*2]))
		smb2Literal(c, "Salt", base+4+count*2, base+len(data), bytes.Clone(data[4+count*2:]))
	case 2: // MS-SMB2 2.2.3.1.2 ENCRYPTION
		if len(data) < 2 {
			opaque("ENCRYPTION cipher count is truncated")
			return
		}
		count := int(binary.LittleEndian.Uint16(data[0:2]))
		if count < 1 || 2+count*2 != len(data) {
			opaque("ENCRYPTION cipher count does not fill this context")
			return
		}
		smb2Literal(c, "Cipher Count", base, base+2, uint16(count))
		smb2Literal(c, "Ciphers", base+2, base+len(data), bytes.Clone(data[2:]))
	case 3: // MS-SMB2 2.2.3.1.3 COMPRESSION
		if len(data) < 8 {
			opaque("COMPRESSION header is truncated")
			return
		}
		count := int(binary.LittleEndian.Uint16(data[0:2]))
		if count < 1 || 8+count*2 != len(data) {
			opaque("COMPRESSION algorithm count does not fill this context")
			return
		}
		smb2Literal(c, "Compression Algorithm Count", base, base+2, uint16(count))
		smb2Literal(c, "Padding", base+2, base+4, binary.LittleEndian.Uint16(data[2:4]))
		smb2Literal(c, "Flags", base+4, base+8, binary.LittleEndian.Uint32(data[4:8]))
		smb2Literal(c, "Compression Algorithms", base+8, base+len(data), bytes.Clone(data[8:]))
	case 5: // MS-SMB2 2.2.3.1.4 NETNAME. The buffer is the UTF-16 name.
		if len(data) == 0 || len(data)%2 != 0 {
			opaque("NETNAME is not an even UTF-16 buffer")
			return
		}
		smb2Literal(c, "Net Name", base, base+len(data), smb2UTF16LE(data))
	default:
		if len(data) != 0 {
			opaque("negotiate context type is not a selected layout")
		}
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
		smb2Literal(body, "Reserved", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
		fid := hex.EncodeToString(pdu[72:88])
		smb2Literal(info, "FileId", 72, 88, fid)
		smb2Literal(body, "FileId", 72, 88, fid)
		return nil
	}
	for i, key := range []string{"Creation Time", "Last Access Time", "Last Write Time", "Change Time", "Allocation Size", "End of File"} {
		at := 72 + 8*i
		smb2Literal(body, key, at, at+8, binary.LittleEndian.Uint64(pdu[at:at+8]))
	}
	smb2Literal(body, "Reserved", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	smb2Literal(body, "File Attributes", 120, 124, binary.LittleEndian.Uint32(pdu[120:124]))
	return nil
}

// MS-SMB2 2.2.2 is the error body. STATUS_MORE_PROCESSING_REQUIRED keeps the
// SESSION_SETUP response. STATUS_BUFFER_OVERFLOW keeps the success body for
// READ, IOCTL, QUERY_DIRECTORY, CHANGE_NOTIFY and QUERY_INFO. STATUS_PENDING
// keeps the command body so an interim async response can publish its AsyncId;
// the suite's READ interim is StructureSize 17, not the 9-byte error body.
func smb2StatusUsesErrorBody(command uint16, status uint32, response bool) bool {
	if !response || status == 0 || status == 0x00000103 {
		return false
	}
	if command == 1 && status == 0xc0000016 {
		return false
	}
	if status == 0x80000005 {
		switch command {
		case 8, 11, 14, 15, 16:
			return false
		}
	}
	return true
}

func smb2SessionSetupFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(1, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if len(pdu) > smb2HeaderSize+(1<<20) {
		return protocolError(ErrResourceExceeded, "SMB2 SESSION_SETUP body exceeds bounded profile")
	}
	minimum, structure := 24, uint16(25)
	bufferAt := 12
	bufferMinimum := 88
	if response {
		minimum, structure, bufferAt, bufferMinimum = 8, 9, 4, 72
	}
	if len(pdu) < smb2HeaderSize+minimum || binary.LittleEndian.Uint16(pdu[64:66]) != structure {
		if !response && len(pdu) >= 66 && binary.LittleEndian.Uint16(pdu[64:66]) == 17 {
			return protocolError(ErrUnsupportedVersion, "SESSION_SETUP observed StructureSize 17; supported modern request layout requires 25")
		}
		return fmt.Errorf("smb2: SESSION_SETUP complete %d-byte fixed body and StructureSize %d required", minimum, structure)
	}
	body, err := smb2Body(pdu, minimum, structure)
	if err != nil {
		return err
	}
	info["Body"] = body
	b := pdu[64:]
	if !response {
		smb2Literal(body, "Flags", 66, 67, b[2])
		smb2Literal(body, "Security Mode", 67, 68, b[3])
		smb2Literal(body, "Capabilities", 68, 72, binary.LittleEndian.Uint32(b[4:8]))
		smb2Literal(body, "Channel", 72, 76, binary.LittleEndian.Uint32(b[8:12]))
		smb2Literal(body, "Previous Session Id", 80, 88, binary.LittleEndian.Uint64(b[16:24]))
	} else {
		flags := binary.LittleEndian.Uint16(b[2:4])
		smb2Literal(body, "Session Flags", 66, 68, flags)
		info["Session Flags"] = flags
		info["Guest Session"] = flags&1 != 0
	}
	offset := int(binary.LittleEndian.Uint16(b[bufferAt : bufferAt+2]))
	n := int(binary.LittleEndian.Uint16(b[bufferAt+2 : bufferAt+4]))
	smb2Literal(body, "Security Buffer Offset", 64+bufferAt, 66+bufferAt, uint16(offset))
	smb2Literal(body, "Security Buffer Length", 66+bufferAt, 68+bufferAt, uint16(n))
	if n == 0 {
		return nil
	}
	if offset < bufferMinimum || offset > len(pdu) || n > len(pdu)-offset {
		return fmt.Errorf("smb2: security buffer outside command")
	}
	token := bytes.Clone(pdu[offset : offset+n])
	smb2Literal(body, "Security Buffer", offset, offset+n, token)
	info["Authentication"] = decodeAuthenticationEnvelope(token)
	return nil
}

// MS-SMB2 2.2.30. CANCEL is a 4-byte request and has no response. It names the
// MessageId to cancel and does not itself retire that outstanding request.
// A non-final compound command is padded to the 8-byte NextCommand boundary.
func smb2CancelFields(info map[string]any, pdu []byte, response, padded bool) error {
	if response {
		return fmt.Errorf("smb2: CANCEL has no response")
	}
	if len(pdu) < smb2HeaderSize+4 || !padded && len(pdu) != smb2HeaderSize+4 {
		return fmt.Errorf("smb2: CANCEL request requires a complete 4-byte body")
	}
	body, err := smb2Body(pdu, 4, 4)
	if err != nil {
		return err
	}
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	if len(pdu) > smb2HeaderSize+4 {
		smb2Literal(body, "Padding", 68, len(pdu), bytes.Clone(pdu[68:]))
	}
	info["Body"] = body
	return nil
}

func smb2SmallCommandFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	command, _ := info["Command"].(uint16)
	if smb2StatusUsesErrorBody(command, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	body, err := smb2Body(pdu, 4, 4)
	if err != nil {
		return err
	}
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.24/2.2.25 FLUSH: a fixed 4-byte body in both directions.
func smb2FlushFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(7, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if response {
		body, err := smb2Body(pdu, 4, 4)
		if err != nil {
			return err
		}
		smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
		info["Body"] = body
		return nil
	}
	body, err := smb2Body(pdu, 24, 24)
	if err != nil {
		return err
	}
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	smb2Literal(body, "Reserved2", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	fid := hex.EncodeToString(pdu[72:88])
	smb2Literal(body, "FileId", 72, 88, fid)
	smb2Literal(info, "FileId", 72, 88, fid)
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.28/2.2.29 ECHO: a fixed 4-byte body in both directions.
func smb2EchoFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(13, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	body, err := smb2Body(pdu, 4, 4)
	if err != nil {
		return err
	}
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	smb2Literal(body, "Reserved2", 68, 70, binary.LittleEndian.Uint16(pdu[68:70]))
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.26/2.2.27 LOCK: a request carries LockCount fixed-size lock
// elements; the response is the 4-byte small body.
func smb2LockFields(info map[string]any, pdu []byte, response bool, max int) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(10, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if response {
		return smb2SmallCommandFields(info, pdu, response)
	}
	body, err := smb2Body(pdu, 48, 48)
	if err != nil {
		return err
	}
	count := int(binary.LittleEndian.Uint16(pdu[66:68]))
	if count == 0 {
		return fmt.Errorf("smb2: LOCK requires at least one lock element")
	}
	if count > max {
		return smb2CollectionLimit("SMB2 LOCK count exceeds collection budget")
	}
	if len(pdu) < 64+48+24*(count-1) {
		return fmt.Errorf("smb2: LOCK body does not cover %d lock elements", count)
	}
	smb2Literal(body, "Lock Count", 66, 68, uint16(count))
	smb2Literal(body, "Lock Sequence", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	fid := hex.EncodeToString(pdu[72:88])
	smb2Literal(body, "FileId", 72, 88, fid)
	smb2Literal(info, "FileId", 72, 88, fid)
	locks := make([]map[string]any, count)
	for i := 0; i < count; i++ {
		at := 88 + 24*i
		entry := map[string]any{}
		smb2Literal(entry, "Offset", at, at+8, binary.LittleEndian.Uint64(pdu[at:at+8]))
		smb2Literal(entry, "Length", at+8, at+16, binary.LittleEndian.Uint64(pdu[at+8:at+16]))
		smb2Literal(entry, "Flags", at+16, at+20, binary.LittleEndian.Uint32(pdu[at+16:at+20]))
		smb2Literal(entry, "Reserved", at+20, at+24, binary.LittleEndian.Uint32(pdu[at+20:at+24]))
		locks[i] = entry
	}
	body["Locks"], info["Locks"] = locks, locks
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.35/2.2.36 CHANGE_NOTIFY: a 32-byte request and a 9-byte
// response whose output buffer starts at OutputBufferOffset.
func smb2ChangeNotifyFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(15, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if !response {
		body, err := smb2Body(pdu, 32, 32)
		if err != nil {
			return err
		}
		smb2Literal(body, "Flags", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
		smb2Literal(body, "Output Buffer Length", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
		fid := hex.EncodeToString(pdu[72:88])
		smb2Literal(body, "FileId", 72, 88, fid)
		smb2Literal(info, "FileId", 72, 88, fid)
		smb2Literal(body, "Completion Filter", 88, 92, binary.LittleEndian.Uint32(pdu[88:92]))
		smb2Literal(body, "Reserved", 92, 96, binary.LittleEndian.Uint32(pdu[92:96]))
		info["Body"] = body
		return nil
	}
	body, err := smb2Body(pdu, 8, 9)
	if err != nil {
		return err
	}
	offset := int(binary.LittleEndian.Uint16(pdu[66:68]))
	length := int(binary.LittleEndian.Uint32(pdu[68:72]))
	smb2Literal(body, "Output Buffer Offset", 66, 68, uint16(offset))
	smb2Literal(body, "Output Buffer Length", 68, 72, uint32(length))
	if length != 0 {
		buffer, err := smb2Buffer(pdu, offset, length, 72)
		if err != nil {
			return fmt.Errorf("smb2: CHANGE_NOTIFY output buffer outside command")
		}
		smb2Literal(body, "Output Buffer", offset, offset+length, bytes.Clone(buffer))
		info["Output Buffer"] = bytes.Clone(buffer)
	}
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.39/2.2.40 SET_INFO: a 32-byte request selector/buffer header and
// a 2-byte response.
func smb2SetInfoFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(17, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if response {
		body, err := smb2Body(pdu, 2, 2)
		if err != nil {
			return err
		}
		info["Body"] = body
		return nil
	}
	body, err := smb2Body(pdu, 32, 33)
	if err != nil {
		return err
	}
	smb2Literal(body, "Info Type", 66, 67, pdu[66])
	smb2Literal(body, "File Info Class", 67, 68, pdu[67])
	bufferLength := binary.LittleEndian.Uint32(pdu[68:72])
	bufferOffset := binary.LittleEndian.Uint16(pdu[72:74])
	smb2Literal(body, "Buffer Length", 68, 72, bufferLength)
	smb2Literal(body, "Buffer Offset", 72, 74, bufferOffset)
	smb2Literal(body, "Reserved", 74, 76, binary.LittleEndian.Uint16(pdu[74:76]))
	smb2Literal(body, "Additional Information", 76, 80, binary.LittleEndian.Uint32(pdu[76:80]))
	fid := hex.EncodeToString(pdu[80:96])
	smb2Literal(body, "FileId", 80, 96, fid)
	smb2Literal(info, "FileId", 80, 96, fid)
	if bufferLength != 0 {
		buffer, err := smb2Buffer(pdu, int(bufferOffset), int(bufferLength), 96)
		if err != nil {
			return fmt.Errorf("smb2: SET_INFO buffer outside command")
		}
		smb2Literal(body, "Buffer", int(bufferOffset), int(bufferOffset)+int(bufferLength), bytes.Clone(buffer))
		info["Buffer"] = bytes.Clone(buffer)
	}
	info["Body"] = body
	return nil
}

// MS-SMB2 2.2.31/2.2.32 OPLOCK_BREAK: a fixed 24-byte body in both directions.
func smb2OplockBreakFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(18, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	body, err := smb2Body(pdu, 24, 24)
	if err != nil {
		return err
	}
	smb2Literal(body, "Oplock Level", 66, 67, pdu[66])
	smb2Literal(body, "Reserved", 67, 68, pdu[67])
	smb2Literal(body, "Reserved2", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	fid := hex.EncodeToString(pdu[72:88])
	smb2Literal(body, "FileId", 72, 88, fid)
	smb2Literal(info, "FileId", 72, 88, fid)
	info["Body"] = body
	return nil
}

func smb2TreeConnectFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(3, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if !response {
		body, err := smb2Body(pdu, 8, 9)
		if err != nil {
			return err
		}
		info["Body"] = body
		smb2Literal(body, "Flags", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
		offset := int(binary.LittleEndian.Uint16(pdu[68:70]))
		n := int(binary.LittleEndian.Uint16(pdu[70:72]))
		smb2Literal(body, "Path Offset", 68, 70, uint16(offset))
		smb2Literal(body, "Path Length", 70, 72, uint16(n))
		if n == 0 {
			return fmt.Errorf("smb2: TREE_CONNECT path is empty")
		}
		if n%2 != 0 || n > 4096 {
			return fmt.Errorf("smb2: TREE_CONNECT path length is not a bounded UTF-16 span")
		}
		path, err := smb2Buffer(pdu, offset, n, 72)
		if err != nil {
			return fmt.Errorf("smb2: TREE_CONNECT path outside command")
		}
		text := smb2UTF16LE(path)
		smb2Literal(body, "Path", offset, offset+n, text)
		smb2Literal(info, "Path", offset, offset+n, text)
		return nil
	}
	body, err := smb2Body(pdu, 16, 16)
	if err != nil {
		return err
	}
	info["Body"] = body
	smb2Literal(body, "Share Type", 66, 67, pdu[66])
	smb2Literal(body, "Reserved", 67, 68, pdu[67])
	smb2Literal(body, "Share Flags", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
	smb2Literal(body, "Capabilities", 72, 76, binary.LittleEndian.Uint32(pdu[72:76]))
	smb2Literal(body, "Maximal Access", 76, 80, binary.LittleEndian.Uint32(pdu[76:80]))
	return nil
}

func smb2ReadFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(8, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if !response {
		body, err := smb2Body(pdu, 48, 49)
		if err != nil {
			return err
		}
		info["Body"] = body
		smb2Literal(body, "Padding", 66, 67, pdu[66])
		smb2Literal(body, "Flags", 67, 68, pdu[67])
		length := binary.LittleEndian.Uint32(pdu[68:72])
		smb2Literal(body, "Length", 68, 72, length)
		info["Length"] = length
		smb2Literal(body, "Offset", 72, 80, binary.LittleEndian.Uint64(pdu[72:80]))
		fid := hex.EncodeToString(pdu[80:96])
		smb2Literal(body, "FileId", 80, 96, fid)
		smb2Literal(info, "FileId", 80, 96, fid)
		smb2Literal(body, "Minimum Count", 96, 100, binary.LittleEndian.Uint32(pdu[96:100]))
		channel := binary.LittleEndian.Uint32(pdu[100:104])
		smb2Literal(body, "Channel", 100, 104, channel)
		smb2Literal(body, "Remaining Bytes", 104, 108, binary.LittleEndian.Uint32(pdu[104:108]))
		channelOffset := int(binary.LittleEndian.Uint16(pdu[108:110]))
		channelLength := int(binary.LittleEndian.Uint16(pdu[110:112]))
		smb2Literal(body, "Read Channel Info Offset", 108, 110, uint16(channelOffset))
		smb2Literal(body, "Read Channel Info Length", 110, 112, uint16(channelLength))
		if channel == 0 {
			if channelOffset != 0 || channelLength != 0 {
				return fmt.Errorf("smb2: READ channel NONE cannot carry channel info")
			}
			return nil
		}
		raw, err := smb2Buffer(pdu, channelOffset, channelLength, 112)
		if err != nil {
			return fmt.Errorf("smb2: READ channel info outside command")
		}
		smb2Literal(body, "Read Channel Info", channelOffset, channelOffset+channelLength, bytes.Clone(raw))
		body["Read Channel Info Status"] = "opaque: channel payload layout is not selected"
		log.Debugf("smb2: READ channel 0x%08x payload left opaque; layout not selected", channel)
		return nil
	}
	body, err := smb2Body(pdu, 16, 17)
	if err != nil {
		return err
	}
	info["Body"] = body
	dataOffset := int(pdu[66])
	smb2Literal(body, "Data Offset", 66, 67, pdu[66])
	smb2Literal(body, "Reserved", 67, 68, pdu[67])
	dataLength := int(binary.LittleEndian.Uint32(pdu[68:72]))
	smb2Literal(body, "Data Length", 68, 72, uint32(dataLength))
	smb2Literal(body, "Data Remaining", 72, 76, binary.LittleEndian.Uint32(pdu[72:76]))
	smb2Literal(body, "Reserved2", 76, 80, binary.LittleEndian.Uint32(pdu[76:80]))
	if dataLength == 0 {
		return nil
	}
	if dataOffset < 80 || dataOffset%8 != 0 || dataOffset > len(pdu) || dataLength > len(pdu)-dataOffset {
		return fmt.Errorf("smb2: READ data outside command")
	}
	smb2Literal(body, "Data", dataOffset, dataOffset+dataLength, bytes.Clone(pdu[dataOffset:dataOffset+dataLength]))
	return nil
}

func smb2WriteFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(9, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if !response {
		body, err := smb2Body(pdu, 48, 49)
		if err != nil {
			return err
		}
		info["Body"] = body
		dataOffset := int(binary.LittleEndian.Uint16(pdu[66:68]))
		length := int(binary.LittleEndian.Uint32(pdu[68:72]))
		smb2Literal(body, "Data Offset", 66, 68, uint16(dataOffset))
		smb2Literal(body, "Length", 68, 72, uint32(length))
		info["Length"] = uint32(length)
		smb2Literal(body, "Offset", 72, 80, binary.LittleEndian.Uint64(pdu[72:80]))
		fid := hex.EncodeToString(pdu[80:96])
		smb2Literal(body, "FileId", 80, 96, fid)
		smb2Literal(info, "FileId", 80, 96, fid)
		channel := binary.LittleEndian.Uint32(pdu[96:100])
		smb2Literal(body, "Channel", 96, 100, channel)
		smb2Literal(body, "Remaining Bytes", 100, 104, binary.LittleEndian.Uint32(pdu[100:104]))
		channelOffset := int(binary.LittleEndian.Uint16(pdu[104:106]))
		channelLength := int(binary.LittleEndian.Uint16(pdu[106:108]))
		smb2Literal(body, "Write Channel Info Offset", 104, 106, uint16(channelOffset))
		smb2Literal(body, "Write Channel Info Length", 106, 108, uint16(channelLength))
		smb2Literal(body, "Flags", 108, 112, binary.LittleEndian.Uint32(pdu[108:112]))
		if length > 0 {
			raw, err := smb2Buffer(pdu, dataOffset, length, 112)
			if err != nil {
				return fmt.Errorf("smb2: WRITE data outside command")
			}
			smb2Literal(body, "Data", dataOffset, dataOffset+length, bytes.Clone(raw))
		}
		if channel == 0 {
			if channelOffset != 0 || channelLength != 0 {
				return fmt.Errorf("smb2: WRITE channel NONE cannot carry channel info")
			}
			return nil
		}
		raw, err := smb2Buffer(pdu, channelOffset, channelLength, 112)
		if err != nil {
			return fmt.Errorf("smb2: WRITE channel info outside command")
		}
		smb2Literal(body, "Write Channel Info", channelOffset, channelOffset+channelLength, bytes.Clone(raw))
		body["Write Channel Info Status"] = "opaque: channel payload layout is not selected"
		log.Debugf("smb2: WRITE channel 0x%08x payload left opaque; layout not selected", channel)
		return nil
	}
	body, err := smb2Body(pdu, 16, 17)
	if err != nil {
		return err
	}
	info["Body"] = body
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	count := binary.LittleEndian.Uint32(pdu[68:72])
	smb2Literal(body, "Count", 68, 72, count)
	info["Count"] = count
	smb2Literal(body, "Remaining", 72, 76, binary.LittleEndian.Uint32(pdu[72:76]))
	smb2Literal(body, "Write Channel Info Offset", 76, 78, binary.LittleEndian.Uint16(pdu[76:78]))
	smb2Literal(body, "Write Channel Info Length", 78, 80, binary.LittleEndian.Uint16(pdu[78:80]))
	return nil
}

func smb2IOCTLFields(info map[string]any, pdu []byte, response bool, max int) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(11, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	minimum, structure := 56, uint16(57)
	if response {
		minimum, structure = 48, 49
	}
	body, err := smb2Body(pdu, minimum, structure)
	if err != nil {
		return err
	}
	info["Body"] = body
	smb2Literal(body, "Reserved", 66, 68, binary.LittleEndian.Uint16(pdu[66:68]))
	code := binary.LittleEndian.Uint32(pdu[68:72])
	smb2Literal(body, "Ctl Code", 68, 72, code)
	fid := hex.EncodeToString(pdu[72:88])
	smb2Literal(body, "FileId", 72, 88, fid)
	smb2Literal(info, "FileId", 72, 88, fid)
	if !response {
		smb2Literal(body, "Input Offset", 88, 92, binary.LittleEndian.Uint32(pdu[88:92]))
		smb2Literal(body, "Input Count", 92, 96, binary.LittleEndian.Uint32(pdu[92:96]))
		smb2Literal(body, "Max Input Response", 96, 100, binary.LittleEndian.Uint32(pdu[96:100]))
		smb2Literal(body, "Output Offset", 100, 104, binary.LittleEndian.Uint32(pdu[100:104]))
		smb2Literal(body, "Output Count", 104, 108, binary.LittleEndian.Uint32(pdu[104:108]))
		smb2Literal(body, "Max Output Response", 108, 112, binary.LittleEndian.Uint32(pdu[108:112]))
		smb2Literal(body, "Flags", 112, 116, binary.LittleEndian.Uint32(pdu[112:116]))
		smb2Literal(body, "Reserved2", 116, 120, binary.LittleEndian.Uint32(pdu[116:120]))
		if err := smb2OpaqueIOCTLBuffer(body, pdu, "Input", int(binary.LittleEndian.Uint32(pdu[88:92])), int(binary.LittleEndian.Uint32(pdu[92:96])), 120); err != nil {
			return err
		}
	} else {
		smb2Literal(body, "Input Offset", 88, 92, binary.LittleEndian.Uint32(pdu[88:92]))
		smb2Literal(body, "Input Count", 92, 96, binary.LittleEndian.Uint32(pdu[92:96]))
		smb2Literal(body, "Output Offset", 96, 100, binary.LittleEndian.Uint32(pdu[96:100]))
		smb2Literal(body, "Output Count", 100, 104, binary.LittleEndian.Uint32(pdu[100:104]))
		smb2Literal(body, "Flags", 104, 108, binary.LittleEndian.Uint32(pdu[104:108]))
		smb2Literal(body, "Reserved2", 108, 112, binary.LittleEndian.Uint32(pdu[108:112]))
		if err := smb2OpaqueIOCTLBuffer(body, pdu, "Input", int(binary.LittleEndian.Uint32(pdu[88:92])), int(binary.LittleEndian.Uint32(pdu[92:96])), 112); err != nil {
			return err
		}
		outOff := int(binary.LittleEndian.Uint32(pdu[96:100]))
		outN := int(binary.LittleEndian.Uint32(pdu[100:104]))
		if err := smb2OpaqueIOCTLBuffer(body, pdu, "Output", outOff, outN, 112); err != nil {
			return err
		}
		if outN > 0 {
			raw := body["Output Buffer"].([]byte)
			if err := smb2DecodeIOCTLOutput(body, code, raw, outOff, max); err != nil {
				return err
			}
		}
	}
	if _, ok := body["Control Payload Status"]; !ok {
		body["Control Payload Status"] = "opaque: IOCTL control payload layout is not selected"
		log.Debugf("smb2: IOCTL control 0x%08x payload left opaque; layout not selected", code)
	}
	return nil
}

func smb2OpaqueIOCTLBuffer(body map[string]any, pdu []byte, name string, offset, length, minimum int) error {
	if length == 0 {
		return nil
	}
	raw, err := smb2Buffer(pdu, offset, length, minimum)
	if err != nil {
		return fmt.Errorf("smb2: IOCTL %s buffer outside command", name)
	}
	smb2Literal(body, name+" Buffer", offset, offset+length, bytes.Clone(raw))
	return nil
}

func smb2QueryDirectoryFields(info map[string]any, pdu []byte, response bool) error {
	status, _ := info["Status"].(uint32)
	if smb2StatusUsesErrorBody(14, status, response) {
		return smb2ErrorFields(info, pdu)
	}
	if !response {
		body, err := smb2Body(pdu, 32, 33)
		if err != nil {
			return err
		}
		info["Body"] = body
		smb2Literal(body, "File Information Class", 66, 67, pdu[66])
		smb2Literal(body, "Flags", 67, 68, pdu[67])
		smb2Literal(body, "File Index", 68, 72, binary.LittleEndian.Uint32(pdu[68:72]))
		fid := hex.EncodeToString(pdu[72:88])
		smb2Literal(body, "FileId", 72, 88, fid)
		smb2Literal(info, "FileId", 72, 88, fid)
		nameOffset := int(binary.LittleEndian.Uint16(pdu[88:90]))
		nameLength := int(binary.LittleEndian.Uint16(pdu[90:92]))
		smb2Literal(body, "File Name Offset", 88, 90, uint16(nameOffset))
		smb2Literal(body, "File Name Length", 90, 92, uint16(nameLength))
		smb2Literal(body, "Output Buffer Length", 92, 96, binary.LittleEndian.Uint32(pdu[92:96]))
		if nameLength == 0 {
			return nil
		}
		if nameLength%2 != 0 {
			return fmt.Errorf("smb2: QUERY_DIRECTORY name length is not UTF-16")
		}
		raw, err := smb2Buffer(pdu, nameOffset, nameLength, 96)
		if err != nil {
			return fmt.Errorf("smb2: QUERY_DIRECTORY name outside command")
		}
		smb2Literal(body, "File Name", nameOffset, nameOffset+nameLength, smb2UTF16LE(raw))
		return nil
	}
	body, err := smb2Body(pdu, 8, 9)
	if err != nil {
		return err
	}
	info["Body"] = body
	outputOffset := int(binary.LittleEndian.Uint16(pdu[66:68]))
	outputLength := int(binary.LittleEndian.Uint32(pdu[68:72]))
	smb2Literal(body, "Output Buffer Offset", 66, 68, uint16(outputOffset))
	smb2Literal(body, "Output Buffer Length", 68, 72, uint32(outputLength))
	if outputLength == 0 {
		body["Directory Entry Status"] = "opaque: empty output; entry chain not present"
		return nil
	}
	raw, err := smb2Buffer(pdu, outputOffset, outputLength, 72)
	if err != nil {
		return fmt.Errorf("smb2: QUERY_DIRECTORY output outside command")
	}
	smb2Literal(body, "Output Buffer", outputOffset, outputOffset+outputLength, bytes.Clone(raw))
	// The information class lives on the matched request. This response does
	// not repeat it, so the entry chain stays opaque instead of guessing.
	body["Directory Entry Status"] = "opaque: directory entry chain requires the matched request class"
	log.Debugf("smb2: QUERY_DIRECTORY output left opaque; entry chain is not selected without a proven class")
	return nil
}

const (
	smb2FSCTLQueryNetworkInterfaceInfo = 0x001401fc
	smb2FSCTLDFSGetReferrals           = 0x00060194
	smb2FSCTLPipeTransceive            = 0x0011c017
	smb2FileIdBothDirectoryInformation = 37
)

// MS-SMB2 2.2.32.5. Each interface is 152 bytes. Next is the distance to the
// following entry and must be zero on the last one. The chain cannot walk
// outside this output buffer.
func smb2DecodeIOCTLOutput(body map[string]any, code uint32, raw []byte, base, max int) error {
	switch code {
	case smb2FSCTLQueryNetworkInterfaceInfo:
		entries, err := smb2DecodeNetworkInterfaces(raw, max)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			smb2OffsetRanges(entry, base)
		}
		body["Network Interfaces"] = entries
		body["Control Payload Status"] = "decoded: FSCTL_QUERY_NETWORK_INTERFACE_INFO"
	case smb2FSCTLDFSGetReferrals:
		referral, version, ok, err := smb2SelectDFSReferral(raw, max)
		if err != nil {
			return err
		}
		if !ok {
			body["Control Payload Status"] = "opaque: DFS referral version is not a selected v1, v2, v3, or v4 layout"
			log.Debugf("smb2: FSCTL_DFS_GET_REFERRALS payload left opaque; v1, v2, v3, and v4 layouts were not present")
			return nil
		}
		smb2OffsetRanges(referral, base)
		body["DFS Referral"] = referral
		body["Control Payload Status"] = "decoded: FSCTL_DFS_GET_REFERRALS version " + version
	case smb2FSCTLPipeTransceive:
		body["Control Payload Status"] = "opaque: FSCTL_PIPE_TRANSCEIVE payload is not an SMB2 layout"
		log.Debugf("smb2: FSCTL_PIPE_TRANSCEIVE payload left opaque; named-pipe bytes are not decoded")
	default:
		body["Control Payload Status"] = "opaque: IOCTL control payload layout is not selected"
		log.Debugf("smb2: IOCTL control 0x%08x payload left opaque; layout not selected", code)
	}
	return nil
}

func smb2DecodeNetworkInterfaces(raw []byte, max int) ([]map[string]any, error) {
	if max <= 0 {
		max = 1
	}
	var out []map[string]any
	for at := 0; ; {
		if len(out) >= max {
			return nil, smb2CollectionLimit("SMB2 network interface count exceeds collection budget")
		}
		if len(raw)-at < 152 {
			return nil, fmt.Errorf("smb2: truncated network interface")
		}
		next := int(binary.LittleEndian.Uint32(raw[at : at+4]))
		if next != 0 && (next < 152 || at+next > len(raw)) {
			return nil, fmt.Errorf("smb2: network interface Next leaves this output buffer")
		}
		entry := map[string]any{}
		smb2Literal(entry, "Next", at, at+4, uint32(next))
		smb2Literal(entry, "IfIndex", at+4, at+8, binary.LittleEndian.Uint32(raw[at+4:at+8]))
		smb2Literal(entry, "Capability", at+8, at+12, binary.LittleEndian.Uint32(raw[at+8:at+12]))
		smb2Literal(entry, "Reserved", at+12, at+16, binary.LittleEndian.Uint32(raw[at+12:at+16]))
		smb2Literal(entry, "Link Speed", at+16, at+24, binary.LittleEndian.Uint64(raw[at+16:at+24]))
		family := binary.LittleEndian.Uint16(raw[at+24 : at+26])
		smb2Literal(entry, "Address Family", at+24, at+26, family)
		smb2Literal(entry, "Socket Address", at+24, at+152, bytes.Clone(raw[at+24:at+152]))
		switch family {
		case 2:
			smb2Literal(entry, "IPv4 Address", at+28, at+32, bytes.Clone(raw[at+28:at+32]))
		case 23:
			smb2Literal(entry, "IPv6 Address", at+32, at+48, bytes.Clone(raw[at+32:at+48]))
		default:
			entry["Address Status"] = "opaque: socket family is not IPv4 or IPv6"
		}
		out = append(out, entry)
		if next == 0 {
			if at+152 != len(raw) {
				return nil, fmt.Errorf("smb2: bytes follow the last network interface")
			}
			return out, nil
		}
		at += next
	}
}

// MS-DFSC 2.2.4 and 2.2.5.3. Only the version 3 entry of exactly 18 bytes is
// selected. Any other version keeps the whole output opaque.
func smb2DecodeDFSReferralV3(raw []byte, max int) (map[string]any, bool, error) {
	if len(raw) < 8 {
		return nil, false, nil
	}
	count := int(binary.LittleEndian.Uint16(raw[2:4]))
	if count <= 0 || count > max {
		if count > max && max > 0 {
			return nil, false, smb2CollectionLimit("SMB2 DFS referral count exceeds collection budget")
		}
		return nil, false, nil
	}
	at := 8
	for i := 0; i < count; i++ {
		if len(raw)-at < 18 || binary.LittleEndian.Uint16(raw[at:at+2]) != 3 || binary.LittleEndian.Uint16(raw[at+2:at+4]) != 18 {
			return nil, false, nil
		}
		// MS-DFSC 2.2.5.3.2. NameListReferral padding is 0 or 16 bytes and has
		// no length, so this 18-byte target layout must not read it as a path.
		if binary.LittleEndian.Uint16(raw[at+6:at+8])&0x0002 != 0 {
			log.Debugf("smb2: DFS referral left opaque; NameListReferral padding width is not fixed")
			return nil, false, nil
		}
		at += 18
	}
	info := map[string]any{}
	smb2Literal(info, "Path Consumed", 0, 2, binary.LittleEndian.Uint16(raw[0:2]))
	smb2Literal(info, "Number Of Referrals", 2, 4, uint16(count))
	smb2Literal(info, "Referral Header Flags", 4, 8, binary.LittleEndian.Uint32(raw[4:8]))
	entries := make([]map[string]any, count)
	at = 8
	for i := 0; i < count; i++ {
		entry := map[string]any{}
		smb2Literal(entry, "Version", at, at+2, uint16(3))
		smb2Literal(entry, "Size", at+2, at+4, uint16(18))
		smb2Literal(entry, "Server Type", at+4, at+6, binary.LittleEndian.Uint16(raw[at+4:at+6]))
		smb2Literal(entry, "Referral Entry Flags", at+6, at+8, binary.LittleEndian.Uint16(raw[at+6:at+8]))
		smb2Literal(entry, "Time To Live", at+8, at+12, binary.LittleEndian.Uint32(raw[at+8:at+12]))
		pathOff := int(binary.LittleEndian.Uint16(raw[at+12 : at+14]))
		altOff := int(binary.LittleEndian.Uint16(raw[at+14 : at+16]))
		netOff := int(binary.LittleEndian.Uint16(raw[at+16 : at+18]))
		smb2Literal(entry, "DFS Path Offset", at+12, at+14, uint16(pathOff))
		smb2Literal(entry, "DFS Alternate Path Offset", at+14, at+16, uint16(altOff))
		smb2Literal(entry, "Network Address Offset", at+16, at+18, uint16(netOff))
		for _, item := range []struct {
			name string
			off  int
		}{{"DFS Path", pathOff}, {"DFS Alternate Path", altOff}, {"Network Address", netOff}} {
			if item.off == 0 {
				continue
			}
			// A path cannot point into the fixed 18-byte entry itself.
			if at+item.off < at+18 {
				log.Debugf("smb2: DFS referral left opaque; %s offset points into the entry header", item.name)
				return nil, false, nil
			}
			text, lo, hi, err := smb2UTF16Z(raw, at+item.off)
			if err != nil {
				log.Debugf("smb2: DFS referral left opaque; %s is not a bounded UTF-16 string: %v", item.name, err)
				return nil, false, nil
			}
			smb2Literal(entry, item.name, lo, hi, text)
		}
		entries[i] = entry
		at += 18
	}
	info["Referrals"] = entries
	return info, true, nil
}

// MS-DFSC 2.2.4 header and 2.2.5.1 DFS_REFERRAL_V1. Version is 1, Size covers
// the 6-byte prefix plus one NUL-terminated UTF-16 share name, and the entries
// consume the output. Version 2 and every other size stay opaque.
func smb2DecodeDFSReferralV1(raw []byte, max int) (map[string]any, bool, error) {
	if len(raw) < 8 {
		return nil, false, nil
	}
	count := int(binary.LittleEndian.Uint16(raw[2:4]))
	if count <= 0 || count > max {
		if count > max && max > 0 {
			return nil, false, smb2CollectionLimit("SMB2 DFS referral count exceeds collection budget")
		}
		return nil, false, nil
	}
	at := 8
	for i := 0; i < count; i++ {
		if len(raw)-at < 8 || binary.LittleEndian.Uint16(raw[at:at+2]) != 1 {
			return nil, false, nil
		}
		size := int(binary.LittleEndian.Uint16(raw[at+2 : at+4]))
		if size < 8 || at+size > len(raw) {
			return nil, false, nil
		}
		_, _, hi, err := smb2UTF16Z(raw, at+6)
		if err != nil || hi != at+size {
			return nil, false, nil
		}
		at += size
	}
	info := map[string]any{}
	smb2Literal(info, "Path Consumed", 0, 2, binary.LittleEndian.Uint16(raw[0:2]))
	smb2Literal(info, "Number Of Referrals", 2, 4, uint16(count))
	smb2Literal(info, "Referral Header Flags", 4, 8, binary.LittleEndian.Uint32(raw[4:8]))
	if at < len(raw) {
		// MS-DFSC 2.2.4 allows trailing padding after the referral entries.
		smb2Literal(info, "Referral Padding", at, len(raw), bytes.Clone(raw[at:]))
	}
	entries := make([]map[string]any, count)
	at = 8
	for i := 0; i < count; i++ {
		size := int(binary.LittleEndian.Uint16(raw[at+2 : at+4]))
		text, lo, hi, err := smb2UTF16Z(raw, at+6)
		if err != nil {
			return nil, false, nil
		}
		entry := map[string]any{}
		smb2Literal(entry, "Version", at, at+2, uint16(1))
		smb2Literal(entry, "Size", at+2, at+4, uint16(size))
		smb2Literal(entry, "Server Type", at+4, at+6, binary.LittleEndian.Uint16(raw[at+4:at+6]))
		smb2Literal(entry, "Share Name", lo, hi, text)
		entries[i] = entry
		at += size
	}
	info["Referrals"] = entries
	return info, true, nil
}

func smb2SelectDFSReferral(raw []byte, max int) (map[string]any, string, bool, error) {
	for _, version := range []string{"3", "1", "2", "4", "3guid"} {
		var referral map[string]any
		var ok bool
		var err error
		switch version {
		case "3":
			referral, ok, err = smb2DecodeDFSReferralV3(raw, max)
		case "1":
			referral, ok, err = smb2DecodeDFSReferralV1(raw, max)
		case "2":
			referral, ok, err = smb2DecodeDFSReferralV2(raw, max)
		case "4":
			referral, ok, err = smb2DecodeDFSReferralV4(raw, max)
		case "3guid":
			referral, ok, err = smb2DecodeDFSReferralV3Spec(raw, max)
		}
		if err != nil || ok {
			if version == "3guid" {
				version = "3"
			}
			return referral, version, ok, err
		}
	}
	return nil, "", false, nil
}

// MS-DFSC 2.2.4 and 2.2.5.2. The fixed prefix is 22 bytes. Size is the distance
// to the next entry. NUL-terminated strings follow the last entry.
func smb2DecodeDFSReferralV2(raw []byte, max int) (map[string]any, bool, error) {
	return smb2DecodeDFSReferralFixed(raw, max, 2, 22, false)
}

// MS-DFSC 2.2.5.4 uses the 2.2.5.3.1 layout: 34 bytes including ServiceSiteGuid.
// NameListReferral stays opaque because its padding width is not fixed. The
// first entry must set TargetSetBoundary.
func smb2DecodeDFSReferralV4(raw []byte, max int) (map[string]any, bool, error) {
	return smb2DecodeDFSReferralFixed(raw, max, 4, 34, true)
}

// MS-DFSC 2.2.5.3.1. NameListReferral clear: the entry is 34 bytes and includes
// the ignored ServiceSiteGuid. This is separate from the 18-byte v3 records
// already selected from capture.
func smb2DecodeDFSReferralV3Spec(raw []byte, max int) (map[string]any, bool, error) {
	return smb2DecodeDFSReferralFixed(raw, max, 3, 34, false)
}

func smb2DecodeDFSReferralFixed(raw []byte, max int, version uint16, fixed int, v4 bool) (map[string]any, bool, error) {
	if len(raw) < 8 {
		return nil, false, nil
	}
	count := int(binary.LittleEndian.Uint16(raw[2:4]))
	if count <= 0 || count > max {
		if count > max && max > 0 {
			return nil, false, smb2CollectionLimit("SMB2 DFS referral count exceeds collection budget")
		}
		return nil, false, nil
	}
	at := 8
	sizes := make([]int, count)
	for i := 0; i < count; i++ {
		if len(raw)-at < fixed || binary.LittleEndian.Uint16(raw[at:at+2]) != version {
			return nil, false, nil
		}
		size := int(binary.LittleEndian.Uint16(raw[at+2 : at+4]))
		if size < fixed || at+size > len(raw) {
			return nil, false, nil
		}
		flags := binary.LittleEndian.Uint16(raw[at+6 : at+8])
		if fixed == 34 && flags&0x0002 != 0 || v4 && i == 0 && flags&0x0004 == 0 {
			return nil, false, nil
		}
		sizes[i] = size
		at += size
	}
	last := at
	info := map[string]any{}
	smb2Literal(info, "Path Consumed", 0, 2, binary.LittleEndian.Uint16(raw[0:2]))
	smb2Literal(info, "Number Of Referrals", 2, 4, uint16(count))
	smb2Literal(info, "Referral Header Flags", 4, 8, binary.LittleEndian.Uint32(raw[4:8]))
	entries := make([]map[string]any, count)
	at = 8
	for i := 0; i < count; i++ {
		entry := map[string]any{}
		smb2Literal(entry, "Version", at, at+2, version)
		smb2Literal(entry, "Size", at+2, at+4, uint16(sizes[i]))
		smb2Literal(entry, "Server Type", at+4, at+6, binary.LittleEndian.Uint16(raw[at+4:at+6]))
		smb2Literal(entry, "Referral Entry Flags", at+6, at+8, binary.LittleEndian.Uint16(raw[at+6:at+8]))
		pathAt, altAt, netAt := 16, 18, 20
		if version == 2 {
			smb2Literal(entry, "Proximity", at+8, at+12, binary.LittleEndian.Uint32(raw[at+8:at+12]))
			smb2Literal(entry, "Time To Live", at+12, at+16, binary.LittleEndian.Uint32(raw[at+12:at+16]))
		} else {
			smb2Literal(entry, "Time To Live", at+8, at+12, binary.LittleEndian.Uint32(raw[at+8:at+12]))
			pathAt, altAt, netAt = 12, 14, 16
			smb2Literal(entry, "Service Site Guid", at+18, at+34, bytes.Clone(raw[at+18:at+34]))
		}
		names := []struct {
			key string
			at  int
		}{{"DFS Path", pathAt}, {"DFS Alternate Path", altAt}, {"Network Address", netAt}}
		for _, name := range names {
			off := int(binary.LittleEndian.Uint16(raw[at+name.at : at+name.at+2]))
			smb2Literal(entry, name.key+" Offset", at+name.at, at+name.at+2, uint16(off))
			if off == 0 {
				continue
			}
			if at+off < last || at+off > len(raw) {
				return nil, false, nil
			}
			text, lo, hi, err := smb2UTF16Z(raw, at+off)
			if err != nil {
				return nil, false, nil
			}
			smb2Literal(entry, name.key, lo, hi, text)
		}
		entries[i] = entry
		at += sizes[i]
	}
	info["Referrals"] = entries
	return info, true, nil
}

func smb2UTF16Z(raw []byte, at int) (string, int, int, error) {
	if at < 0 || at > len(raw) || at%2 != 0 {
		return "", 0, 0, fmt.Errorf("smb2: UTF-16 string offset is outside the buffer")
	}
	for i := at; i+1 < len(raw); i += 2 {
		if raw[i] == 0 && raw[i+1] == 0 {
			return smb2UTF16LE(raw[at:i]), at, i + 2, nil
		}
	}
	return "", 0, 0, fmt.Errorf("smb2: UTF-16 string is not terminated inside the buffer")
}

// MS-FSCC 2.4.17 FileIdBothDirectoryInformation. NextEntryOffset is from the
// start of this entry, must be a multiple of 8, and must stay inside the
// output buffer. Zero ends the chain.
func smb2DecodeMatchedDirectory(command map[string]any, class uint8, max int) {
	body, ok := command["Body"].(map[string]any)
	if !ok {
		return
	}
	if class != smb2FileIdBothDirectoryInformation {
		body["Directory Entry Status"] = "opaque: directory information class is not selected"
		log.Debugf("smb2: QUERY_DIRECTORY class %d left opaque; FileIdBothDirectoryInformation is the selected chain", class)
		return
	}
	output, _ := body["Output Buffer"].([]byte)
	ranges, _ := body["Wire Field Ranges"].(map[string]any)
	span, _ := ranges["Output Buffer"].([]any)
	if len(output) == 0 || len(span) != 2 {
		body["Directory Entry Status"] = "opaque: directory output source range unavailable"
		return
	}
	lo, loOK := displayUint(span[0])
	hi, hiOK := displayUint(span[1])
	if !loOK || !hiOK || hi-lo != uint64(len(output)) {
		body["Directory Entry Status"] = "opaque: directory output source range inconsistent"
		return
	}
	entries, err := smb2DecodeFileIdBothEntries(output, max)
	if err != nil {
		command["Directory Decode Error"] = err.Error()
		body["Directory Entry Status"] = "opaque: directory entry chain is incomplete or walks outside the output buffer"
		log.Debugf("smb2: QUERY_DIRECTORY FileIdBoth chain refused: %s", err.Error())
		return
	}
	for _, entry := range entries {
		smb2OffsetRanges(entry, int(lo))
	}
	body["Directory Entries"] = entries
	body["Directory Entry Status"] = "decoded: FileIdBothDirectoryInformation from matched request"
}

func smb2DecodeFileIdBothEntries(raw []byte, max int) ([]map[string]any, error) {
	if max <= 0 {
		max = 1
	}
	var out []map[string]any
	for at := 0; at < len(raw); {
		if len(out) >= max {
			return nil, smb2CollectionLimit("SMB2 directory entry count exceeds collection budget")
		}
		if len(raw)-at < 104 {
			return nil, fmt.Errorf("smb2: truncated directory entry")
		}
		next := int(binary.LittleEndian.Uint32(raw[at : at+4]))
		nameLen := int(binary.LittleEndian.Uint32(raw[at+60 : at+64]))
		shortLen := int(raw[at+68])
		if nameLen < 0 || nameLen%2 != 0 || at+104+nameLen > len(raw) || shortLen > 24 || shortLen%2 != 0 {
			return nil, fmt.Errorf("smb2: directory name leaves this entry")
		}
		if next != 0 && (next < 104+nameLen || next%8 != 0 || at+next > len(raw)) {
			return nil, fmt.Errorf("smb2: directory NextEntryOffset leaves this output buffer")
		}
		endOfFile := int64(binary.LittleEndian.Uint64(raw[at+40 : at+48]))
		allocation := int64(binary.LittleEndian.Uint64(raw[at+48 : at+56]))
		if endOfFile < 0 || allocation < 0 {
			return nil, fmt.Errorf("smb2: directory size is a negative LARGE_INTEGER")
		}
		entry := map[string]any{}
		smb2Literal(entry, "Next Entry Offset", at, at+4, uint32(next))
		smb2Literal(entry, "File Index", at+4, at+8, binary.LittleEndian.Uint32(raw[at+4:at+8]))
		for i, key := range []string{"Creation Time", "Last Access Time", "Last Write Time", "Change Time"} {
			from := at + 8 + 8*i
			smb2Literal(entry, key, from, from+8, binary.LittleEndian.Uint64(raw[from:from+8]))
		}
		smb2Literal(entry, "End Of File", at+40, at+48, endOfFile)
		smb2Literal(entry, "Allocation Size", at+48, at+56, allocation)
		smb2Literal(entry, "File Attributes", at+56, at+60, binary.LittleEndian.Uint32(raw[at+56:at+60]))
		smb2Literal(entry, "File Name Length", at+60, at+64, uint32(nameLen))
		smb2Literal(entry, "EA Size", at+64, at+68, binary.LittleEndian.Uint32(raw[at+64:at+68]))
		smb2Literal(entry, "Short Name Length", at+68, at+69, raw[at+68])
		smb2Literal(entry, "Reserved", at+69, at+70, raw[at+69])
		smb2Literal(entry, "Short Name Bytes", at+70, at+94, bytes.Clone(raw[at+70:at+94]))
		smb2Literal(entry, "Reserved2", at+94, at+96, binary.LittleEndian.Uint16(raw[at+94:at+96]))
		smb2Literal(entry, "FileId", at+96, at+104, binary.LittleEndian.Uint64(raw[at+96:at+104]))
		if nameLen > 0 {
			smb2Literal(entry, "File Name", at+104, at+104+nameLen, smb2UTF16LE(raw[at+104:at+104+nameLen]))
		}
		if shortLen > 0 {
			smb2Literal(entry, "Short Name", at+70, at+70+shortLen, smb2UTF16LE(raw[at+70:at+70+shortLen]))
		}
		out = append(out, entry)
		if next == 0 {
			return out, nil
		}
		at += next
	}
	return nil, fmt.Errorf("smb2: directory chain has no terminal entry")
}
