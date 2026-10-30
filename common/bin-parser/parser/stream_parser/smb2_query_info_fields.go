package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// MS-SMB2 2.2.37/38 and 3.3.5.20.1/2. The caller supplies exactly one
// command's body, excluding the 64-byte SMB2 header and any following command.
// Offsets on the wire include that header; returned spans are body-relative.
// Request selectors are not present in a response. Output remains opaque here.
const SMB2QueryInfoMaxBodyBytes = 1 << 20

func DecodeSMB2QueryInfoBody(wire []byte, response bool, status uint32) (map[string]any, error) {
	fields, info, err := decodeSMB2QueryInfoBody(wire, response, status)
	if err != nil {
		return nil, err
	}
	return projectSMB2BodyFields(wire, fields, info), nil
}

func projectSMB2BodyFields(wire []byte, fields []tlsCertificateField, info map[string]any) map[string]any {
	ranges := make(map[string]any, len(fields))
	info["Wire Field Ranges"] = ranges
	for _, field := range fields {
		b := wire[field.Start:field.End]
		var value any
		switch field.Type {
		case "uint8":
			value = b[0]
		case "uint16":
			value = binary.LittleEndian.Uint16(b)
		case "uint32":
			value = binary.LittleEndian.Uint32(b)
		default:
			value = bytes.Clone(b)
		}
		info[field.Name] = value
		ranges[field.Name] = []any{uint64(field.Start), uint64(field.End)}
	}
	return info
}

func decodeSMB2QueryInfoBody(wire []byte, response bool, status uint32) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) > SMB2QueryInfoMaxBodyBytes {
		return nil, nil, fmt.Errorf("smb2 query info: body exceeds bounded profile")
	}
	minimum, structure := 40, uint16(41)
	errorResponse := response && status != 0 && status != 0x80000005 // STATUS_BUFFER_OVERFLOW carries the response layout.
	if errorResponse {
		return decodeSMB2ErrorBody(wire)
	}
	if response {
		minimum, structure = 8, 9
	}
	if len(wire) < minimum || binary.LittleEndian.Uint16(wire) != structure {
		return nil, nil, fmt.Errorf("smb2 query info: complete %d-byte fixed body and StructureSize %d required", minimum, structure)
	}
	fields := []tlsCertificateField{tlsCertificateLeaf("Structure Size", "uint16", 0, 2)}
	info := map[string]any{}
	var offset, length uint64
	bufferName := "Input Buffer"
	if response {
		fields = append(fields,
			tlsCertificateLeaf("Output Buffer Offset", "uint16", 2, 4),
			tlsCertificateLeaf("Output Buffer Length", "uint32", 4, 8))
		offset = uint64(binary.LittleEndian.Uint16(wire[2:4]))
		length = uint64(binary.LittleEndian.Uint32(wire[4:8]))
		bufferName = "Output Buffer"
	} else {
		fields = append(fields,
			tlsCertificateLeaf("Info Type", "uint8", 2, 3),
			tlsCertificateLeaf("File Info Class", "uint8", 3, 4),
			tlsCertificateLeaf("Output Buffer Length", "uint32", 4, 8),
			tlsCertificateLeaf("Input Buffer Offset", "uint16", 8, 10),
			tlsCertificateLeaf("Reserved", "uint16", 10, 12),
			tlsCertificateLeaf("Input Buffer Length", "uint32", 12, 16),
			tlsCertificateLeaf("Additional Information", "uint32", 16, 20),
			tlsCertificateLeaf("Flags", "uint32", 20, 24),
			tlsCertificateLeaf("File ID", "raw", 24, 40))
		offset = uint64(binary.LittleEndian.Uint16(wire[8:10]))
		length = uint64(binary.LittleEndian.Uint32(wire[12:16]))
	}
	end := minimum
	// A zero-length buffer does not dereference its ignored offset. Widen before
	// subtraction/addition so a hostile uint32 length cannot wrap on any platform.
	if length != 0 {
		if offset < uint64(64+minimum) || offset-64 > uint64(len(wire)) || length > uint64(len(wire))-(offset-64) {
			return nil, nil, fmt.Errorf("smb2 query info: %s outside command body", bufferName)
		}
		start := int(offset - 64)
		if start > minimum {
			fields = append(fields, tlsCertificateLeaf("Buffer Padding", "raw", minimum, start))
		}
		end = start + int(length)
		fields = append(fields, tlsCertificateLeaf(bufferName, "raw", start, end))
	}
	if end < len(wire) {
		fields = append(fields, tlsCertificateLeaf("Trailing Bytes", "raw", end, len(wire)))
	}
	return fields, info, nil
}

func parseSMB2QueryInfoBody(node *base.Node, process func(*base.Node) (func(bool), error), response bool, status uint32) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		fields, info, err := decodeSMB2QueryInfoBody(wire, response, status)
		return fields, info, err
	}, "smb2-query-info", "little")
}
