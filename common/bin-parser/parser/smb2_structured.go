package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const smb2StructuredMaxCommands = 4096

// captureSMB2Structured parses every command in one SMB2 PDU. The YAML SMB2
// entry parses a single command and consumes the remaining compound bytes as
// an opaque tail; this adapter splits the PDU at NextCommand boundaries and
// replays the same rule for each span, so a compound keeps every command's
// own fields instead of only the first.
func captureSMB2Structured(data []byte) (map[string]any, bool, error) {
	commands, ok, err := parseSMB2Commands(data)
	if err != nil {
		return nil, false, err
	}
	if !ok || len(commands) == 0 {
		return nil, false, nil
	}
	if len(commands) == 1 {
		return commands[0], true, nil
	}
	// Clone the first command before attaching the command list. Reusing
	// commands[0] would make Commands[0] point back to the top-level map and
	// create a JSON cycle.
	out := make(map[string]any, len(commands[0])+3)
	for key, value := range commands[0] {
		out[key] = value
	}
	out["Compound"] = true
	out["Compound Count"] = len(commands)
	out["Commands"] = commands
	return out, true, nil
}

func parseSMB2Commands(data []byte) ([]map[string]any, bool, error) {
	if len(data) < 64 || binary.LittleEndian.Uint32(data[:4]) != 0x424d53fe {
		return nil, false, nil
	}
	var commands []map[string]any
	for at := 0; ; {
		if len(commands) >= smb2StructuredMaxCommands {
			return nil, false, fmt.Errorf("smb2: compound command count exceeds %d", smb2StructuredMaxCommands)
		}
		if len(data)-at < 64 {
			return nil, false, fmt.Errorf("smb2: truncated compound header at offset %d", at)
		}
		if binary.LittleEndian.Uint32(data[at:at+4]) != 0x424d53fe {
			return nil, false, fmt.Errorf("smb2: compound command at offset %d has no SMB2 signature", at)
		}
		next := int(binary.LittleEndian.Uint32(data[at+20 : at+24]))
		end := len(data)
		if next != 0 {
			if next < 64 || next%8 != 0 || next > len(data)-at {
				return nil, false, fmt.Errorf("smb2: NextCommand at offset %d leaves the PDU", at)
			}
			end = at + next
		}
		span := data[at:end]
		reader := &smb2StructuredReader{Reader: bytes.NewReader(span), bits: uint64(len(span)) * 8}
		node, err := ParseBinary(reader, "application-layer.smb2", "SMB2")
		if err != nil {
			return nil, false, fmt.Errorf("smb2: command at offset %d: %w", at, err)
		}
		if reader.Len() != 0 {
			return nil, false, fmt.Errorf("smb2: command at offset %d left %d unconsumed bytes", at, reader.Len())
		}
		fields, ok := stream_parser.NodeToMap(node).(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("smb2: command at offset %d did not project to a map", at)
		}
		attachSMB2DirectoryOutput(fields)
		commands = append(commands, fields)
		if next == 0 {
			break
		}
		at = end
	}
	return commands, true, nil
}

type smb2StructuredReader struct {
	*bytes.Reader
	bits uint64
}

func (r *smb2StructuredReader) InputBitLength() uint64 { return r.bits }

// attachSMB2DirectoryOutput decodes a QUERY_DIRECTORY response's output buffer
// even when no matched request is available in this single PDU. The shared
// decoder selects the class-independent prefix and the UTF-16 name; a matched
// request, when present in native analysis, can refine the class-specific tail.
func attachSMB2DirectoryOutput(fields map[string]any) {
	response, ok := fields["Query Directory Response"].(map[string]any)
	if !ok {
		return
	}
	raw, ok := response["Output Buffer"].([]byte)
	if !ok || len(raw) == 0 {
		return
	}
	decoded, err := stream_parser.DecodeSMB2DirectoryEntries(raw, 0, smb2StructuredMaxCommands)
	if err != nil {
		response["Directory Entry Status"] = "opaque: " + err.Error()
		response["Directory Decode Error"] = err.Error()
		return
	}
	entries, _ := decoded["Entries"].([]map[string]any)
	ranges, _ := response["Wire Field Ranges"].(map[string]any)
	span, _ := ranges["Output Buffer"].([]any)
	if len(span) == 2 {
		if base, ok := span[0].(uint64); ok {
			for _, entry := range entries {
				rebaseSMB2FieldRanges(entry, int(base))
			}
		}
	}
	response["Directory Entries"] = entries
	response["Directory Entry Status"] = "decoded: " + decoded["Status"].(string)
}

// rebaseSMB2FieldRanges translates the directory decoder's output-buffer
// relative spans to the SMB2 PDU. It deliberately skips the Wire Field Ranges
// key while walking children so a span is translated exactly once.
func rebaseSMB2FieldRanges(m map[string]any, base int) {
	if ranges, ok := m["Wire Field Ranges"].(map[string]any); ok {
		for key, value := range ranges {
			if span, ok := value.([]any); ok && len(span) == 2 {
				lo, loOK := span[0].(uint64)
				hi, hiOK := span[1].(uint64)
				if loOK && hiOK {
					ranges[key] = []any{lo + uint64(base), hi + uint64(base)}
				}
			}
		}
	}
	for key, value := range m {
		if key == "Wire Field Ranges" {
			continue
		}
		switch value := value.(type) {
		case map[string]any:
			rebaseSMB2FieldRanges(value, base)
		case []map[string]any:
			for _, child := range value {
				rebaseSMB2FieldRanges(child, base)
			}
		case []any:
			for _, item := range value {
				if child, ok := item.(map[string]any); ok {
					rebaseSMB2FieldRanges(child, base)
				}
			}
		}
	}
}
