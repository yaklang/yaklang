package parser

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// These projectors share the exact successful canonical Nodes with native
// callers. They project only literal source bytes, never request/connection
// state. Decoded text excludes NUL; its owned Raw Bytes retain the terminator.
func ProjectCIFSTreeConnectAndXFields(node *base.Node, raw []byte) (map[string]any, error) {
	return projectCIFSFileCommandFields(node, raw, true)
}

func ProjectCIFSDeleteFields(node *base.Node, raw []byte) (map[string]any, error) {
	return projectCIFSFileCommandFields(node, raw, false)
}

func projectCIFSFileCommandFields(node *base.Node, raw []byte, tree bool) (fields map[string]any, err error) {
	defer func() {
		if v := recover(); v != nil {
			fields, err = nil, fmt.Errorf("CIFS file-command projection: invalid Node: %v", v)
		}
	}()
	if node == nil || node.Cfg == nil {
		return nil, fmt.Errorf("CIFS file-command projection: missing Node")
	}
	requestEntry, responseEntry := "CIFSDirectTCPDeleteRequest", "CIFSDirectTCPDeleteResponse"
	if tree {
		requestEntry, responseEntry = "CIFSDirectTCPTreeConnectAndXRequest", "CIFSDirectTCPTreeConnectAndXResponse"
	}
	response := node.Name == responseEntry
	if node.Name != requestEntry && !response {
		return nil, fmt.Errorf("CIFS file-command projection: unselected entry %q", node.Name)
	}
	// Also bind a mutable Node to the same explicit wire profile. A matching name
	// or plausible leaf map alone cannot authorize a field/range projection.
	if tree {
		err = stream_parser.ValidateCIFSTreeConnectAndXProfile(raw, response)
	} else {
		err = stream_parser.ValidateCIFSDeleteProfile(raw, response)
	}
	if err != nil {
		return nil, err
	}
	parts, err := cifsProjectionChildren(node, "Direct TCP", "Header", "Body")
	if err != nil {
		return nil, err
	}
	p := cifsFieldProjection{raw: raw}
	fields = map[string]any{"Wire Field Ranges": map[string]any{}}
	direct := map[string]any{"Wire Field Ranges": map[string]any{}}
	dc, err := cifsProjectionChildren(parts[0], "Message Type", "Payload Length")
	if err != nil {
		return nil, err
	}
	if err = p.literal(direct, "Message Type", dc[0], "uint8", 1, false); err != nil {
		return nil, err
	}
	if err = p.literal(direct, "Payload Length", dc[1], "uint32", 3, false); err != nil {
		return nil, err
	}
	fields["Direct TCP"] = direct
	header := map[string]any{"Wire Field Ranges": map[string]any{}}
	hnames := []string{"Protocol ID", "Command", "Status", "Flags", "Flags 2", "PID High", "Security Features", "Reserved", "Tree ID", "Process ID Low", "User ID", "Multiplex ID"}
	hc, err := cifsProjectionChildren(parts[1], hnames...)
	if err != nil {
		return nil, err
	}
	for i, spec := range []struct {
		typ   string
		width int
	}{{"uint32", 4}, {"uint8", 1}, {"uint32", 4}, {"uint8", 1}, {"uint16", 2}, {"uint16", 2}, {"raw", 8}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}} {
		if err = p.literal(header, hnames[i], hc[i], spec.typ, spec.width, true); err != nil {
			return nil, err
		}
	}
	fields["Header"] = header
	type spec struct {
		name, typ string
		width     int
	}
	specs := []spec{{"Word Count", "uint8", 1}}
	if tree {
		specs = append(specs, spec{"AndX Command", "uint8", 1}, spec{"AndX Reserved", "uint8", 1}, spec{"AndX Offset", "uint16", 2})
		if response {
			specs = append(specs, spec{"Optional Support", "uint16", 2})
		} else {
			specs = append(specs, spec{"Flags", "uint16", 2}, spec{"Password Length", "uint16", 2})
		}
	} else if !response {
		specs = append(specs, spec{"Search Attributes", "uint16", 2})
	}
	specs = append(specs, spec{"Byte Count", "uint16", 2})
	if tree && response {
		specs = append(specs, spec{"Service Raw Bytes", "raw", -1}, spec{"Alignment Pad", "raw", -1}, spec{"Native File System Raw Bytes", "raw", -1})
	} else if tree {
		specs = append(specs, spec{"Password", "raw", -1}, spec{"Alignment Pad", "raw", -1}, spec{"Path Raw Bytes", "raw", -1}, spec{"Service Raw Bytes", "raw", -1})
	} else if !response {
		specs = append(specs, spec{"Buffer Format", "uint8", 1}, spec{"Alignment Pad", "raw", -1}, spec{"File Name Raw Bytes", "raw", -1})
	}
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.name)
	}
	bc, err := cifsProjectionChildren(parts[2], names...)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"Wire Field Ranges": map[string]any{}}
	for i, s := range specs {
		width := s.width
		if s.name == "Password" {
			width = int(body["Password Length"].(uint16))
		}
		if s.name == "Alignment Pad" {
			width = int(p.at % 2)
		}
		if err = p.literal(body, s.name, bc[i], s.typ, width, true); err != nil {
			return nil, err
		}
		if s.typ == "raw" && len(body[s.name].([]byte)) == 0 {
			body[s.name] = []byte{}
		}
	}
	if p.at != uint64(len(raw)) {
		return nil, fmt.Errorf("CIFS file-command projection: incomplete source coverage")
	}
	unicodeNames := []string{}
	if tree && response {
		unicodeNames = append(unicodeNames, "Native File System")
	} else if tree {
		unicodeNames = append(unicodeNames, "Path")
	} else if !response {
		unicodeNames = append(unicodeNames, "File Name")
	}
	for _, name := range unicodeNames {
		key := name + " Raw Bytes"
		b := body[key].([]byte)
		if len(b) < 2 || b[len(b)-2] != 0 || b[len(b)-1] != 0 {
			return nil, fmt.Errorf("CIFS file-command projection: missing Unicode terminator")
		}
		text, e := stream_parser.DecodeCIFSUTF16(b[:len(b)-2])
		if e != nil || strings.IndexByte(text, 0) >= 0 {
			return nil, fmt.Errorf("CIFS file-command projection: invalid Unicode string")
		}
		body[name] = text
		span := body["Wire Field Ranges"].(map[string]any)[key].([]any)
		cifsProjectionRange(body, name, [2]uint64{span[0].(uint64), span[1].(uint64) - 2})
	}
	if tree {
		b := body["Service Raw Bytes"].([]byte)
		if len(b) == 0 || b[len(b)-1] != 0 {
			return nil, fmt.Errorf("CIFS file-command projection: missing OEM Service terminator")
		}
		for _, ch := range b[:len(b)-1] {
			if ch < 0x20 || ch > 0x7e {
				return nil, fmt.Errorf("CIFS file-command projection: invalid ASCII-compatible Service")
			}
		}
		body["Service"] = string(b[:len(b)-1])
		span := body["Wire Field Ranges"].(map[string]any)["Service Raw Bytes"].([]any)
		cifsProjectionRange(body, "Service", [2]uint64{span[0].(uint64), span[1].(uint64) - 1})
		fields["AndX Offset Ignored"] = true
		fields["Service Encoding"] = "OEM (ASCII-compatible subset)"
	}
	fields["Body"], fields["Response"] = body, response
	fields["Packet Name"] = "SMB_COM_DELETE"
	if tree {
		fields["Packet Name"] = "SMB_COM_TREE_CONNECT_ANDX"
	}
	if len(unicodeNames) != 0 {
		fields["String Encoding"] = "UTF-16LE"
	}
	fields["Authentication Verified"], fields["Signature Verified"], fields["Session Established"] = false, false, false
	return fields, nil
}
