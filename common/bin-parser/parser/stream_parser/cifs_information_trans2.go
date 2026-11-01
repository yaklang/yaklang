package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const (
	cifsTrans2Complete   = "complete in this record"
	cifsTrans2Incomplete = "opaque: parameter or data bytes are incomplete or displaced; SMB_COM_TRANSACTION2_SECONDARY is not reconstructed"
	cifsMaxFEAEntries    = 1024
)

// MS-CIFS 2.2.4.30 and 2.2.4.31. These success layouts have no AndX and no
// SMB_STRING, so the Unicode flag does not select a different word layout.
// A non-zero response status stays unsupported.
func DecodeCIFSSetInformation2(raw []byte, response bool) (map[string]any, error) {
	return decodeCIFSInformation2(raw, true, response)
}

func DecodeCIFSQueryInformation2(raw []byte, response bool) (map[string]any, error) {
	return decodeCIFSInformation2(raw, false, response)
}

func parseCIFSSetInformation2(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsInformation2Tree(w, true, response)
	}, "cifs-set-information2", "little")
}

func parseCIFSQueryInformation2(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsInformation2Tree(w, false, response)
	}, "cifs-query-information2", "little")
}

func decodeCIFSInformation2(raw []byte, set, response bool) (map[string]any, error) {
	fields, _, err := cifsInformation2Tree(raw, set, response)
	if err != nil {
		return nil, err
	}
	out, err := cifsFieldsToMap(raw, fields)
	if err != nil {
		return nil, err
	}
	name := "SMB_COM_QUERY_INFORMATION2"
	if set {
		name = "SMB_COM_SET_INFORMATION2"
	}
	out["Packet Name"] = name
	out["Response"] = response
	out["Authentication Verified"] = false
	out["Active AndX Chain Parsed"] = false
	return out, nil
}

func cifsInformation2Tree(w []byte, set, response bool) ([]tlsCertificateField, map[string]any, error) {
	command := byte(0x23)
	wc, fixed := byte(1), 41
	if response {
		wc, fixed = 11, 61
	}
	if set {
		command = 0x22
		wc, fixed = 7, 53
		if response {
			wc, fixed = 0, 39
		}
	}
	fields, err := cifsFileCommandEnvelope(w, command, wc, fixed, fixed, response, false)
	if err != nil {
		return nil, nil, err
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37),
	}}
	switch {
	case set && !response:
		cifsAppendLeaves(&body,
			cifsLeaf("FID", "uint16", 37, 39),
			cifsLeaf("CreateDate", "uint16", 39, 41),
			cifsLeaf("CreationTime", "uint16", 41, 43),
			cifsLeaf("LastAccessDate", "uint16", 43, 45),
			cifsLeaf("LastAccessTime", "uint16", 45, 47),
			cifsLeaf("LastWriteDate", "uint16", 47, 49),
			cifsLeaf("LastWriteTime", "uint16", 49, 51),
		)
	case !set && !response:
		cifsAppendLeaves(&body, cifsLeaf("FID", "uint16", 37, 39))
	case !set && response:
		cifsAppendLeaves(&body,
			cifsLeaf("CreateDate", "uint16", 37, 39),
			cifsLeaf("CreationTime", "uint16", 39, 41),
			cifsLeaf("LastAccessDate", "uint16", 41, 43),
			cifsLeaf("LastAccessTime", "uint16", 43, 45),
			cifsLeaf("LastWriteDate", "uint16", 45, 47),
			cifsLeaf("LastWriteTime", "uint16", 47, 49),
			cifsLeaf("FileDataSize", "uint32", 49, 53),
			cifsLeaf("FileAllocationSize", "uint32", 53, 57),
			cifsLeaf("FileAttributes", "uint16", 57, 59),
		)
	}
	cifsAppendLeaves(&body, cifsLeaf("Byte Count", "uint16", fixed-2, fixed))
	fields = append(fields, body)
	return fields, map[string]any{}, nil
}

// MS-CIFS 2.2.4.46.1 and 2.2.4.46.2. Offsets are from the SMB header at PDU
// byte 4. A zero count does not read its offset. Counts that differ from the
// totals, or a non-zero response displacement, stay in this record and are
// not rebuilt from SMB_COM_TRANSACTION2_SECONDARY.
func DecodeCIFSTransaction2(raw []byte, response bool) (map[string]any, error) {
	fields, extra, err := cifsTransaction2Tree(raw, response)
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
	cifsNameASCIIAttributes(body)
	out["Packet Name"] = "SMB_COM_TRANSACTION2"
	out["Response"] = response
	out["Authentication Verified"] = false
	out["Active AndX Chain Parsed"] = false
	return out, nil
}

func parseCIFSTransaction2(node *base.Node, process func(*base.Node) (func(bool), error), response bool) error {
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return cifsTransaction2Tree(w, response)
	}, "cifs-transaction2", "little")
}

func cifsTransaction2Tree(w []byte, response bool) ([]tlsCertificateField, map[string]any, error) {
	if len(w) < 37 {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2: truncated WordCount")
	}
	wc := int(w[36])
	minWC, setupAt := 14, 63
	if response {
		minWC, setupAt = 10, 55
	}
	if wc < minWC {
		return nil, nil, fmt.Errorf("%w: WordCount %d", ErrCIFSFileCommandUnsupported, wc)
	}
	if len(w) < setupAt+2 {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2: truncated setup count")
	}
	setupCount := int(w[setupAt])
	if wc != minWC+setupCount {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2: WordCount does not equal the setup count")
	}
	fixed := setupAt + 4 + 2*setupCount
	fields, err := cifsFileCommandEnvelope(w, 0x32, byte(wc), fixed, 65582, response, false)
	if err != nil {
		return nil, nil, err
	}
	body := tlsCertificateField{Name: "Body", Start: 36, End: len(w), Children: []tlsCertificateField{
		tlsCertificateLeaf("Word Count", "uint8", 36, 37),
	}}
	if response {
		cifsAppendLeaves(&body,
			cifsLeaf("Total Parameter Count", "uint16", 37, 39),
			cifsLeaf("Total Data Count", "uint16", 39, 41),
			cifsLeaf("Reserved 1", "uint16", 41, 43),
			cifsLeaf("Parameter Count", "uint16", 43, 45),
			cifsLeaf("Parameter Offset", "uint16", 45, 47),
			cifsLeaf("Parameter Displacement", "uint16", 47, 49),
			cifsLeaf("Data Count", "uint16", 49, 51),
			cifsLeaf("Data Offset", "uint16", 51, 53),
			cifsLeaf("Data Displacement", "uint16", 53, 55),
			cifsLeaf("Setup Count", "uint8", 55, 56),
			cifsLeaf("Reserved 2", "uint8", 56, 57),
		)
	} else {
		cifsAppendLeaves(&body,
			cifsLeaf("Total Parameter Count", "uint16", 37, 39),
			cifsLeaf("Total Data Count", "uint16", 39, 41),
			cifsLeaf("Max Parameter Count", "uint16", 41, 43),
			cifsLeaf("Max Data Count", "uint16", 43, 45),
			cifsLeaf("Max Setup Count", "uint8", 45, 46),
			cifsLeaf("Reserved 1", "uint8", 46, 47),
			cifsLeaf("Flags", "uint16", 47, 49),
			cifsLeaf("Timeout", "uint32", 49, 53),
			cifsLeaf("Reserved 2", "uint16", 53, 55),
			cifsLeaf("Parameter Count", "uint16", 55, 57),
			cifsLeaf("Parameter Offset", "uint16", 57, 59),
			cifsLeaf("Data Count", "uint16", 59, 61),
			cifsLeaf("Data Offset", "uint16", 61, 63),
			cifsLeaf("Setup Count", "uint8", 63, 64),
			cifsLeaf("Reserved 3", "uint8", 64, 65),
		)
	}
	if setupCount == 1 {
		cifsAppendLeaves(&body, cifsLeaf("Setup", "uint16", setupAt+2, setupAt+4))
	} else if setupCount > 1 {
		cifsAppendLeaves(&body, cifsLeaf("Setup", "raw", setupAt+2, setupAt+2+2*setupCount))
	}
	cifsAppendLeaves(&body, cifsLeaf("Byte Count", "uint16", fixed-2, fixed))
	extra := map[string]any{}
	paramCount := int(binary.LittleEndian.Uint16(w[cifsTrans2ParamCountAt(response):]))
	paramOffset := int(binary.LittleEndian.Uint16(w[cifsTrans2ParamCountAt(response)+2:]))
	dataCount := int(binary.LittleEndian.Uint16(w[cifsTrans2DataCountAt(response):]))
	dataOffset := int(binary.LittleEndian.Uint16(w[cifsTrans2DataCountAt(response)+2:]))
	totalParam := int(binary.LittleEndian.Uint16(w[37:39]))
	totalData := int(binary.LittleEndian.Uint16(w[39:41]))
	if paramCount > totalParam || dataCount > totalData {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2: count exceeds the total in this record")
	}
	param, err := cifsTrans2Located(w, fixed, paramCount, paramOffset, !response)
	if err != nil {
		return nil, nil, err
	}
	data, err := cifsTrans2Located(w, fixed, dataCount, dataOffset, !response)
	if err != nil {
		return nil, nil, err
	}
	if param.ok && data.ok && param.start < data.end && data.start < param.end {
		return nil, nil, fmt.Errorf("CIFS TRANSACTION2: parameter and data bytes overlap")
	}
	covered := make([]cifsTrans2Span, 0, 3)
	if !response {
		if fixed >= len(w) || w[fixed] != 0 {
			return nil, nil, fmt.Errorf("CIFS TRANSACTION2: name byte must be zero")
		}
		cifsAppendLeaves(&body, cifsLeaf("Name", "uint8", fixed, fixed+1))
		covered = append(covered, cifsTrans2Span{fixed, fixed + 1, true})
	}
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
	incomplete := paramCount != totalParam || dataCount != totalData
	if response && (binary.LittleEndian.Uint16(w[47:49]) != 0 || binary.LittleEndian.Uint16(w[53:55]) != 0) {
		incomplete = true
	}
	if incomplete {
		extra["Transaction Status"] = cifsTrans2Incomplete
	} else {
		extra["Transaction Status"] = cifsTrans2Complete
		if setupCount == 1 {
			cifsTrans2Select(w, &body, extra, response, binary.LittleEndian.Uint16(w[setupAt+2:setupAt+4]), param, data)
		}
	}
	fields = append(fields, body)
	return fields, extra, nil
}

func cifsTrans2Select(w []byte, body *tlsCertificateField, extra map[string]any, response bool, setup uint16, param, data cifsTrans2Span) {
	switch setup {
	case 0x0008:
		extra["Setup Name"] = "TRANS2_SET_FILE_INFORMATION"
		if response || !param.ok || param.end-param.start != 6 {
			return
		}
		// MS-CIFS 2.2.6.9.1. The reserved word is ignored, not rejected.
		cifsAppendLeaves(body,
			cifsLeaf("FID", "uint16", param.start, param.start+2),
			cifsLeaf("Information Level", "uint16", param.start+2, param.start+4),
			cifsLeaf("Reserved", "uint16", param.start+4, param.start+6),
		)
		level := binary.LittleEndian.Uint16(w[param.start+2 : param.start+4])
		if level == 0x0002 && data.ok {
			extra["Information Level Name"] = "SMB_INFO_SET_EAS"
			if items, sizeAt, sizeEnd, ok := cifsFEAFields(w[data.start:data.end], data.start); ok {
				cifsAppendLeaves(body, cifsLeaf("EA List Size", "uint32", sizeAt, sizeEnd))
				body.Children = append(body.Children, tlsCertificateField{Name: "Extended Attributes", List: true, Start: sizeEnd, End: data.end, Children: items})
				extra["Data Status"] = "SMB_INFO_SET_EAS"
				return
			}
			extra["Data Status"] = "opaque: SMB_INFO_SET_EAS does not fill this data block"
		} else if data.ok {
			extra["Data Status"] = "opaque: information level has no selected data layout"
		}
	case 0x0005:
		extra["Setup Name"] = "TRANS2_QUERY_PATH_INFORMATION"
		if response || !param.ok {
			return
		}
		// MS-CIFS 2.2.6.6.1. The file name is an SMB_STRING inside this parameter block.
		levelAt := param.start
		if param.end-param.start < 6 || binary.LittleEndian.Uint32(w[levelAt+2:levelAt+6]) != 0 {
			extra["File Name Status"] = "opaque: QUERY_PATH_INFORMATION parameter block is not InformationLevel, zero Reserved, and one SMB_STRING"
			return
		}
		name := w[levelAt+6 : param.end]
		unicode := binary.LittleEndian.Uint16(w[14:16])&0x8000 != 0
		if !unicode {
			extra["File Name Status"] = "opaque: OEM SMB_STRING is not selected"
			cifsAppendLeaves(body, cifsLeaf("Information Level", "uint16", levelAt, levelAt+2), cifsLeaf("Reserved", "uint32", levelAt+2, levelAt+6))
			return
		}
		if len(name) < 2 || len(name)%2 != 0 || (param.start+6-4)%2 != 0 || binary.LittleEndian.Uint16(name[len(name)-2:]) != 0 {
			extra["File Name Status"] = "opaque: Unicode SMB_STRING does not fill this parameter block"
			return
		}
		for at := 0; at+2 < len(name); at += 2 {
			if binary.LittleEndian.Uint16(name[at:at+2]) == 0 {
				extra["File Name Status"] = "opaque: Unicode SMB_STRING does not fill this parameter block"
				return
			}
		}
		text, err := DecodeCIFSUTF16(name[:len(name)-2])
		if err != nil {
			extra["File Name Status"] = "opaque: Unicode SMB_STRING does not fill this parameter block"
			return
		}
		cifsAppendLeaves(body,
			cifsLeaf("Information Level", "uint16", levelAt, levelAt+2),
			cifsLeaf("Reserved", "uint32", levelAt+2, levelAt+6),
			cifsLeaf("File Name Raw Bytes", "raw", levelAt+6, param.end),
		)
		extra["File Name"] = text
		extra["File Name Range"] = []any{uint64(levelAt + 6), uint64(param.end - 2)}
		if binary.LittleEndian.Uint16(w[levelAt:levelAt+2]) == 0x0107 {
			extra["Information Level Name"] = "SMB_QUERY_FILE_ALL_INFO"
		}
	}
}

// AnnotateCIFSTransaction2Response types a complete response from the matched
// request's setup word and information level. The response command byte alone
// does not select a data layout.
func AnnotateCIFSTransaction2Response(fields map[string]any, setup, level uint16, hasLevel bool) {
	body, _ := fields["Body"].(map[string]any)
	if body == nil || body["Transaction Status"] != cifsTrans2Complete {
		return
	}
	ranges, _ := body["Wire Field Ranges"].(map[string]any)
	switch setup {
	case 0x0008:
		if params, ok := body["Parameters"].([]byte); ok && len(params) == 2 && ranges["Parameters"] != nil {
			body["Ea Error Offset"] = binary.LittleEndian.Uint16(params)
			ranges["Ea Error Offset"] = ranges["Parameters"]
			if hasLevel && level == 0x0002 {
				body["Ea Error Offset Status"] = "offset into SMB_INFO_SET_EAS; zero means this response reported no EA error"
			} else {
				body["Ea Error Offset Status"] = "EaErrorOffset is meaningful only for SMB_INFO_SET_EAS"
			}
		}
		if _, ok := body["Data"]; ok {
			body["Data Status"] = "opaque: TRANS2_SET_FILE_INFORMATION response defines no data bytes"
		}
	case 0x0005:
		if hasLevel && level > 0x0100 {
			body["Response Parameter Status"] = "one parameter word is ignored for information levels above 0x0100"
		}
		if hasLevel && level == 0x0107 && cifsApplyQueryFileAllInfo(body, ranges) {
			body["Data Status"] = "SMB_QUERY_FILE_ALL_INFO from the matched TRANS2_QUERY_PATH_INFORMATION request"
			return
		}
		body["Data Status"] = "opaque: matched information level is not a selected SMB_QUERY_FILE_ALL_INFO block"
	default:
		if _, ok := body["Data"]; ok {
			body["Data Status"] = "opaque: setup subcommand has no selected information layout"
		}
	}
}

func cifsApplyQueryFileAllInfo(body map[string]any, ranges map[string]any) bool {
	data, _ := body["Data"].([]byte)
	span, _ := ranges["Data"].([]any)
	if len(data) < 72 || len(span) != 2 {
		return false
	}
	start, ok := span[0].(uint64)
	if !ok || 72+int(binary.LittleEndian.Uint32(data[68:72])) != len(data) {
		return false
	}
	nameLen := int(binary.LittleEndian.Uint32(data[68:72]))
	if nameLen%2 != 0 {
		return false
	}
	text, err := DecodeCIFSUTF16(data[72:])
	if err != nil {
		return false
	}
	put := func(name, typ string, at, width int) {
		body[name] = cifsSpanValue(data[at:at+width], typ)
		ranges[name] = []any{start + uint64(at), start + uint64(at+width)}
	}
	put("Creation Time", "uint64", 0, 8)
	put("Last Access Time", "uint64", 8, 8)
	put("Last Write Time", "uint64", 16, 8)
	put("Last Change Time", "uint64", 24, 8)
	put("Extended File Attributes", "uint32", 32, 4)
	put("Reserved 1", "uint32", 36, 4)
	put("Allocation Size", "int64", 40, 8)
	put("End Of File", "int64", 48, 8)
	put("Number Of Links", "uint32", 56, 4)
	put("Delete Pending", "uint8", 60, 1)
	put("Directory", "uint8", 61, 1)
	put("Reserved 2", "uint16", 62, 2)
	put("EA Size", "uint32", 64, 4)
	put("File Name Length", "uint32", 68, 4)
	body["File Name"] = text
	body["File Name Raw Bytes"] = bytes.Clone(data[72:])
	ranges["File Name"] = []any{start + 72, start + uint64(len(data))}
	ranges["File Name Raw Bytes"] = ranges["File Name"]
	return true
}

func cifsFEAFields(data []byte, at int) ([]tlsCertificateField, int, int, bool) {
	if len(data) < 4 || int(binary.LittleEndian.Uint32(data[:4])) != len(data) {
		return nil, 0, 0, false
	}
	var items []tlsCertificateField
	pos := 4
	for pos < len(data) {
		if len(items) >= cifsMaxFEAEntries || len(data)-pos < 4 {
			return nil, 0, 0, false
		}
		nameLen := int(data[pos+1])
		valueLen := int(binary.LittleEndian.Uint16(data[pos+2 : pos+4]))
		need := 4 + nameLen + 1 + valueLen
		if nameLen > 255 || need > len(data)-pos || data[pos+4+nameLen] != 0 {
			return nil, 0, 0, false
		}
		base := at + pos
		items = append(items, tlsCertificateField{Name: "Extended Attribute", Start: base, End: base + need, Children: []tlsCertificateField{
			tlsCertificateLeaf("Extended Attribute Flag", "uint8", base, base+1),
			tlsCertificateLeaf("Attribute Name Length", "uint8", base+1, base+2),
			tlsCertificateLeaf("Attribute Value Length", "uint16", base+2, base+4),
			tlsCertificateLeaf("Attribute Name Raw Bytes", "raw", base+4, base+5+nameLen),
			tlsCertificateLeaf("Attribute Value", "raw", base+5+nameLen, base+need),
		}})
		pos += need
	}
	if pos != len(data) {
		return nil, 0, 0, false
	}
	return items, at, at + 4, true
}

func cifsNameASCIIAttributes(body map[string]any) {
	eas, _ := body["Extended Attributes"].([]map[string]any)
	for _, ea := range eas {
		raw, _ := ea["Attribute Name Raw Bytes"].([]byte)
		ranges, _ := ea["Wire Field Ranges"].(map[string]any)
		span, _ := ranges["Attribute Name Raw Bytes"].([]any)
		if len(raw) == 0 || raw[len(raw)-1] != 0 || len(span) != 2 {
			continue
		}
		text, ok := cifsPrintableASCII(raw[:len(raw)-1])
		if !ok {
			ea["Attribute Name Status"] = "OEM code page is not selected"
			continue
		}
		ea["Attribute Name"] = text
		end, _ := span[1].(uint64)
		ranges["Attribute Name"] = []any{span[0], end - 1}
	}
	if span, ok := body["File Name Range"].([]any); ok {
		body["Wire Field Ranges"].(map[string]any)["File Name"] = span
		delete(body, "File Name Range")
	}
}

func cifsPrintableASCII(raw []byte) (string, bool) {
	for _, b := range raw {
		if b < 0x20 || b > 0x7e {
			return "", false
		}
	}
	return string(raw), true
}

type cifsTrans2Span struct {
	start, end int
	ok         bool
}

func cifsTrans2Located(w []byte, byteStart, count, smbOffset int, request bool) (cifsTrans2Span, error) {
	if count == 0 {
		return cifsTrans2Span{}, nil
	}
	start := 4 + smbOffset
	end := start + count
	if smbOffset < 0 || start < byteStart || end > len(w) || request && start < byteStart+1 {
		return cifsTrans2Span{}, fmt.Errorf("CIFS TRANSACTION2: parameter or data bytes are outside this command")
	}
	return cifsTrans2Span{start, end, true}, nil
}

func cifsTrans2Pads(byteStart, byteEnd int, covered []cifsTrans2Span) []cifsTrans2Span {
	for i := 1; i < len(covered); i++ {
		item := covered[i]
		j := i
		for j > 0 && covered[j-1].start > item.start {
			covered[j] = covered[j-1]
			j--
		}
		covered[j] = item
	}
	var pads []cifsTrans2Span
	cursor := byteStart
	for _, item := range covered {
		if item.start > cursor {
			pads = append(pads, cifsTrans2Span{cursor, item.start, true})
		}
		if item.end > cursor {
			cursor = item.end
		}
	}
	if cursor < byteEnd {
		pads = append(pads, cifsTrans2Span{cursor, byteEnd, true})
	}
	return pads
}

func cifsTrans2ParamCountAt(response bool) int {
	if response {
		return 43
	}
	return 55
}

func cifsTrans2DataCountAt(response bool) int {
	if response {
		return 49
	}
	return 59
}

func cifsLeaf(name, typ string, start, end int) tlsCertificateField {
	return tlsCertificateLeaf(name, typ, start, end)
}

func cifsAppendLeaves(body *tlsCertificateField, leaves ...tlsCertificateField) {
	body.Children = append(body.Children, leaves...)
}

func cifsFieldsToMap(raw []byte, fields []tlsCertificateField) (map[string]any, error) {
	out := map[string]any{}
	for _, field := range fields {
		value, err := cifsFieldValue(raw, field)
		if err != nil {
			return nil, err
		}
		out[field.Name] = value
	}
	return out, nil
}

func cifsFieldValue(raw []byte, field tlsCertificateField) (any, error) {
	if field.List {
		items := make([]map[string]any, 0, len(field.Children))
		for _, child := range field.Children {
			value, err := cifsFieldValue(raw, child)
			if err != nil {
				return nil, err
			}
			item, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("CIFS: list element %s is not a structure", child.Name)
			}
			items = append(items, item)
		}
		return items, nil
	}
	if len(field.Children) > 0 {
		child := map[string]any{"Wire Field Ranges": map[string]any{}}
		for _, item := range field.Children {
			value, err := cifsFieldValue(raw, item)
			if err != nil {
				return nil, err
			}
			child[item.Name] = value
			if item.Type != "" && len(item.Children) == 0 && !item.List {
				child["Wire Field Ranges"].(map[string]any)[item.Name] = []any{uint64(item.Start), uint64(item.End)}
			}
		}
		return child, nil
	}
	return cifsScalar(raw, field)
}

func cifsScalar(raw []byte, field tlsCertificateField) (any, error) {
	if field.Start < 0 || field.End > len(raw) || field.Start > field.End {
		return nil, fmt.Errorf("CIFS: field %s is outside the record", field.Name)
	}
	b := raw[field.Start:field.End]
	if field.Name == "Payload Length" && len(b) == 3 {
		return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]), nil
	}
	return cifsSpanValue(b, field.Type), nil
}

func cifsSpanValue(b []byte, typ string) any {
	switch typ {
	case "uint8":
		return b[0]
	case "uint16":
		return binary.LittleEndian.Uint16(b)
	case "uint32":
		return binary.LittleEndian.Uint32(b)
	case "uint64":
		return binary.LittleEndian.Uint64(b)
	case "int64":
		return int64(binary.LittleEndian.Uint64(b))
	case "raw":
		return bytes.Clone(b)
	default:
		return bytes.Clone(b)
	}
}
