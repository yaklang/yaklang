package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
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
	// MS-FSCC 2.4.40. One entry whose NextEntryOffset is zero and whose name fills the buffer.
	if len(wire) < 24 {
		return nil, nil, fmt.Errorf("FileStreamInformation: truncated fixed prefix")
	}
	next := binary.LittleEndian.Uint32(wire[0:4])
	nameLen := int(binary.LittleEndian.Uint32(wire[4:8]))
	if next != 0 || nameLen != len(wire)-24 || nameLen%2 != 0 {
		return nil, nil, fmt.Errorf("FileStreamInformation: entry does not fill this output")
	}
	text, err := DecodeCIFSUTF16(wire[24:])
	if err != nil {
		return nil, nil, err
	}
	info["Stream Name"] = text
	return []tlsCertificateField{
		tlsCertificateLeaf("Next Entry Offset", "uint32", 0, 4), tlsCertificateLeaf("Stream Name Length", "uint32", 4, 8),
		tlsCertificateLeaf("Stream Size", "int64", 8, 16), tlsCertificateLeaf("Stream Allocation Size", "int64", 16, 24),
		tlsCertificateLeaf("Stream Name Raw Bytes", "raw", 24, len(wire)),
	}, info, nil
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
	// MS-DTYP 2.4.6 self-relative header. Nonzero offsets must land inside this buffer.
	// SID and ACL bytes stay one raw tail; this profile does not name their fields.
	if len(wire) < 20 {
		return nil, nil, fmt.Errorf("SecurityDescriptor: truncated 20-byte header")
	}
	for _, at := range []int{4, 8, 12, 16} {
		off := int(binary.LittleEndian.Uint32(wire[at : at+4]))
		if off != 0 && (off < 20 || off >= len(wire)) {
			return nil, nil, fmt.Errorf("SecurityDescriptor: offset leaves this buffer")
		}
	}
	info["Security Body Status"] = "opaque: SID and ACL bytes are not selected"
	fields := []tlsCertificateField{
		tlsCertificateLeaf("Revision", "uint8", 0, 1), tlsCertificateLeaf("Sbz1", "uint8", 1, 2),
		tlsCertificateLeaf("Control", "uint16", 2, 4), tlsCertificateLeaf("Offset Owner", "uint32", 4, 8),
		tlsCertificateLeaf("Offset Group", "uint32", 8, 12), tlsCertificateLeaf("Offset Sacl", "uint32", 12, 16),
		tlsCertificateLeaf("Offset Dacl", "uint32", 16, 20),
	}
	if len(wire) > 20 {
		fields = append(fields, tlsCertificateLeaf("Security Descriptor Body", "raw", 20, len(wire)))
	}
	return fields, info, nil
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
