package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Authentication envelopes are decoded only at the host protocol's exact token
// boundary. Searching an arbitrary payload for NTLMSSP would be false evidence.
// RFC 4178; MS-NLMP 2.2.1; MS-SMB2 2.2.5/2.2.6. No key/MIC is verified here.
func decodeAuthenticationEnvelope(w []byte) map[string]any {
	if bytes.HasPrefix(w, []byte("NTLMSSP\x00")) {
		info, err := decodeNTLMEnvelope(w)
		if err != nil {
			return map[string]any{"Status": "malformed", "Error": err.Error(), "Authentication Verified": false}
		}
		return info
	}
	if len(w) == 0 || w[0] != 0x60 && w[0] != 0xa1 {
		return map[string]any{"Status": "opaque", "Authentication Verified": false}
	}
	info, err := stream_parser.DecodeGSSAPIToken(w)
	if err != nil {
		return map[string]any{"Status": "malformed", "Error": err.Error(), "Authentication Verified": false}
	}
	info["Protocol"], info["Status"], info["Authentication Verified"] = "SPNEGO", "decoded", false
	if token, ok := info["Mechanism Token"].([]byte); ok && bytes.HasPrefix(token, []byte("NTLMSSP\x00")) {
		if selected, _ := info["Selected Mechanism"].(string); selected != "" && selected != "1.3.6.1.4.1.311.2.2.10" {
			info["Mechanism Error"] = "selected mechanism does not match NTLMSSP token"
			return info
		}
		child, err := decodeNTLMEnvelope(token)
		if err != nil {
			info["Mechanism Error"] = err.Error()
		} else {
			info["Mechanism"] = child
			info["Mechanism Semantics Decoded"] = true
		}
	}
	return info
}

func decodeNTLMEnvelope(w []byte) (map[string]any, error) {
	if len(w) < 12 || len(w) > 65535 || !bytes.Equal(w[:8], []byte("NTLMSSP\x00")) {
		return nil, fmt.Errorf("ntlm: invalid bounded signature/header")
	}
	typ := binary.LittleEndian.Uint32(w[8:12])
	header, flagsAt := 0, 0
	var descriptors []int
	name := ""
	switch typ {
	case 1:
		header, flagsAt, descriptors, name = 32, 12, []int{16, 24}, "NEGOTIATE"
	case 2:
		header, flagsAt, descriptors, name = 48, 20, []int{12, 40}, "CHALLENGE"
	case 3:
		header, flagsAt, descriptors, name = 64, 60, []int{12, 20, 28, 36, 44, 52}, "AUTHENTICATE"
	default:
		return nil, fmt.Errorf("ntlm: unsupported message type")
	}
	if len(w) < header {
		return nil, fmt.Errorf("ntlm: truncated fixed header")
	}
	flags := binary.LittleEndian.Uint32(w[flagsAt : flagsAt+4])
	if flags&0x02000000 != 0 {
		header += 8
	}
	if len(w) < header {
		return nil, fmt.Errorf("ntlm: truncated version")
	}
	info := map[string]any{"Protocol": "NTLMSSP", "Status": "decoded", "Message Type": typ, "Message Name": name, "Negotiate Flags": flags, "Authentication Verified": false, "MIC Verified": false, "Identity Evidence": "client assertion; no credential verification"}
	if typ == 2 {
		info["Server Challenge"] = hex.EncodeToString(w[24:32])
		info["Identity Evidence"] = "server challenge observed; no credential verification"
	}
	names := []string{"Domain", "Workstation"}
	if typ == 2 {
		names = []string{"Target Name", "Target Info"}
	}
	if typ == 3 {
		names = []string{"LM Response", "NT Response", "Domain", "User Name", "Workstation", "Session Key"}
	}
	for i, at := range descriptors {
		// Flags gate optional descriptors in NEGOTIATE/CHALLENGE; MaxLength is
		// ignored on receipt by MS-NLMP and cannot make missing bytes valid.
		if typ == 1 && (i == 0 && flags&0x1000 == 0 || i == 1 && flags&0x2000 == 0) {
			continue
		}
		if typ == 2 && (i == 0 && flags&4 == 0 || i == 1 && flags&0x00800000 == 0) {
			continue
		}
		n := int(binary.LittleEndian.Uint16(w[at : at+2]))
		offset := uint64(binary.LittleEndian.Uint32(w[at+4 : at+8]))
		if n == 0 {
			continue
		}
		if offset < uint64(header) || offset > uint64(len(w)) || uint64(n) > uint64(len(w))-offset {
			return nil, fmt.Errorf("ntlm: %s security buffer outside token", names[i])
		}
		data := w[int(offset) : int(offset)+n]
		if names[i] == "Domain" || names[i] == "User Name" || names[i] == "Workstation" || names[i] == "Target Name" {
			unicode := flags&1 != 0 && typ != 1
			if unicode {
				if n%2 != 0 {
					return nil, fmt.Errorf("ntlm: odd UTF-16 string length")
				}
				units := make([]uint16, n/2)
				for j := range units {
					units[j] = binary.LittleEndian.Uint16(data[j*2:])
				}
				for j := 0; j < len(units); j++ {
					if units[j] >= 0xd800 && units[j] <= 0xdbff {
						if j+1 >= len(units) || units[j+1] < 0xdc00 || units[j+1] > 0xdfff {
							return nil, fmt.Errorf("ntlm: invalid UTF-16 surrogate")
						}
						j++
					} else if units[j] >= 0xdc00 && units[j] <= 0xdfff {
						return nil, fmt.Errorf("ntlm: unpaired UTF-16 surrogate")
					}
				}
				info[names[i]] = string(utf16.Decode(units))
			} else {
				info[names[i]+" Bytes"] = bytes.Clone(data)
				info["String Encoding"] = "OEM code page not supplied"
			}
		} else {
			info[names[i]+" Bytes"] = n
		}
	}
	return info, nil
}
