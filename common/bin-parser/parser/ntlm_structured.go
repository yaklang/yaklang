package parser

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// ParseNTLMSSPFields exposes the bounded identity fields of an NTLMSSP
// message without attempting to recover secrets or decrypt RPC payloads.
// Offsets are checked against the token before any slice is read.
func ParseNTLMSSPFields(w []byte) (map[string]any, bool) {
	if len(w) < 12 || string(w[:8]) != "NTLMSSP\x00" {
		return nil, false
	}
	messageType := binary.LittleEndian.Uint32(w[8:12])
	fields := map[string]any{"Message Type": messageType}
	switch messageType {
	case 1:
		if len(w) < 32 {
			return nil, false
		}
		flags := binary.LittleEndian.Uint32(w[12:16])
		domain, ok := ntlmMessageString(w, 16, flags, 32)
		if !ok {
			return nil, false
		}
		workstation, ok := ntlmMessageString(w, 24, flags, 32)
		if !ok {
			return nil, false
		}
		fields["Negotiate Flags"] = uint64(flags)
		fields["Domain"] = domain
		fields["Workstation"] = workstation
	case 2:
		if len(w) < 48 {
			return nil, false
		}
		target, ok := ntlmMessageString(w, 12, binary.LittleEndian.Uint32(w[20:24]), 48)
		if !ok {
			return nil, false
		}
		fields["Negotiate Flags"] = uint64(binary.LittleEndian.Uint32(w[20:24]))
		fields["Target Name"] = target
	case 3:
		if len(w) < 64 {
			return nil, false
		}
		flags := binary.LittleEndian.Uint32(w[60:64])
		domain, ok := ntlmMessageString(w, 28, flags, 64)
		if !ok {
			return nil, false
		}
		user, ok := ntlmMessageString(w, 36, flags, 64)
		if !ok {
			return nil, false
		}
		workstation, ok := ntlmMessageString(w, 44, flags, 64)
		if !ok {
			return nil, false
		}
		fields["Negotiate Flags"] = uint64(flags)
		fields["Domain"] = domain
		fields["User"] = user
		fields["Workstation"] = workstation
		fields["NT Response Length"] = uint64(binary.LittleEndian.Uint16(w[20:22]))
		fields["LM Response Length"] = uint64(binary.LittleEndian.Uint16(w[12:14]))
		fields["Session Key Length"] = uint64(binary.LittleEndian.Uint16(w[52:54]))
	default:
		return nil, false
	}
	return fields, true
}

func ntlmMessageString(w []byte, descriptorOffset int, flags uint32, minOffset int) (string, bool) {
	if descriptorOffset < 0 || descriptorOffset+8 > len(w) {
		return "", false
	}
	length := int(binary.LittleEndian.Uint16(w[descriptorOffset : descriptorOffset+2]))
	offset := int(binary.LittleEndian.Uint32(w[descriptorOffset+4 : descriptorOffset+8]))
	if length == 0 {
		return "", true
	}
	if offset < minOffset || offset > len(w) || length > len(w)-offset {
		return "", false
	}
	raw := w[offset : offset+length]
	if flags&1 == 0 {
		return strings.TrimRight(string(raw), "\x00"), true
	}
	if len(raw)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(units)), true
}
