package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// MS-CIFS 2.2.4.64 and 2.2.4.43, plus the MS-SMB 2.2.4.3 high-count slots.
// These four profiles are the ordinary WC24/WC34 and WC14/WC6 success layouts.
// Active AndX chains and nonzero response statuses stay unsupported.
func DecodeCIFSNTCreateAndX(raw []byte, response bool) (map[string]any, error) {
	return decodeCIFSOpenWrite(raw, true, response)
}

func DecodeCIFSWriteAndX(raw []byte, response bool) (map[string]any, error) {
	return decodeCIFSOpenWrite(raw, false, response)
}

func parseCIFSNTCreateAndX(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsOpenWriteTree(w, true, response)
	}, "cifs-nt-create-andx", "little")
}

func parseCIFSWriteAndX(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsOpenWriteTree(w, false, response)
	}, "cifs-write-andx", "little")
}

func decodeCIFSOpenWrite(raw []byte, create, response bool) (map[string]any, error) {
	fields, info, err := cifsOpenWriteTree(raw, create, response)
	if err != nil {
		return nil, err
	}
	name := "SMB_COM_WRITE_ANDX"
	if create {
		name = "SMB_COM_NT_CREATE_ANDX"
	}
	out := map[string]any{"Wire Field Ranges": map[string]any{}, "Packet Name": name, "Response": response, "Authentication Verified": false, "Active AndX Chain Parsed": false}
	for key, value := range info {
		out[key] = value
	}
	for _, field := range fields {
		child := map[string]any{"Wire Field Ranges": map[string]any{}}
		for _, leaf := range field.Children {
			value, err := cifsOpenWriteValue(raw, leaf)
			if err != nil {
				return nil, err
			}
			child[leaf.Name] = value
			child["Wire Field Ranges"].(map[string]any)[leaf.Name] = []any{uint64(leaf.Start), uint64(leaf.End)}
		}
		out[field.Name] = child
	}
	if body, ok := out["Body"].(map[string]any); ok {
		if text, ok := info["File Name"].(string); ok {
			body["File Name"] = text
		}
	}
	if !create && !response {
		body := out["Body"].(map[string]any)
		// The 64-bit file offset is split. Neither half is that combined value.
		body["Offset Status"] = "low and high halves are separate literal fields"
	}
	return out, nil
}

func cifsOpenWriteTree(w []byte, create, response bool) ([]tlsCertificateField, map[string]any, error) {
	command := byte(0x2f)
	wc, fixed := byte(14), 67
	if response {
		wc, fixed = 6, 51
	}
	if create {
		command = 0xa2
		wc, fixed = 24, 87
		if response {
			wc, fixed = 34, 107
		}
	}
	fields, err := cifsFileCommandEnvelope(w, command, wc, fixed, fixed+65535, response, true)
	if err != nil {
		return nil, nil, err
	}
	if w[37] != 0xff {
		return nil, nil, fmt.Errorf("%w: active AndX chaining", ErrCIFSFileCommandUnsupported)
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w)}
	words := []struct {
		name, typ  string
		start, end int
	}{
		{"Word Count", "uint8", 36, 37},
		{"AndX Command", "uint8", 37, 38},
		{"AndX Reserved", "uint8", 38, 39},
		{"AndX Offset", "uint16", 39, 41},
	}
	if create && !response {
		words = append(words,
			cifsWord("Reserved", "uint8", 41, 42),
			cifsWord("Name Length", "uint16", 42, 44),
			cifsWord("Create Flags", "uint32", 44, 48),
			cifsWord("Root Directory FID", "uint32", 48, 52),
			cifsWord("Desired Access", "uint32", 52, 56),
			cifsWord("Allocation Size", "int64", 56, 64),
			cifsWord("Extended File Attributes", "uint32", 64, 68),
			cifsWord("Share Access", "uint32", 68, 72),
			cifsWord("Create Disposition", "uint32", 72, 76),
			cifsWord("Create Options", "uint32", 76, 80),
			cifsWord("Impersonation Level", "uint32", 80, 84),
			cifsWord("Security Flags", "uint8", 84, 85),
			cifsWord("Byte Count", "uint16", 85, 87),
		)
	}
	if create && response {
		words = append(words,
			cifsWord("OpLock Level", "uint8", 41, 42),
			cifsWord("FID", "uint16", 42, 44),
			cifsWord("Create Action", "uint32", 44, 48),
			cifsWord("Creation Time", "int64", 48, 56),
			cifsWord("Last Access Time", "int64", 56, 64),
			cifsWord("Last Write Time", "int64", 64, 72),
			cifsWord("Last Change Time", "int64", 72, 80),
			cifsWord("Extended File Attributes", "uint32", 80, 84),
			cifsWord("Allocation Size", "int64", 84, 92),
			cifsWord("End Of File", "int64", 92, 100),
			cifsWord("Resource Type", "uint16", 100, 102),
			cifsWord("Named Pipe Status", "uint16", 102, 104),
			cifsWord("Directory", "uint8", 104, 105),
			cifsWord("Byte Count", "uint16", 105, 107),
		)
	}
	if !create && !response {
		words = append(words,
			cifsWord("FID", "uint16", 41, 43),
			cifsWord("Offset Low", "uint32", 43, 47),
			cifsWord("Timeout", "uint32", 47, 51),
			cifsWord("Write Mode", "uint16", 51, 53),
			cifsWord("Remaining", "uint16", 53, 55),
			cifsWord("Data Length High", "uint16", 55, 57),
			cifsWord("Data Length Low", "uint16", 57, 59),
			cifsWord("Data Offset", "uint16", 59, 61),
			cifsWord("Offset High", "uint32", 61, 65),
			cifsWord("Byte Count", "uint16", 65, 67),
		)
	}
	if !create && response {
		words = append(words,
			cifsWord("Count Low", "uint16", 41, 43),
			cifsWord("Available", "uint16", 43, 45),
			cifsWord("Count High", "uint16", 45, 47),
			cifsWord("Reserved", "uint16", 47, 49),
			cifsWord("Byte Count", "uint16", 49, 51),
		)
	}
	for _, word := range words {
		body.Children = append(body.Children, tlsCertificateLeaf(word.name, word.typ, word.start, word.end))
	}
	info := map[string]any{}
	if create && !response {
		nameLen := int(binary.LittleEndian.Uint16(w[42:44]))
		// WC24 ends on an odd PDU offset. The following byte aligns the UTF-16
		// name to the SMB header; its value is not part of the name.
		body.Children = append(body.Children, tlsCertificateLeaf("Alignment Pad", "raw", 87, 88))
		if 88+nameLen+2 != len(w) {
			return nil, nil, fmt.Errorf("CIFS NT_CREATE_ANDX: Name Length does not match the command boundary")
		}
		if binary.LittleEndian.Uint16(w[88+nameLen:88+nameLen+2]) != 0 {
			return nil, nil, fmt.Errorf("CIFS NT_CREATE_ANDX: file name is not UTF-16 terminated")
		}
		text, err := DecodeCIFSUTF16(w[88 : 88+nameLen])
		if err != nil {
			return nil, nil, err
		}
		body.Children = append(body.Children, tlsCertificateLeaf("File Name Raw Bytes", "raw", 88, len(w)))
		info["File Name"] = text
	}
	if !create && !response {
		dataLen := int(binary.LittleEndian.Uint16(w[57:59]))
		dataOff := int(binary.LittleEndian.Uint16(w[59:61]))
		if binary.LittleEndian.Uint16(w[55:57]) != 0 {
			return nil, nil, fmt.Errorf("%w: nonzero Data Length High", ErrCIFSFileCommandUnsupported)
		}
		// Data Offset is relative to the SMB header at PDU byte 4.
		at := 4 + dataOff
		if at < fixed || at+dataLen != len(w) || dataLen != len(w)-fixed {
			return nil, nil, fmt.Errorf("CIFS WRITE_ANDX: data is outside this command")
		}
		if at > fixed {
			body.Children = append(body.Children, tlsCertificateLeaf("Alignment Pad", "raw", fixed, at))
		}
		body.Children = append(body.Children, tlsCertificateLeaf("Data", "raw", at, at+dataLen))
	}
	fields = append(fields, body)
	return fields, info, nil
}

func cifsWord(name, typ string, start, end int) struct {
	name, typ  string
	start, end int
} {
	return struct {
		name, typ  string
		start, end int
	}{name, typ, start, end}
}

func cifsOpenWriteValue(raw []byte, leaf tlsCertificateField) (any, error) {
	if leaf.Start < 0 || leaf.End > len(raw) || leaf.Start > leaf.End {
		return nil, fmt.Errorf("CIFS open/write: field %s is outside the record", leaf.Name)
	}
	b := raw[leaf.Start:leaf.End]
	if leaf.Name == "Payload Length" && len(b) == 3 {
		return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]), nil
	}
	switch leaf.Type {
	case "uint8":
		return b[0], nil
	case "uint16":
		return binary.LittleEndian.Uint16(b), nil
	case "uint32":
		return binary.LittleEndian.Uint32(b), nil
	case "int64":
		return int64(binary.LittleEndian.Uint64(b)), nil
	case "raw":
		return bytes.Clone(b), nil
	default:
		return nil, fmt.Errorf("CIFS open/write: unsupported field type %s", leaf.Type)
	}
}
