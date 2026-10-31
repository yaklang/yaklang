package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// A response does not carry these selectors. Callers must establish them from
// a matched request, or explicitly choose a standalone canonical entry.
func SMB2QueryInfoOutputProfile(infoType, infoClass uint8) (string, bool) {
	if infoType == 1 {
		switch infoClass {
		case 7:
			return "FileEaInformation", true
		case 5:
			return "FileStandardInformation", true
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
	if infoClass == 7 {
		// MS-FSCC 2.4.13: one uint32, not an EA name/value list.
		if len(wire) != 4 {
			return nil, nil, fmt.Errorf("FileEaInformation: exactly 4 output bytes required")
		}
		return []tlsCertificateField{tlsCertificateLeaf("EA Size", "uint32", 0, 4)}, info, nil
	}
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
