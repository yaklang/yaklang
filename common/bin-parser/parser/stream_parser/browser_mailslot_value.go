package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// One complete unfragmented direct-group NetBIOS datagram, unscoped names,
// second-class BROWSE mailslot and MS-BRWS 0x0c/0x0f announcement layouts.
// Offsets refer to original input bytes; no alignment repair is performed.
const browserMailslotMaxBytes = 65527

type browserMailslotField struct {
	Name, Type, Endian string
	Start, End         int
	Value              *string
	Info               map[string]any
	Children           []browserMailslotField
}

func browserMailslotLeaf(name, typ string, start, end int) browserMailslotField {
	return browserMailslotField{Name: name, Type: typ, Start: start, End: end}
}

func browserMailslotString(name string, wire []byte, start, end, max int) (browserMailslotField, error) {
	f := browserMailslotLeaf(name, "string", start, end)
	if start >= end || end > len(wire) || end-start > max {
		return f, fmt.Errorf("browser-mailslot: %s string length outside profile", name)
	}
	n := bytes.IndexByte(wire[start:end], 0)
	if n < 0 {
		return f, fmt.Errorf("browser-mailslot: %s lacks NUL terminator", name)
	}
	for _, b := range wire[start : start+n] {
		if b > 127 {
			return f, fmt.Errorf("browser-mailslot: %s requires ASCII", name)
		}
	}
	value := string(wire[start : start+n])
	f.Value = &value
	f.Info = map[string]any{"String Bytes": n, "Terminator Offset": start + n, "Ignored Bytes After Terminator": end - start - n - 1}
	return f, nil
}

func decodeBrowserMailslotDatagram(wire []byte, strict bool) ([]browserMailslotField, map[string]any, error) {
	fail := func(s string) ([]browserMailslotField, map[string]any, error) {
		return nil, nil, fmt.Errorf("browser-mailslot: %s", s)
	}
	if len(wire) < 82+69+1 || len(wire) > browserMailslotMaxBytes {
		return fail("complete datagram size outside profile")
	}
	if wire[0] != 0x11 {
		return fail("only direct-group datagram is in this profile")
	}
	if wire[1]&0xf0 != 0 || wire[1]&3 != 2 || binary.BigEndian.Uint16(wire[12:14]) != 0 {
		return fail("reserved flags or fragmented datagram outside profile")
	}
	if int(binary.BigEndian.Uint16(wire[10:12])) != len(wire)-14 {
		return fail("Datagram Length differs from exact payload boundary")
	}
	fields := []browserMailslotField{
		browserMailslotLeaf("Datagram Message Type", "uint8", 0, 1), browserMailslotLeaf("Datagram Flags", "uint8", 1, 2),
		browserMailslotLeaf("Datagram ID", "uint16", 2, 4), browserMailslotLeaf("Datagram Source IP", "raw", 4, 8),
		browserMailslotLeaf("Datagram Source Port", "uint16", 8, 10), browserMailslotLeaf("Datagram Length", "uint16", 10, 12),
		browserMailslotLeaf("Packet Offset", "uint16", 12, 14),
	}
	fields[1].Info = map[string]any{"Source Node Type": wire[1] >> 2, "First Fragment": true, "More Fragments": false}
	var decodedNames [2][16]byte
	for i, name := range []string{"Source NetBIOS Name", "Destination NetBIOS Name"} {
		at := 14 + i*34
		if wire[at] != 32 || wire[at+33] != 0 {
			return fail("only uncompressed unscoped 32-octet encoded NetBIOS names are in profile")
		}
		for j := 0; j < 16; j++ {
			a, b := wire[at+1+j*2], wire[at+2+j*2]
			if a < 'A' || a > 'P' || b < 'A' || b > 'P' {
				return fail("invalid first-level NetBIOS name nibble")
			}
			decodedNames[i][j] = (a-'A')<<4 | (b - 'A')
		}
		value := strings.TrimRight(string(decodedNames[i][:15]), " ")
		encoded := browserMailslotLeaf("Encoded Name", "string", at+1, at+33)
		encoded.Value = &value
		encoded.Info = map[string]any{"Decoded Name Bytes": append([]byte(nil), decodedNames[i][:]...), "Suffix": decodedNames[i][15]}
		fields = append(fields, browserMailslotField{Name: name, Start: at, End: at + 34, Children: []browserMailslotField{
			browserMailslotLeaf("Encoded Name Length", "uint8", at, at+1), encoded, browserMailslotLeaf("Scope Terminator", "uint8", at+33, at+34),
		}})
	}
	const smbStart = 82
	smb := wire[smbStart:]
	u16 := func(at int) uint16 { return binary.LittleEndian.Uint16(smb[at : at+2]) }
	if !bytes.Equal(smb[:4], []byte{255, 'S', 'M', 'B'}) || smb[4] != 0x25 || smb[32] != 17 || smb[59] != 3 || u16(61) != 1 {
		return fail("requires SMB_COM_TRANSACTION mailslot write with WordCount17/SetupCount3/Opcode1")
	}
	if u16(63) > 9 || u16(65) != 2 {
		return fail("mailslot Priority outside 0..9 or non-second-class mailslot")
	}
	dataOffset, dataCount := int(u16(57)), int(u16(55))
	if dataCount != int(u16(35)) || dataOffset < 70 || dataOffset > len(smb) || dataCount != len(smb)-dataOffset || int(u16(67)) != len(smb)-69 {
		return fail("SMB TotalDataCount/DataCount/DataOffset/ByteCount differ from complete record")
	}
	nameEnd := bytes.IndexByte(smb[69:dataOffset], 0)
	if nameEnd < 0 {
		return fail("mailslot name lacks bounded terminator")
	}
	nameEnd += 70
	if !strings.EqualFold(string(smb[69:nameEnd-1]), `\MAILSLOT\BROWSE`) {
		return fail("only BROWSE mailslot is in this profile")
	}
	padding := dataOffset - nameEnd
	if padding > 3 {
		return fail("mailslot padding exceeds 0..3 octet profile")
	}
	aligned := dataOffset%4 == 0
	if strict && !aligned {
		return fail("DataOffset violates MS-MAIL 32-bit data alignment")
	}
	smbField := browserMailslotField{Name: "SMB Mailslot", Endian: "little", Start: smbStart, End: len(wire)}
	for _, f := range []struct {
		name, typ  string
		start, end int
	}{
		{"Protocol ID", "raw", 0, 4}, {"SMB Command", "uint8", 4, 5}, {"SMB Status", "uint32", 5, 9}, {"SMB Flags", "uint8", 9, 10}, {"SMB Flags2", "uint16", 10, 12},
		{"PID High", "uint16", 12, 14}, {"Header Signature Bytes", "raw", 14, 22}, {"Header Reserved", "raw", 22, 24}, {"TID", "uint16", 24, 26}, {"PID Low", "uint16", 26, 28}, {"UID", "uint16", 28, 30}, {"MID", "uint16", 30, 32},
		{"Word Count", "uint8", 32, 33}, {"Total Parameter Count", "uint16", 33, 35}, {"Total Data Count", "uint16", 35, 37}, {"Max Parameter Count", "uint16", 37, 39}, {"Max Data Count", "uint16", 39, 41}, {"Max Setup Count", "uint8", 41, 42}, {"Transaction Reserved", "uint8", 42, 43}, {"Transaction Flags", "uint16", 43, 45},
		{"Timeout", "uint32", 45, 49}, {"Transaction Reserved2", "raw", 49, 51}, {"Parameter Count", "uint16", 51, 53}, {"Parameter Offset", "uint16", 53, 55}, {"Data Count", "uint16", 55, 57}, {"Data Offset", "uint16", 57, 59}, {"Setup Count", "uint8", 59, 60}, {"Transaction Reserved3", "uint8", 60, 61},
		{"Mailslot Opcode", "uint16", 61, 63}, {"Priority", "uint16", 63, 65}, {"Class", "uint16", 65, 67}, {"Byte Count", "uint16", 67, 69},
	} {
		smbField.Children = append(smbField.Children, browserMailslotLeaf(f.name, f.typ, smbStart+f.start, smbStart+f.end))
	}
	mailslotName, err := browserMailslotString("Mailslot Name", wire, smbStart+69, smbStart+nameEnd, 256)
	if err != nil {
		return nil, nil, err
	}
	smbField.Children = append(smbField.Children, mailslotName, browserMailslotLeaf("Mailslot Padding", "raw", smbStart+nameEnd, smbStart+dataOffset))
	start := smbStart + dataOffset
	if dataCount < 33 || (wire[start] != 0x0c && wire[start] != 0x0f) {
		return fail("only complete Domain/LocalMasterAnnouncement is in profile")
	}
	domain := wire[start] == 0x0c
	name, major, minor, lastName, maxLast := "Server Name", "OS Version Major", "OS Version Minor", "Comment", 43
	if domain {
		name, major, minor, lastName, maxLast = "Machine Group", "Browser Config Version Major", "Browser Config Version Minor", "Local Master Browser Name", 16
	}
	fixedName, err := browserMailslotString(name, wire, start+6, start+22, 16)
	if err != nil {
		return nil, nil, err
	}
	last, err := browserMailslotString(lastName, wire, start+32, len(wire), maxLast)
	if err != nil {
		return nil, nil, err
	}
	if last.Info["Ignored Bytes After Terminator"].(int) != 0 {
		return fail("unexpected bytes after final announcement string")
	}
	signature := binary.LittleEndian.Uint16(wire[start+30 : start+32])
	if !domain && signature != 0xaa55 {
		return fail("LocalMasterAnnouncement Signature must be 0xaa55")
	}
	versionMajor, versionMinor := "Browser Config Version Major", "Browser Config Version Minor"
	if domain {
		versionMajor, versionMinor = "Browser Version Major", "Browser Version Minor"
	}
	browser := browserMailslotField{Name: "Browser Announcement", Start: start, End: len(wire), Children: []browserMailslotField{
		browserMailslotLeaf("Browser Opcode", "uint8", start, start+1), browserMailslotLeaf("Update Count", "uint8", start+1, start+2), browserMailslotLeaf("Periodicity", "uint32", start+2, start+6), fixedName,
		browserMailslotLeaf(major, "uint8", start+22, start+23), browserMailslotLeaf(minor, "uint8", start+23, start+24), browserMailslotLeaf("Server Type", "uint32", start+24, start+28),
		browserMailslotLeaf(versionMajor, "uint8", start+28, start+29), browserMailslotLeaf(versionMinor, "uint8", start+29, start+30), browserMailslotLeaf("Browser Signature", "uint16", start+30, start+32), last,
	}}
	smbField.Children = append(smbField.Children, browser)
	fields = append(fields, smbField)
	info := map[string]any{
		"Profile": "Explicit unfragmented direct-group BROWSE receiver layout observation", "Strict Alignment Required": strict, "Data Alignment Conformant": aligned,
		"Data Offset Origin": "SMB header first byte", "Sender Conformance Validated": false, "Byte Count Validated": true,
		"Ignored Receiver Fields":    "MS-MAIL ignored header/parameter/reserved fields and padding; MS-BRWS UpdateCount/fixed-name suffix bytes retained verbatim",
		"Connection State Validated": false, "Endpoint Identity Proven": false, "Declared Names Verified": false,
		"Transport Source Correlated": false, "Other Browser Opcodes Decoded": false, "Datagram Reassembly Performed": false,
		"Browser Opcode": wire[start], "Browser Body Offset": start, "SMB Offset": smbStart,
		"Browser Version Advisory Applicable": domain,
		"Browser Version Advisory Conformant": !domain || wire[start+28] == 15 && wire[start+29] == 1 && signature == 0xaa55,
	}
	return fields, info, nil
}
