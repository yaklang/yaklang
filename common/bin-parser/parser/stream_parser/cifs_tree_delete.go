package stream_parser

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Fixed words plus the largest literal 16-bit ByteCount. Callers must apply
// their own tighter transport/resource limits before allocating Nodes.
const (
	CIFSTreeConnectAndXRequestMaxBytes  = 47 + 65535
	CIFSTreeConnectAndXResponseMaxBytes = 45 + 65535
	CIFSDeleteRequestMaxBytes           = 41 + 65535
	CIFSDeleteResponseMaxBytes          = 39
)

// ErrCIFSFileCommandUnsupported means that this finite profile does not
// interpret the selected wire form. It does not certify that the form is bad.
var ErrCIFSFileCommandUnsupported = errors.New("SMB1 file-command form unsupported")

func ValidateCIFSTreeConnectAndXProfile(raw []byte, response bool) error {
	_, _, err := decodeCIFSTreeConnectAndX(raw, response)
	return err
}

func ValidateCIFSDeleteProfile(raw []byte, response bool) error {
	_, _, err := decodeCIFSDelete(raw, response)
	return err
}

// A complete DirectTCP record, not a stream detector. The two response profiles
// deliberately cover successful fixed layouts only; other statuses stay
// unsupported and are never treated as authentication/session proof.
func cifsFileCommandEnvelope(w []byte, command, wc byte, fixed, maximum int, response, unicode bool) ([]tlsCertificateField, error) {
	if len(w) < 39 || len(w) > maximum || w[0] != 0 || tlsCertificateUint24(w[1:4]) != len(w)-4 {
		return nil, fmt.Errorf("CIFS file command: complete bounded DirectTCP record required")
	}
	if binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff || w[8] != command || (w[13]&0x80 != 0) != response {
		return nil, fmt.Errorf("CIFS file command: signature, command or direction differs from selected profile")
	}
	if w[36] != wc {
		return nil, fmt.Errorf("%w: WordCount %d", ErrCIFSFileCommandUnsupported, w[36])
	}
	if len(w) < fixed {
		return nil, fmt.Errorf("CIFS file command: truncated fixed words")
	}
	if response && binary.LittleEndian.Uint32(w[9:13]) != 0 {
		return nil, fmt.Errorf("%w: non-success response layout", ErrCIFSFileCommandUnsupported)
	}
	if unicode && binary.LittleEndian.Uint16(w[14:16])&0x8000 == 0 {
		return nil, fmt.Errorf("%w: Unicode string profile required", ErrCIFSFileCommandUnsupported)
	}
	if command == 0x75 && w[37] != 0xff {
		return nil, fmt.Errorf("%w: active AndX chaining", ErrCIFSFileCommandUnsupported)
	}
	if int(binary.LittleEndian.Uint16(w[fixed-2:fixed])) != len(w)-fixed {
		return nil, fmt.Errorf("CIFS file command: ByteCount differs from complete command boundary")
	}
	direct := tlsCertificateField{Name: "Direct TCP", Start: 0, End: 4, Children: []tlsCertificateField{
		{Name: "Message Type", Type: "uint8", Start: 0, End: 1},
		{Name: "Payload Length", Type: "uint32", Endian: "big", Start: 1, End: 4},
	}}
	header := tlsCertificateField{Name: "Header", Start: 4, End: 36}
	for _, f := range []struct {
		name, typ  string
		start, end int
	}{
		{"Protocol ID", "uint32", 4, 8}, {"Command", "uint8", 8, 9}, {"Status", "uint32", 9, 13},
		{"Flags", "uint8", 13, 14}, {"Flags 2", "uint16", 14, 16}, {"PID High", "uint16", 16, 18},
		{"Security Features", "raw", 18, 26}, {"Reserved", "uint16", 26, 28}, {"Tree ID", "uint16", 28, 30},
		{"Process ID Low", "uint16", 30, 32}, {"User ID", "uint16", 32, 34}, {"Multiplex ID", "uint16", 34, 36},
	} {
		header.Children = append(header.Children, tlsCertificateLeaf(f.name, f.typ, f.start, f.end))
	}
	return []tlsCertificateField{direct, header}, nil
}

// Append only the explicit alignment byte, without treating its ignored value
// as a string byte. SMB begins at PDU byte 4, so its parity equals PDU parity.
func cifsFileCommandPad(w []byte, body *tlsCertificateField, at *int) error {
	pad := *at % 2
	if pad > len(w)-*at {
		return fmt.Errorf("CIFS file command: missing Unicode alignment byte")
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Alignment Pad", "raw", *at, *at+pad))
	*at += pad
	return nil
}

func cifsFileCommandUnicode(w []byte, body *tlsCertificateField, at *int, name string) error {
	start := *at
	for *at+2 <= len(w) && binary.LittleEndian.Uint16(w[*at:*at+2]) != 0 {
		*at += 2
	}
	if *at+2 > len(w) {
		return fmt.Errorf("CIFS file command: unterminated Unicode %s", name)
	}
	if _, err := DecodeCIFSUTF16(w[start:*at]); err != nil {
		return fmt.Errorf("CIFS file command: %s: %w", name, err)
	}
	*at += 2
	body.Children = append(body.Children, tlsCertificateLeaf(name+" Raw Bytes", "raw", start, *at))
	return nil
}

// Service is OEM even when FLAGS2_UNICODE is set. This finite profile decodes
// the ASCII-compatible service vocabulary and preserves its NUL in raw bytes;
// it never guesses a locale/codepage for non-ASCII bytes or rejects an unknown
// printable ASCII service solely because it is outside a familiar enum.
func cifsFileCommandService(w []byte, body *tlsCertificateField, at *int) error {
	start := *at
	for *at < len(w) && w[*at] != 0 {
		if w[*at] < 0x20 || w[*at] > 0x7e {
			return fmt.Errorf("%w: non-ASCII OEM service", ErrCIFSFileCommandUnsupported)
		}
		*at += 1
	}
	if *at == len(w) {
		return fmt.Errorf("CIFS file command: unterminated OEM Service")
	}
	*at += 1
	body.Children = append(body.Children, tlsCertificateLeaf("Service Raw Bytes", "raw", start, *at))
	return nil
}

func decodeCIFSTreeConnectAndX(w []byte, response bool) ([]tlsCertificateField, map[string]any, error) {
	fixed, maximum, wc := 47, CIFSTreeConnectAndXRequestMaxBytes, byte(4)
	if response {
		fixed, maximum, wc = 45, CIFSTreeConnectAndXResponseMaxBytes, 3
	}
	fields, err := cifsFileCommandEnvelope(w, 0x75, wc, fixed, maximum, response, true)
	if err != nil {
		return nil, nil, err
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37), tlsCertificateLeaf("AndX Command", "uint8", 37, 38),
		tlsCertificateLeaf("AndX Reserved", "uint8", 38, 39), tlsCertificateLeaf("AndX Offset", "uint16", 39, 41),
	}}
	if response {
		body.Children = append(body.Children, tlsCertificateLeaf("Optional Support", "uint16", 41, 43))
	} else {
		body.Children = append(body.Children, tlsCertificateLeaf("Flags", "uint16", 41, 43), tlsCertificateLeaf("Password Length", "uint16", 43, 45))
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Byte Count", "uint16", fixed-2, fixed))
	at := fixed
	if response {
		if err = cifsFileCommandService(w, &body, &at); err != nil {
			return nil, nil, err
		}
		if err = cifsFileCommandPad(w, &body, &at); err != nil {
			return nil, nil, err
		}
		if err = cifsFileCommandUnicode(w, &body, &at, "Native File System"); err != nil {
			return nil, nil, err
		}
	} else {
		passwordLength := int(binary.LittleEndian.Uint16(w[43:45]))
		if passwordLength > len(w)-at {
			return nil, nil, fmt.Errorf("CIFS tree connect: PasswordLength exceeds ByteCount")
		}
		body.Children = append(body.Children, tlsCertificateLeaf("Password", "raw", at, at+passwordLength))
		at += passwordLength
		if err = cifsFileCommandPad(w, &body, &at); err != nil {
			return nil, nil, err
		}
		if err = cifsFileCommandUnicode(w, &body, &at, "Path"); err != nil {
			return nil, nil, err
		}
		if err = cifsFileCommandService(w, &body, &at); err != nil {
			return nil, nil, err
		}
	}
	if at != len(w) {
		return nil, nil, fmt.Errorf("%w: unclassified trailing data", ErrCIFSFileCommandUnsupported)
	}
	fields = append(fields, body)
	return fields, map[string]any{"Profile": "SMB1 Unicode terminal TREE_CONNECT_ANDX", "Response": response, "Active AndX Chain Parsed": false, "Authentication Verified": false}, nil
}

func decodeCIFSDelete(w []byte, response bool) ([]tlsCertificateField, map[string]any, error) {
	fixed, maximum, wc := 41, CIFSDeleteRequestMaxBytes, byte(1)
	if response {
		fixed, maximum, wc = 39, CIFSDeleteResponseMaxBytes, 0
	}
	fields, err := cifsFileCommandEnvelope(w, 0x06, wc, fixed, maximum, response, !response)
	if err != nil {
		return nil, nil, err
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{tlsCertificateLeaf("Word Count", "uint8", 36, 37)}}
	if !response {
		body.Children = append(body.Children, tlsCertificateLeaf("Search Attributes", "uint16", 37, 39))
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Byte Count", "uint16", fixed-2, fixed))
	if !response {
		if fixed == len(w) || w[fixed] != 4 {
			return nil, nil, fmt.Errorf("CIFS delete: BufferFormat must be 4")
		}
		body.Children = append(body.Children, tlsCertificateLeaf("Buffer Format", "uint8", fixed, fixed+1))
		at := fixed + 1
		if err = cifsFileCommandPad(w, &body, &at); err != nil {
			return nil, nil, err
		}
		if err = cifsFileCommandUnicode(w, &body, &at, "File Name"); err != nil {
			return nil, nil, err
		}
		if at != len(w) {
			return nil, nil, fmt.Errorf("%w: unclassified trailing data", ErrCIFSFileCommandUnsupported)
		}
	}
	fields = append(fields, body)
	return fields, map[string]any{"Profile": "SMB1 Unicode DELETE", "Response": response, "Authentication Verified": false}, nil
}

func parseCIFSTreeConnectAndX(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeCIFSTreeConnectAndX(w, response)
	}, "cifs-tree-connect-andx", "little")
}

func parseCIFSDelete(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeCIFSDelete(w, response) }, "cifs-delete", "little")
}
