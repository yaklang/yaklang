package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// MS-SMB2 2.2.41: full bounded encrypted carrier, without decrypting or
// validating the authentication tag, nonce uniqueness or negotiated cipher.
func decodeSMB3TransformFields(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 53 || len(wire) > 1<<20 {
		return nil, nil, fmt.Errorf("smb3-transform: explicit 53..1048576 byte boundary required")
	}
	if binary.LittleEndian.Uint32(wire) != 0x424d53fd {
		return nil, nil, fmt.Errorf("smb3-transform: expected FD SMB")
	}
	if binary.LittleEndian.Uint16(wire[42:]) != 1 {
		return nil, nil, fmt.Errorf("smb3-transform: unsupported flags/encryption algorithm")
	}
	if uint64(binary.LittleEndian.Uint32(wire[36:])) != uint64(len(wire)-52) {
		return nil, nil, fmt.Errorf("smb3-transform: message size does not match encrypted payload boundary")
	}
	fields := []tlsCertificateField{
		tlsCertificateLeaf("ProtocolId", "uint32", 0, 4),
		tlsCertificateLeaf("Signature", "raw", 4, 20),
		tlsCertificateLeaf("Nonce", "raw", 20, 36),
		tlsCertificateLeaf("OriginalMessageSize", "uint32", 36, 40),
		tlsCertificateLeaf("Reserved", "uint16", 40, 42),
		tlsCertificateLeaf("Flags", "uint16", 42, 44),
		tlsCertificateLeaf("SessionId", "uint64", 44, 52),
		tlsCertificateLeaf("Encrypted Payload", "raw", 52, len(wire)),
	}
	return fields, map[string]any{"Layout Context": "SMB3 Transform", "Decrypted": false, "Authentication Verified": false, "Session State Validated": false, "Cipher Selected": false}, nil
}

func parseSMB3TransformFields(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	return parseExactByteFieldTreeWithEndian(node, process, decodeSMB3TransformFields, "smb3-transform", "little")
}
