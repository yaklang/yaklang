package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
)

// MS-CIFS 2.2.4.47.1. WordCount is 9. There is no SMB_STRING name and no
// defined secondary response. Counts in this record must stay strictly inside
// the totals when a total is nonzero; displacement is recorded, not applied.
func DecodeCIFSTransaction2Secondary(raw []byte) (map[string]any, error) {
	fields, extra, err := cifsTransaction2SecondaryTree(raw)
	if err != nil {
		return nil, err
	}
	out, err := cifsFieldsToMap(raw, fields)
	if err != nil {
		return nil, err
	}
	body := out["Body"].(map[string]any)
	for key, value := range extra {
		body[key] = value
	}
	out["Packet Name"] = "SMB_COM_TRANSACTION2_SECONDARY"
	out["Response"] = false
	out["Authentication Verified"] = false
	out["Active AndX Chain Parsed"] = false
	return out, nil
}

func parseCIFSTransaction2Secondary(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsTransaction2SecondaryTree(w)
	}, "cifs-transaction2-secondary", "little")
}

func cifsTransaction2SecondaryTree(w []byte) ([]tlsCertificateField, map[string]any, error) {
	if len(w) < 37 || w[13]&0x80 != 0 {
		return nil, nil, fmt.Errorf("%w: TRANSACTION2_SECONDARY has no response", ErrCIFSFileCommandUnsupported)
	}
	if w[36] != 9 {
		return nil, nil, fmt.Errorf("%w: WordCount %d", ErrCIFSFileCommandUnsupported, w[36])
	}
	const fixed = 57
	fields, err := cifsFileCommandEnvelope(w, 0x33, 9, fixed, 65582, false, false)
	if err != nil {
		return nil, nil, err
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37),
	}}
	cifsAppendLeaves(&body,
		cifsLeaf("Total Parameter Count", "uint16", 37, 39),
		cifsLeaf("Total Data Count", "uint16", 39, 41),
		cifsLeaf("Parameter Count", "uint16", 41, 43),
		cifsLeaf("Parameter Offset", "uint16", 43, 45),
		cifsLeaf("Parameter Displacement", "uint16", 45, 47),
		cifsLeaf("Data Count", "uint16", 47, 49),
		cifsLeaf("Data Offset", "uint16", 49, 51),
		cifsLeaf("Data Displacement", "uint16", 51, 53),
		cifsLeaf("FID", "uint16", 53, 55),
		cifsLeaf("Byte Count", "uint16", 55, 57),
	)
	totalParam := int(binary.LittleEndian.Uint16(w[37:39]))
	totalData := int(binary.LittleEndian.Uint16(w[39:41]))
	paramCount := int(binary.LittleEndian.Uint16(w[41:43]))
	paramOffset := int(binary.LittleEndian.Uint16(w[43:45]))
	dataCount := int(binary.LittleEndian.Uint16(w[47:49]))
	dataOffset := int(binary.LittleEndian.Uint16(w[49:51]))
	if paramCount > totalParam || dataCount > totalData {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2_SECONDARY: count exceeds the total in this record")
	}
	if totalParam > 0 && paramCount >= totalParam || totalData > 0 && dataCount >= totalData {
		return nil, nil, fmt.Errorf("%w: secondary count must be shorter than the total", ErrCIFSFileCommandUnsupported)
	}
	param, err := cifsTrans2Located(w, fixed, paramCount, paramOffset, false)
	if err != nil {
		return nil, nil, err
	}
	data, err := cifsTrans2Located(w, fixed, dataCount, dataOffset, false)
	if err != nil {
		return nil, nil, err
	}
	if param.ok && data.ok && param.start < data.end && data.start < param.end {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2_SECONDARY: parameter and data bytes overlap")
	}
	var covered []cifsTrans2Span
	if param.ok {
		cifsAppendLeaves(&body, cifsLeaf("Parameters", "raw", param.start, param.end))
		covered = append(covered, param)
	}
	if data.ok {
		cifsAppendLeaves(&body, cifsLeaf("Data", "raw", data.start, data.end))
		covered = append(covered, data)
	}
	pads := cifsTrans2Pads(fixed, len(w), covered)
	if len(pads) > 0 {
		list := tlsCertificateField{Name: "Padding", List: true, Start: pads[0].start, End: pads[len(pads)-1].end}
		for _, pad := range pads {
			list.Children = append(list.Children, tlsCertificateField{Name: "Pad", Start: pad.start, End: pad.end, Children: []tlsCertificateField{
				tlsCertificateLeaf("Bytes", "raw", pad.start, pad.end),
			}})
		}
		body.Children = append(body.Children, list)
	}
	fields = append(fields, body)
	return fields, map[string]any{"Transaction Status": "secondary slice; logical buffer is not complete in this record"}, nil
}

// MS-CIFS 2.2.3.2 and 2.2.3.3: an error response uses WordCount 0 and ByteCount 0.
// The success words of the command are not present.
func DecodeCIFSEmptyError(raw []byte) (map[string]any, error) {
	if len(raw) < 39 || raw[0] != 0 || binary.LittleEndian.Uint32(raw[4:8]) != 0x424d53ff {
		return nil, fmt.Errorf("CIFS error reply: complete DirectTCP record required")
	}
	if raw[13]&0x80 == 0 || binary.LittleEndian.Uint32(raw[9:13]) == 0 || len(raw) != 39 || raw[36] != 0 || binary.LittleEndian.Uint16(raw[37:39]) != 0 || tlsCertificateUint24(raw[1:4]) != 35 {
		return nil, fmt.Errorf("%w: SMB1 error reply is a non-success WC0/BC0 response", ErrCIFSFileCommandUnsupported)
	}
	name := cifsCommandName(raw[8])
	if name == "" || raw[8] == 0x33 {
		return nil, fmt.Errorf("%w: SMB1 command 0x%02x has no empty error profile", ErrCIFSFileCommandUnsupported, raw[8])
	}
	direct := tlsCertificateField{Name: "Direct TCP", Start: 0, End: 4, Children: []tlsCertificateField{
		tlsCertificateLeaf("Message Type", "uint8", 0, 1),
		tlsCertificateLeaf("Payload Length", "uint32", 1, 4),
	}}
	direct.Children[1].Endian = "big"
	header := tlsCertificateField{Name: "Header", Start: 4, End: 36}
	for _, field := range []struct {
		name, typ  string
		start, end int
	}{
		{"Protocol ID", "uint32", 4, 8}, {"Command", "uint8", 8, 9}, {"Status", "uint32", 9, 13},
		{"Flags", "uint8", 13, 14}, {"Flags 2", "uint16", 14, 16}, {"PID High", "uint16", 16, 18},
		{"Security Features", "raw", 18, 26}, {"Reserved", "uint16", 26, 28}, {"Tree ID", "uint16", 28, 30},
		{"Process ID Low", "uint16", 30, 32}, {"User ID", "uint16", 32, 34}, {"Multiplex ID", "uint16", 34, 36},
	} {
		header.Children = append(header.Children, tlsCertificateLeaf(field.name, field.typ, field.start, field.end))
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: 39, Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37),
		tlsCertificateLeaf("Byte Count", "uint16", 37, 39),
	}}
	out, err := cifsFieldsToMap(raw, []tlsCertificateField{direct, header, body})
	if err != nil {
		return nil, err
	}
	out["Packet Name"] = name
	out["Response"] = true
	out["Error Layout"] = true
	out["Success Layout"] = false
	out["Authentication Verified"] = false
	out["Signature Verified"] = false
	out["Session Established"] = false
	return out, nil
}

// DecodeCIFSQueryPathParameters reads one TRANS2_QUERY_PATH_INFORMATION
// parameter block. Unicode and OEM are selected by the caller's FLAGS2 bit.
// OEM bytes are never decoded as UTF-16.
func DecodeCIFSQueryPathParameters(params []byte, unicode bool) (string, bool) {
	if len(params) < 7 || binary.LittleEndian.Uint32(params[2:6]) != 0 {
		return "", false
	}
	name := params[6:]
	if unicode {
		if len(name) < 2 || len(name)%2 != 0 || binary.LittleEndian.Uint16(name[len(name)-2:]) != 0 {
			return "", false
		}
		for at := 0; at+2 < len(name); at += 2 {
			if binary.LittleEndian.Uint16(name[at:at+2]) == 0 {
				return "", false
			}
		}
		text, err := DecodeCIFSUTF16(name[:len(name)-2])
		if err != nil {
			return "", false
		}
		return text, true
	}
	return cifsOEMString(name)
}

func cifsOEMString(raw []byte) (string, bool) {
	if len(raw) < 1 || raw[len(raw)-1] != 0 {
		return "", false
	}
	content := raw[:len(raw)-1]
	for _, b := range content {
		if b < 0x20 || b > 0x7e {
			return "", false
		}
	}
	return string(content), true
}

// MS-CIFS 2.2.8.4.1, 2.2.8.4.3, 2.2.8.4.4, 2.2.8.4.5 and 2.2.8.4.6. The record
// must fill the data block exactly. Level 0x0104 here is SMB_SET_FILE_END_OF_FILE_INFO,
// not the QUERY_PATH name record that uses the same numeric level. Any other
// width stays opaque.
func cifsSetInformationRecord(data []byte, at int, level uint16) (tlsCertificateField, string, bool) {
	var leaves []tlsCertificateField
	label := ""
	switch level {
	case 0x0001:
		if len(data) != 22 {
			return tlsCertificateField{}, "", false
		}
		label = "SMB_INFO_STANDARD"
		leaves = []tlsCertificateField{
			cifsLeaf("CreationDate", "uint16", at, at+2), cifsLeaf("CreationTime", "uint16", at+2, at+4),
			cifsLeaf("LastAccessDate", "uint16", at+4, at+6), cifsLeaf("LastAccessTime", "uint16", at+6, at+8),
			cifsLeaf("LastWriteDate", "uint16", at+8, at+10), cifsLeaf("LastWriteTime", "uint16", at+10, at+12),
			cifsLeaf("Reserved", "raw", at+12, at+22),
		}
	case 0x0101:
		if len(data) != 40 {
			return tlsCertificateField{}, "", false
		}
		label = "SMB_SET_FILE_BASIC_INFO"
		leaves = cifsBasicInfoLeaves(at)
	case 0x0102:
		if len(data) != 1 {
			return tlsCertificateField{}, "", false
		}
		label = "SMB_SET_FILE_DISPOSITION_INFO"
		leaves = []tlsCertificateField{cifsLeaf("Delete Pending", "uint8", at, at+1)}
	case 0x0103:
		if len(data) != 8 {
			return tlsCertificateField{}, "", false
		}
		label = "SMB_SET_FILE_ALLOCATION_INFO"
		leaves = []tlsCertificateField{cifsLeaf("Allocation Size", "int64", at, at+8)}
	case 0x0104:
		if len(data) != 8 {
			return tlsCertificateField{}, "", false
		}
		label = "SMB_SET_FILE_END_OF_FILE_INFO"
		leaves = []tlsCertificateField{cifsLeaf("End Of File", "int64", at, at+8)}
	default:
		return tlsCertificateField{}, "", false
	}
	return tlsCertificateField{Name: "Information Record", Start: at, End: at + len(data), Children: leaves}, label, true
}

func cifsBasicInfoLeaves(at int) []tlsCertificateField {
	return []tlsCertificateField{
		cifsLeaf("Creation Time", "uint64", at, at+8), cifsLeaf("Last Access Time", "uint64", at+8, at+16),
		cifsLeaf("Last Write Time", "uint64", at+16, at+24), cifsLeaf("Last Change Time", "uint64", at+24, at+32),
		cifsLeaf("Extended File Attributes", "uint32", at+32, at+36), cifsLeaf("Reserved", "uint32", at+36, at+40),
	}
}

// MS-CIFS 2.2.8.3.6, 2.2.8.3.7, 2.2.8.3.8 and 2.2.8.3.9. SMB_QUERY_FILE_STANDARD_INFO
// in this document is 22 bytes and has no trailing reserved word.
func cifsApplyQueryRecord(body map[string]any, ranges map[string]any, level uint16) bool {
	data, _ := body["Data"].([]byte)
	span, _ := ranges["Data"].([]any)
	if len(span) != 2 {
		return false
	}
	start, ok := span[0].(uint64)
	if !ok {
		return false
	}
	record := map[string]any{"Wire Field Ranges": map[string]any{}}
	put := func(name, typ string, at, width int) {
		record[name] = cifsSpanValue(data[at:at+width], typ)
		record["Wire Field Ranges"].(map[string]any)[name] = []any{start + uint64(at), start + uint64(at+width)}
	}
	switch level {
	case 0x0101:
		if len(data) != 40 {
			return false
		}
		put("Creation Time", "uint64", 0, 8)
		put("Last Access Time", "uint64", 8, 8)
		put("Last Write Time", "uint64", 16, 8)
		put("Last Change Time", "uint64", 24, 8)
		put("Extended File Attributes", "uint32", 32, 4)
		put("Reserved", "uint32", 36, 4)
		body["Information Level Name"] = "SMB_QUERY_FILE_BASIC_INFO"
	case 0x0102:
		if len(data) != 22 {
			return false
		}
		put("Allocation Size", "int64", 0, 8)
		put("End Of File", "int64", 8, 8)
		put("Number Of Links", "uint32", 16, 4)
		put("Delete Pending", "uint8", 20, 1)
		put("Directory", "uint8", 21, 1)
		body["Information Level Name"] = "SMB_QUERY_FILE_STANDARD_INFO"
	case 0x0103:
		if len(data) != 4 {
			return false
		}
		put("EA Size", "uint32", 0, 4)
		body["Information Level Name"] = "SMB_QUERY_FILE_EA_INFO"
	case 0x0104, 0x0108:
		if len(data) < 4 {
			return false
		}
		n := int(binary.LittleEndian.Uint32(data[:4]))
		put("File Name Length", "uint32", 0, 4)
		label := "SMB_QUERY_FILE_NAME_INFO"
		if level == 0x0108 {
			label = "SMB_QUERY_FILE_ALT_NAME_INFO"
		}
		body["Information Level Name"] = label
		if n != len(data)-4 || n < 0 || n%2 != 0 {
			body["Data Status"] = "opaque: file name count does not fill this data block"
			log.Debugf("cifs: %s name left opaque; count %d does not fill %d data bytes", label, n, len(data)-4)
			body["Information Record"] = record
			return true
		}
		text, err := DecodeCIFSUTF16(data[4:])
		if err != nil {
			body["Data Status"] = "opaque: file name count does not fill this data block"
			log.Debugf("cifs: %s name left opaque; UTF-16 bytes are not a complete name", label)
			body["Information Record"] = record
			return true
		}
		record["File Name"] = text
		record["File Name Raw Bytes"] = append([]byte(nil), data[4:]...)
		record["Wire Field Ranges"].(map[string]any)["File Name"] = []any{start + 4, start + uint64(len(data))}
		record["Wire Field Ranges"].(map[string]any)["File Name Raw Bytes"] = record["Wire Field Ranges"].(map[string]any)["File Name"]
		body["File Name"] = text
	case 0x0109:
		// MS-CIFS 2.2.8.3.12. One entry whose name fills the data block.
		if len(data) < 24 {
			return false
		}
		next := binary.LittleEndian.Uint32(data[0:4])
		n := int(binary.LittleEndian.Uint32(data[4:8]))
		put("Next Entry Offset", "uint32", 0, 4)
		put("Stream Name Length", "uint32", 4, 4)
		put("Stream Size", "int64", 8, 8)
		put("Stream Allocation Size", "int64", 16, 8)
		body["Information Level Name"] = "SMB_QUERY_FILE_STREAM_INFO"
		if next != 0 || n != len(data)-24 || n < 0 || n%2 != 0 {
			body["Data Status"] = "opaque: stream name count does not fill this data block"
			log.Debugf("cifs: SMB_QUERY_FILE_STREAM_INFO name left opaque; count %d next %d", n, next)
			body["Information Record"] = record
			return true
		}
		text, err := DecodeCIFSUTF16(data[24:])
		if err != nil {
			body["Data Status"] = "opaque: stream name count does not fill this data block"
			log.Debugf("cifs: SMB_QUERY_FILE_STREAM_INFO name left opaque; UTF-16 bytes are not a complete name")
			body["Information Record"] = record
			return true
		}
		record["Stream Name"] = text
		record["Stream Name Raw Bytes"] = append([]byte(nil), data[24:]...)
		record["Wire Field Ranges"].(map[string]any)["Stream Name"] = []any{start + 24, start + uint64(len(data))}
		record["Wire Field Ranges"].(map[string]any)["Stream Name Raw Bytes"] = record["Wire Field Ranges"].(map[string]any)["Stream Name"]
	case 0x010B:
		// MS-CIFS 2.2.8.3.13. Exactly 16 bytes; a neighboring width is not this record.
		if len(data) != 16 {
			return false
		}
		put("Compressed File Size", "int64", 0, 8)
		put("Compression Format", "uint16", 8, 2)
		put("Compression Unit Shift", "uint8", 10, 1)
		put("Chunk Shift", "uint8", 11, 1)
		put("Cluster Shift", "uint8", 12, 1)
		put("Reserved", "raw", 13, 3)
		body["Information Level Name"] = "SMB_QUERY_FILE_COMPRESSION_INFO"
	default:
		return false
	}
	body["Information Record"] = record
	return true
}

func cifsCommandName(command byte) string {
	switch command {
	case 0x72:
		return "SMB_COM_NEGOTIATE"
	case 0x73:
		return "SMB_COM_SESSION_SETUP_ANDX"
	case 0x75:
		return "SMB_COM_TREE_CONNECT_ANDX"
	case 0x06:
		return "SMB_COM_DELETE"
	case 0xa2:
		return "SMB_COM_NT_CREATE_ANDX"
	case 0x2f:
		return "SMB_COM_WRITE_ANDX"
	case 0x22:
		return "SMB_COM_SET_INFORMATION2"
	case 0x23:
		return "SMB_COM_QUERY_INFORMATION2"
	case 0x32:
		return "SMB_COM_TRANSACTION2"
	case 0x33:
		return "SMB_COM_TRANSACTION2_SECONDARY"
	default:
		return ""
	}
}
