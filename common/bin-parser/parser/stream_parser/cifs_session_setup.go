package stream_parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// ErrCIFSSessionSetupUnsupported distinguishes a known layout outside this
// finite profile from a broken boundary or malformed string in its grammar.
var ErrCIFSSessionSetupUnsupported = errors.New("CIFS session setup layout unsupported")

// ValidateCIFSSessionSetupProfile uses the canonical grammar. Native callers
// invoke it only to classify a failed VM parse without matching error strings.
func ValidateCIFSSessionSetupProfile(raw []byte, response bool) error {
	_, _, err := decodeCIFSSessionSetup(raw, response)
	return err
}

// DecodeCIFSUTF16 decodes a complete, unterminated SMB Unicode string. Invalid
// surrogate sequences are refused rather than silently replaced by U+FFFD.
func DecodeCIFSUTF16(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("CIFS: odd Unicode byte count")
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	for i := 0; i < len(units); i++ {
		v := units[i]
		if v >= 0xd800 && v <= 0xdbff {
			if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", fmt.Errorf("CIFS: unpaired high surrogate")
			}
			i++
		} else if v >= 0xdc00 && v <= 0xdfff {
			return "", fmt.Errorf("CIFS: unpaired low surrogate")
		}
	}
	return string(utf16.Decode(units)), nil
}

// One explicit complete Direct TCP PDU. No token authentication, OEM codepage
// inference, active AndX chaining or connection-state mutation is performed.
// MS-SMB 2.2.4.6.1/2.2.4.6.2; terminal AndX offsets are ignored (MS-CIFS).
func decodeCIFSSessionSetup(w []byte, response bool) ([]tlsCertificateField, map[string]any, error) {
	fail := func(s string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("cifs-session-setup: %s", s)
	}
	baseSize, maxSize, wc := 63, 65598, byte(12)
	if response {
		baseSize, maxSize, wc = 47, 65582, 4
	}
	if len(w) < baseSize || len(w) > maxSize || w[0] != 0 || tlsCertificateUint24(w[1:4]) != len(w)-4 {
		return fail("complete bounded Direct TCP record required")
	}
	if binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff || w[8] != 0x73 || (w[13]&0x80 != 0) != response || w[36] != wc {
		return fail("signature, command, role or WordCount differs from selected profile")
	}
	flags2 := binary.LittleEndian.Uint16(w[14:16])
	if flags2&0x800 == 0 || flags2&0x8000 == 0 {
		return nil, nil, fmt.Errorf("%w: extended security and Unicode required", ErrCIFSSessionSetupUnsupported)
	}
	status := binary.LittleEndian.Uint32(w[9:13])
	if response && (status != 0 && (status != 0xc0000016 || flags2&0x4000 == 0)) {
		return fail("response status outside success/NTSTATUS continuation profile")
	}
	if w[37] != 0xff {
		return nil, nil, fmt.Errorf("%w: active AndX chaining", ErrCIFSSessionSetupUnsupported)
	}
	if !response && binary.LittleEndian.Uint32(w[57:61])&0x80000000 == 0 {
		return fail("request CAP_EXTENDED_SECURITY required")
	}
	if response && binary.LittleEndian.Uint16(w[41:43])&2 != 0 {
		return fail("extended-security response cannot use LANMAN key")
	}
	count := int(binary.LittleEndian.Uint16(w[baseSize-2 : baseSize]))
	minimum := 4
	if response {
		minimum = 6
	}
	if count < minimum || count != len(w)-baseSize {
		return fail("ByteCount differs from complete data boundary or protocol minimum")
	}
	blobAt := 51
	if response {
		blobAt = 43
	}
	blobLen := int(binary.LittleEndian.Uint16(w[blobAt : blobAt+2]))
	if blobLen > count {
		return fail("security blob exceeds ByteCount")
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
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37), tlsCertificateLeaf("AndX Command", "uint8", 37, 38),
		tlsCertificateLeaf("AndX Reserved", "uint8", 38, 39), tlsCertificateLeaf("AndX Offset", "uint16", 39, 41),
	}}
	if response {
		body.Children = append(body.Children, tlsCertificateLeaf("Action", "uint16", 41, 43), tlsCertificateLeaf("Security Blob Length", "uint16", 43, 45))
	} else {
		for _, f := range []struct {
			name, typ  string
			start, end int
		}{
			{"Max Buffer Size", "uint16", 41, 43}, {"Max Mpx Count", "uint16", 43, 45}, {"VC Number", "uint16", 45, 47},
			{"Session Key", "uint32", 47, 51}, {"Security Blob Length", "uint16", 51, 53},
			{"Reserved", "uint32", 53, 57}, {"Capabilities", "uint32", 57, 61},
		} {
			body.Children = append(body.Children, tlsCertificateLeaf(f.name, f.typ, f.start, f.end))
		}
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Byte Count", "uint16", baseSize-2, baseSize))
	at := baseSize + blobLen
	body.Children = append(body.Children, tlsCertificateLeaf("Security Blob", "raw", baseSize, at))
	pad := at % 2 // the SMB header starts at byte 4, an even Unicode origin
	if at+pad > len(w) {
		return fail("missing Unicode alignment pad")
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Alignment Pad", "raw", at, at+pad))
	at += pad
	for _, name := range []string{"Native OS Raw Bytes", "Native LAN Manager Raw Bytes"} {
		start := at
		if response && pad != 0 && name == "Native LAN Manager Raw Bytes" && at == len(w)-1 && w[at] == 0 {
			return nil, nil, fmt.Errorf("%w: Windows padded single-NUL NativeLanMan variation", ErrCIFSSessionSetupUnsupported)
		}
		for at+2 <= len(w) && binary.LittleEndian.Uint16(w[at:at+2]) != 0 {
			at += 2
		}
		if at+2 > len(w) {
			return fail("unterminated aligned Unicode string")
		}
		if _, err := DecodeCIFSUTF16(w[start:at]); err != nil {
			return fail(err.Error())
		}
		at += 2
		body.Children = append(body.Children, tlsCertificateLeaf(name, "raw", start, at))
	}
	body.Children = append(body.Children, tlsCertificateLeaf("Trailing Extension Bytes", "raw", at, len(w)))
	return []tlsCertificateField{direct, header, body}, map[string]any{
		"Profile": "SMB1 Unicode extended-security terminal SESSION_SETUP_ANDX", "Response": response,
		"Token Parsed": false, "Authentication Verified": false, "Active AndX Chain Parsed": false,
	}, nil
}

func parseCIFSSessionSetup(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeCIFSSessionSetup(w, response)
	}, "cifs-session-setup", "little")
}
