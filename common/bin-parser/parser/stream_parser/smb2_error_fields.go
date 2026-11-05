package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// DecodeSMB2ErrorBody is the common bounded ERROR layout. Callers select it
// from the command's status; these bytes are not a successful command body.
func DecodeSMB2ErrorBody(wire []byte) (map[string]any, error) {
	fields, info, err := decodeSMB2ErrorBody(wire)
	if err != nil {
		return nil, err
	}
	return projectSMB2BodyFields(wire, fields, info), nil
}

func decodeSMB2ErrorBody(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) > SMB2QueryInfoMaxBodyBytes || len(wire) < 8 || binary.LittleEndian.Uint16(wire) != 9 {
		return nil, nil, fmt.Errorf("smb2 error: complete bounded 8-byte fixed body and StructureSize 9 required")
	}
	n := uint64(binary.LittleEndian.Uint32(wire[4:8]))
	if n > uint64(len(wire)-8) {
		return nil, nil, fmt.Errorf("smb2 error: ByteCount exceeds command")
	}
	fields := []tlsCertificateField{
		tlsCertificateLeaf("Structure Size", "uint16", 0, 2),
		tlsCertificateLeaf("Error Context Count", "uint8", 2, 3),
		tlsCertificateLeaf("Reserved", "uint8", 3, 4),
		tlsCertificateLeaf("Byte Count", "uint32", 4, 8),
		tlsCertificateLeaf("Error Data", "raw", 8, 8+int(n)),
	}
	if end := 8 + int(n); end < len(wire) {
		fields = append(fields, tlsCertificateLeaf("Trailing Bytes", "raw", end, len(wire)))
	}
	return fields, map[string]any{"Error Response": true}, nil
}

func parseSMB2ErrorBody(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	return parseExactByteFieldTreeWithEndian(node, process, decodeSMB2ErrorBody, "smb2-error", "little")
}
