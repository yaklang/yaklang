package stream_parser

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
)

// A response does not carry these selectors. Callers must establish them from
// a matched request, or explicitly choose a standalone canonical entry.
func SMB2QueryInfoOutputProfile(infoType, infoClass uint8) (string, bool) {
	switch infoType {
	case 1:
		switch infoClass {
		case 5:
			return "FileStandardInformation", true
		case 7:
			return "FileEaInformation", true
		case 22:
			return "FileStreamInformation", true
		case 34:
			return "FileNetworkOpenInformation", true
		}
	case 2:
		switch infoClass {
		case 1:
			return "FileFsVolumeInformation", true
		case 5:
			return "FileFsAttributeInformation", true
		}
	case 3:
		if infoClass == 0 {
			return "SecurityDescriptor", true
		}
	}
	return "", false
}

func decodeSMB2QueryInfoOutput(wire []byte, infoType, infoClass uint8) ([]tlsCertificateField, map[string]any, error) {
	profile, supported := SMB2QueryInfoOutputProfile(infoType, infoClass)
	if !supported {
		return nil, nil, fmt.Errorf("smb2 query output: unsupported request selector %d/%d", infoType, infoClass)
	}
	info := map[string]any{"Output Information Type": profile}
	switch {
	case infoType == 1 && infoClass == 7:
		// MS-FSCC 2.4.13: one uint32, not an EA name/value list.
		if len(wire) != 4 {
			return nil, nil, fmt.Errorf("FileEaInformation: exactly 4 output bytes required")
		}
		return []tlsCertificateField{tlsCertificateLeaf("EA Size", "uint32", 0, 4)}, info, nil
	case infoType == 1 && infoClass == 5:
		// MS-FSCC 2.4.47. Reserved can contain any value. Without the volume's
		// cluster size, the allocation-size multiple cannot be validated here.
		if len(wire) != 24 {
			return nil, nil, fmt.Errorf("FileStandardInformation: exactly 24 output bytes required")
		}
		if int64(binary.LittleEndian.Uint64(wire[8:16])) < 0 {
			return nil, nil, fmt.Errorf("FileStandardInformation: negative EndOfFile")
		}
		// MS-FSCC 2.1.8: every nonzero Boolean byte means true. Keep the literal
		// byte alongside its interpretation, without giving that metadata a range.
		info["Delete Pending State"] = wire[20] != 0
		info["Directory State"] = wire[21] != 0
		return []tlsCertificateField{
			tlsCertificateLeaf("Allocation Size", "int64", 0, 8),
			tlsCertificateLeaf("End Of File", "int64", 8, 16),
			tlsCertificateLeaf("Number Of Links", "uint32", 16, 20),
			tlsCertificateLeaf("Delete Pending", "uint8", 20, 21),
			tlsCertificateLeaf("Directory", "uint8", 21, 22),
			tlsCertificateLeaf("Reserved", "uint16", 22, 24),
		}, info, nil
	case infoType == 1 && infoClass == 34:
		// MS-FSCC 2.4.34. Exactly seven fixed fields, 56 bytes.
		if len(wire) != 56 {
			return nil, nil, fmt.Errorf("FileNetworkOpenInformation: exactly 56 output bytes required")
		}
		return []tlsCertificateField{
			tlsCertificateLeaf("Creation Time", "uint64", 0, 8), tlsCertificateLeaf("Last Access Time", "uint64", 8, 16),
			tlsCertificateLeaf("Last Write Time", "uint64", 16, 24), tlsCertificateLeaf("Change Time", "uint64", 24, 32),
			tlsCertificateLeaf("Allocation Size", "int64", 32, 40), tlsCertificateLeaf("End Of File", "int64", 40, 48),
			tlsCertificateLeaf("File Attributes", "uint32", 48, 52), tlsCertificateLeaf("Reserved", "uint32", 52, 56),
		}, info, nil
	case infoType == 1 && infoClass == 22:
		return smb2FileStreamFields(wire, info)
	case infoType == 2 && infoClass == 1:
		return smb2FsVolumeFields(wire, info)
	case infoType == 2 && infoClass == 5:
		return smb2FsAttributeFields(wire, info)
	case infoType == 3 && infoClass == 0:
		return smb2SecurityDescriptorFields(wire, info)
	default:
		return nil, nil, fmt.Errorf("smb2 query output: unsupported request selector %d/%d", infoType, infoClass)
	}
}

func smb2FileStreamFields(wire []byte, info map[string]any) ([]tlsCertificateField, map[string]any, error) {
	// MS-FSCC 2.4.40. Entries are chained by NextEntryOffset; the last one is
	// zero. Each entry owns its UTF-16 stream name.
	if len(wire) < 24 {
		return nil, nil, fmt.Errorf("FileStreamInformation: truncated fixed prefix")
	}
	const maxEntries = 4096
	var fields []tlsCertificateField
	var streams []map[string]any
	at := 0
	for {
		if len(streams) >= maxEntries {
			return nil, nil, fmt.Errorf("FileStreamInformation: entry count exceeds bounded profile")
		}
		if len(wire)-at < 24 {
			return nil, nil, fmt.Errorf("FileStreamInformation: truncated entry prefix")
		}
		next := int(binary.LittleEndian.Uint32(wire[at : at+4]))
		nameLen := int(binary.LittleEndian.Uint32(wire[at+4 : at+8]))
		if nameLen%2 != 0 || nameLen > len(wire)-(at+24) {
			return nil, nil, fmt.Errorf("FileStreamInformation: stream name leaves this output")
		}
		text, err := DecodeCIFSUTF16(wire[at+24 : at+24+nameLen])
		if err != nil {
			return nil, nil, err
		}
		prefix := fmt.Sprintf("Stream %d ", len(streams)+1)
		entry := map[string]any{
			"Stream Name":     text,
			"Stream Size":     int64(binary.LittleEndian.Uint64(wire[at+8 : at+16])),
			"Allocation Size": int64(binary.LittleEndian.Uint64(wire[at+16 : at+24])),
			"Wire Field Ranges": map[string]any{
				"Stream Name":        []any{uint64(at + 24), uint64(at + 24 + nameLen)},
				"Stream Size":        []any{uint64(at + 8), uint64(at + 16)},
				"Allocation Size":    []any{uint64(at + 16), uint64(at + 24)},
				"Next Entry Offset":  []any{uint64(at), uint64(at + 4)},
				"Stream Name Length": []any{uint64(at + 4), uint64(at + 8)},
			},
		}
		streams = append(streams, entry)
		fields = append(fields,
			tlsCertificateLeaf(prefix+"Next Entry Offset", "uint32", at, at+4),
			tlsCertificateLeaf(prefix+"Stream Name Length", "uint32", at+4, at+8),
			tlsCertificateLeaf(prefix+"Stream Size", "int64", at+8, at+16),
			tlsCertificateLeaf(prefix+"Stream Allocation Size", "int64", at+16, at+24),
			tlsCertificateLeaf(prefix+"Stream Name Raw Bytes", "raw", at+24, at+24+nameLen),
		)
		if next == 0 {
			break
		}
		if next < 24 || next > len(wire)-at || next%8 != 0 {
			return nil, nil, fmt.Errorf("FileStreamInformation: NextEntryOffset leaves this output")
		}
		at += next
	}
	info["Stream Count"] = len(streams)
	info["Streams"] = streams
	if len(streams) == 1 {
		info["Stream Name"] = streams[0]["Stream Name"]
	}
	return fields, info, nil
}

func smb2FsVolumeFields(wire []byte, info map[string]any) ([]tlsCertificateField, map[string]any, error) {
	// MS-FSCC 2.5.9. VolumeLabelLength counts the UTF-16 bytes after the 18-byte prefix.
	if len(wire) < 18 {
		return nil, nil, fmt.Errorf("FileFsVolumeInformation: truncated fixed prefix")
	}
	labelLen := int(binary.LittleEndian.Uint32(wire[12:16]))
	if labelLen != len(wire)-18 || labelLen%2 != 0 {
		return nil, nil, fmt.Errorf("FileFsVolumeInformation: label does not fill this output")
	}
	text, err := DecodeCIFSUTF16(wire[18:])
	if err != nil {
		return nil, nil, err
	}
	info["Volume Label"] = text
	return []tlsCertificateField{
		tlsCertificateLeaf("Volume Creation Time", "uint64", 0, 8), tlsCertificateLeaf("Volume Serial Number", "uint32", 8, 12),
		tlsCertificateLeaf("Volume Label Length", "uint32", 12, 16), tlsCertificateLeaf("Supports Objects", "uint8", 16, 17),
		tlsCertificateLeaf("Reserved", "uint8", 17, 18), tlsCertificateLeaf("Volume Label Raw Bytes", "raw", 18, len(wire)),
	}, info, nil
}

func smb2FsAttributeFields(wire []byte, info map[string]any) ([]tlsCertificateField, map[string]any, error) {
	// MS-FSCC 2.5.1. FileSystemNameLength counts the UTF-16 bytes after the 12-byte prefix.
	if len(wire) < 12 {
		return nil, nil, fmt.Errorf("FileFsAttributeInformation: truncated fixed prefix")
	}
	nameLen := int(binary.LittleEndian.Uint32(wire[8:12]))
	if nameLen != len(wire)-12 || nameLen%2 != 0 {
		return nil, nil, fmt.Errorf("FileFsAttributeInformation: name does not fill this output")
	}
	text, err := DecodeCIFSUTF16(wire[12:])
	if err != nil {
		return nil, nil, err
	}
	info["File System Name"] = text
	return []tlsCertificateField{
		tlsCertificateLeaf("File System Attributes", "uint32", 0, 4), tlsCertificateLeaf("Maximum Component Name Length", "uint32", 4, 8),
		tlsCertificateLeaf("File System Name Length", "uint32", 8, 12), tlsCertificateLeaf("File System Name Raw Bytes", "raw", 12, len(wire)),
	}, info, nil
}

func smb2SecurityDescriptorFields(wire []byte, info map[string]any) ([]tlsCertificateField, map[string]any, error) {
	// MS-DTYP 2.4.6. Nonzero offsets are buffer offsets only when SE_SELF_RELATIVE
	// is set. A SID or ACL that does not fit is not an identity.
	if len(wire) < 20 {
		return nil, nil, fmt.Errorf("SecurityDescriptor: truncated 20-byte header")
	}
	control := binary.LittleEndian.Uint16(wire[2:4])
	offsets := []int{
		int(binary.LittleEndian.Uint32(wire[4:8])),
		int(binary.LittleEndian.Uint32(wire[8:12])),
		int(binary.LittleEndian.Uint32(wire[12:16])),
		int(binary.LittleEndian.Uint32(wire[16:20])),
	}
	for _, off := range offsets {
		if off != 0 && (off < 20 || off >= len(wire)) {
			return nil, nil, fmt.Errorf("SecurityDescriptor: offset leaves this buffer")
		}
	}
	info["Identity Verified"] = false
	fields := []tlsCertificateField{
		tlsCertificateLeaf("Revision", "uint8", 0, 1), tlsCertificateLeaf("Sbz1", "uint8", 1, 2),
		tlsCertificateLeaf("Control", "uint16", 2, 4), tlsCertificateLeaf("Offset Owner", "uint32", 4, 8),
		tlsCertificateLeaf("Offset Group", "uint32", 8, 12), tlsCertificateLeaf("Offset Sacl", "uint32", 12, 16),
		tlsCertificateLeaf("Offset Dacl", "uint32", 16, 20),
	}
	if control&0x8000 == 0 {
		info["Security Body Status"] = "opaque: SID and ACL bytes are not selected"
		if offsets[0]|offsets[1]|offsets[2]|offsets[3] != 0 {
			info["Security Body Status"] = "opaque: security descriptor offsets are not self-relative"
			log.Debugf("smb2: security descriptor offsets left opaque; SE_SELF_RELATIVE is clear")
		}
		return fields, info, nil
	}
	type piece struct{ off, end int }
	var used []piece
	overlaps := func(off, end int) bool {
		for _, prev := range used {
			if off < prev.end && prev.off < end {
				return true
			}
		}
		return false
	}
	placeSID := func(name string, off int) {
		if off == 0 {
			return
		}
		text, n, reason := smb2SIDLiteral(wire[off:])
		if reason != "" {
			info[name+" SID Status"] = reason
			log.Debugf("smb2: %s SID left opaque: %s", name, reason)
			return
		}
		if overlaps(off, off+n) {
			info[name+" SID Status"] = "opaque: security descriptor ranges overlap"
			log.Debugf("smb2: %s SID left opaque; its bytes overlap another security descriptor field", name)
			return
		}
		info[name+" SID"] = text
		fields = append(fields,
			tlsCertificateLeaf(name+" SID Revision", "uint8", off, off+1),
			tlsCertificateLeaf(name+" SID Sub Authority Count", "uint8", off+1, off+2),
			tlsCertificateLeaf(name+" SID Identifier Authority", "raw", off+2, off+8),
		)
		if n > 8 {
			fields = append(fields, tlsCertificateLeaf(name+" SID Sub Authorities", "raw", off+8, off+n))
		}
		used = append(used, piece{off, off + n})
	}
	placeACL := func(name string, off int) {
		if off == 0 {
			return
		}
		aclFields, n, reason := smb2ACLFields(wire[off:], name, info)
		if reason != "" {
			info[name+" Status"] = reason
			log.Debugf("smb2: %s left opaque: %s", name, reason)
			return
		}
		if overlaps(off, off+n) {
			for key := range info {
				if strings.HasPrefix(key, name+" ACE ") {
					delete(info, key)
				}
			}
			info[name+" Status"] = "opaque: security descriptor ranges overlap"
			log.Debugf("smb2: %s left opaque; its bytes overlap another security descriptor field", name)
			return
		}
		for i := range aclFields {
			aclFields[i].Start += off
			aclFields[i].End += off
		}
		fields = append(fields, aclFields...)
		if binary.LittleEndian.Uint16(wire[off+4:off+6]) == 0 {
			info[name+" Status"] = "empty"
		} else {
			info[name+" Status"] = "decoded literal ACE bytes; identity not verified"
		}
		used = append(used, piece{off, off + n})
	}
	placeSID("Owner", offsets[0])
	placeSID("Group", offsets[1])
	placeACL("Sacl", offsets[2])
	placeACL("Dacl", offsets[3])
	failed := smb2OpaqueStatus(info["Owner SID Status"]) || smb2OpaqueStatus(info["Group SID Status"]) || smb2OpaqueStatus(info["Sacl Status"]) || smb2OpaqueStatus(info["Dacl Status"])
	decoded := info["Owner SID"] != nil || info["Group SID"] != nil || info["Sacl Status"] != nil || info["Dacl Status"] != nil
	switch {
	case failed:
		info["Security Body Status"] = "opaque: a SID or ACL does not fit in this buffer"
	case decoded:
		info["Security Body Status"] = "decoded literal SID and ACL; identity not verified"
	default:
		info["Security Body Status"] = "opaque: SID and ACL bytes are not selected"
	}
	return fields, info, nil
}

func smb2OpaqueStatus(value any) bool {
	text, ok := value.(string)
	return ok && len(text) >= 7 && text[:7] == "opaque:"
}

// MS-DTYP 2.4.2.2. Revision 1 and a sub-authority count that fits this buffer.
// The returned text is the literal S-1 form, not a resolved account.
func smb2SIDLiteral(wire []byte) (string, int, string) {
	if len(wire) < 8 {
		return "", 0, "opaque: SID sub-authority count leaves this buffer"
	}
	if wire[0] != 1 {
		return "", 0, "opaque: SID revision is not 1"
	}
	n := int(wire[1])
	if n > 15 {
		return "", 0, "opaque: SID sub-authority count leaves this buffer"
	}
	need := 8 + 4*n
	if len(wire) < need {
		return "", 0, "opaque: SID sub-authority count leaves this buffer"
	}
	var authority uint64
	for _, b := range wire[2:8] {
		authority = authority<<8 | uint64(b)
	}
	text := fmt.Sprintf("S-1-%d", authority)
	for i := 0; i < n; i++ {
		text += fmt.Sprintf("-%d", binary.LittleEndian.Uint32(wire[8+4*i:12+4*i]))
	}
	return text, need, ""
}

// MS-DTYP 2.4.5. AclSize covers the 8-byte header and any ACE bytes that follow.
// An ACE that does not fit is not published.
func smb2ACLFields(wire []byte, name string, info map[string]any) ([]tlsCertificateField, int, string) {
	if len(wire) < 8 {
		return nil, 0, "opaque: ACL header leaves this buffer"
	}
	size := int(binary.LittleEndian.Uint16(wire[2:4]))
	aces := int(binary.LittleEndian.Uint16(wire[4:6]))
	declaredACEs := aces
	if aces > 4096 {
		// A bounded DACL can declare up to 65535 four-byte ACEs. Walk only the
		// first 4096 and publish the truncation instead of allocating ~65k
		// projected fields for one record.
		aces = 4096
	}
	if size < 8 || size > len(wire) {
		return nil, 0, "opaque: ACL size leaves this buffer"
	}
	fields := []tlsCertificateField{
		tlsCertificateLeaf(name+" Revision", "uint8", 0, 1),
		tlsCertificateLeaf(name+" Sbz1", "uint8", 1, 2),
		tlsCertificateLeaf(name+" Size", "uint16", 2, 4),
		tlsCertificateLeaf(name+" Ace Count", "uint16", 4, 6),
		tlsCertificateLeaf(name+" Sbz2", "uint16", 6, 8),
	}
	at := 8
	for i := 0; i < aces; i++ {
		if at+4 > size {
			return nil, 0, "opaque: ACL bytes do not cover the declared ACE count"
		}
		aceSize := int(binary.LittleEndian.Uint16(wire[at+2 : at+4]))
		if aceSize < 4 || at+aceSize > size {
			return nil, 0, "opaque: ACL bytes do not cover the declared ACE count"
		}
		prefix := fmt.Sprintf("%s ACE %d", name, i)
		fields = append(fields,
			tlsCertificateLeaf(prefix+" Type", "uint8", at, at+1),
			tlsCertificateLeaf(prefix+" Flags", "uint8", at+1, at+2),
			tlsCertificateLeaf(prefix+" Size", "uint16", at+2, at+4),
			tlsCertificateLeaf(prefix+" Body", "raw", at+4, at+aceSize),
		)
		// MS-DTYP 2.4.4.2 ACCESS_ALLOWED_ACE and the denied/audit/alarm forms:
		// mask, then one SID. Other ACE types do not share that offset.
		if aceType := wire[at]; (aceType <= 3) && aceSize >= 8 {
			text, n, reason := smb2SIDLiteral(wire[at+8 : at+aceSize])
			if reason == "" && n == aceSize-8 {
				info[prefix+" SID"] = text
			} else if reason != "" {
				log.Debugf("smb2: %s SID left raw: %s", prefix, reason)
			}
		}
		at += aceSize
	}
	if aces == 0 && size != 8 {
		return nil, 0, "opaque: empty ACL size is not 8"
	}
	if declaredACEs > aces {
		info[name+" Ace Count Declared"] = declaredACEs
		info[name+" Ace Count Parsed"] = aces
		info[name+" Truncated"] = "declared ACE count exceeds 4096-entry bounded profile"
	}
	return fields, size, ""
}

// DecodeSMB2QueryInfoOutput decodes only a complete selected output buffer.
// Returned literal ranges are output-relative; values and range maps are owned.
// Unsupported or malformed content never authorizes guessing another class.
func DecodeSMB2QueryInfoOutput(wire []byte, infoType, infoClass uint8) (map[string]any, error) {
	fields, info, err := decodeSMB2QueryInfoOutput(wire, infoType, infoClass)
	if err != nil {
		return nil, err
	}
	projected, err := projectStructuredFields(wire, fields, false, "little")
	if err != nil {
		return nil, err
	}
	out := projected.(map[string]any)
	ranges := make(map[string]any, len(fields))
	for _, field := range fields {
		ranges[field.Name] = []any{uint64(field.Start), uint64(field.End)}
	}
	for key, value := range info {
		out[key] = value
	}
	out["Wire Field Ranges"] = ranges
	return out, nil
}

func parseSMB2QueryInfoOutput(node *base.Node, process func(*base.Node) (func(bool), error), infoType, infoClass uint8) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeSMB2QueryInfoOutput(wire, infoType, infoClass)
	}, "smb2-query-output", "little")
}
