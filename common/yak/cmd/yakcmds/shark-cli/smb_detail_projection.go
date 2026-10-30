package sharkcli

import (
	"encoding/binary"
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func smbDetailProtocol(protocol string) bool { return protocol == "smb" || protocol == "smb2" }

// Only known current-PDU owners participate in display deduplication. A match
// in an arbitrary context subtree, or equal bytes in another event, is not
// evidence that two fields describe the same wire occurrence.
func smbDisplayOwner(values map[string]any, raw []byte) bool {
	header, ok := values["Header"].(map[string]any)
	if !ok {
		return false
	}
	id, ok := header["Protocol ID"].(uint32)
	if !ok || id != 0x424d53ff && id != 0x424d53fe {
		return false
	}
	span := detailWireRanges(header["Wire Field Ranges"])["Protocol ID"]
	return len(span) == 2 && span[0] >= 4 && span[1]-span[0] == 4 && span[1] <= len(raw) && binary.LittleEndian.Uint32(raw[span[0]:span[1]]) == id
}

func smbDisplaySameOwner(a, b map[string]any, raw []byte) bool {
	if !smbDisplayOwner(a, raw) || !smbDisplayOwner(b, raw) {
		return false
	}
	left := a["Header"].(map[string]any)
	right := b["Header"].(map[string]any)
	return reflect.DeepEqual(left["Protocol ID"], right["Protocol ID"]) && reflect.DeepEqual(smbDisplayRange(left, "Protocol ID"), smbDisplayRange(right, "Protocol ID"))
}

func smbDisplayRange(owner map[string]any, key string) any {
	switch spans := owner["Wire Field Ranges"].(type) {
	case map[string]any:
		return spans[key]
	case map[string][]int:
		return spans[key]
	default:
		return nil
	}
}

func smbDisplayOwnedRange(owner map[string]any, key string, length int) bool {
	span := detailWireRanges(owner["Wire Field Ranges"])[key]
	return len(span) == 2 && span[0] >= 0 && span[1] > span[0] && span[1] <= length
}

func smbDisplayCompositeHasRange(value any, length, depth int) bool {
	if depth > 32 {
		return false
	}
	switch value := value.(type) {
	case map[string]any:
		for key := range value {
			if key != "Wire Field Ranges" && smbDisplayOwnedRange(value, key, length) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range value {
			if smbDisplayCompositeHasRange(child, length, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if smbDisplayCompositeHasRange(child, length, depth+1) {
				return true
			}
		}
	}
	return false
}

func smbDisplaySameEntry(a, b map[string]any, key string, raw []byte) bool {
	left, exists := a[key]
	right, present := b[key]
	if !exists || !present || !reflect.DeepEqual(left, right) {
		return false
	}
	leftRange, rightRange := smbDisplayRange(a, key), smbDisplayRange(b, key)
	if leftRange != nil || rightRange != nil {
		return smbDisplayOwnedRange(a, key, len(raw)) && smbDisplayOwnedRange(b, key, len(raw)) && reflect.DeepEqual(leftRange, rightRange)
	}
	switch key {
	case "Direct TCP", "Header", "Body", "Dialects", "Commands":
		// DeepEqual includes every nested value, Go type and range. The
		// containing owners have already established the same PDU position.
		return smbDisplayCompositeHasRange(left, len(raw), 0)
	}
	return false
}

func smbDisplayCopy(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func smbDisplayWithout(values map[string]any, omitted map[string]bool) map[string]any {
	if len(omitted) == 0 {
		return values
	}
	out := smbDisplayCopy(values)
	for key := range omitted {
		delete(out, key)
	}
	switch ranges := values["Wire Field Ranges"].(type) {
	case map[string]any:
		copy := smbDisplayCopy(ranges)
		for key := range omitted {
			delete(copy, key)
		}
		out["Wire Field Ranges"] = copy
	case map[string][]int:
		copy := make(map[string][]int, len(ranges))
		for key, span := range ranges {
			if !omitted[key] {
				copy[key] = span
			}
		}
		out["Wire Field Ranges"] = copy
	}
	return out
}

func smbDisplayCommands(value any) []map[string]any {
	switch commands := value.(type) {
	case []map[string]any:
		return commands
	case []any:
		out := make([]map[string]any, 0, len(commands))
		for _, value := range commands {
			command, ok := value.(map[string]any)
			if !ok {
				return nil
			}
			out = append(out, command)
		}
		return out
	}
	return nil
}

func smbDisplayPruneAliases(values map[string]any, raw []byte, depth int) map[string]any {
	if depth > 32 || !smbDisplayOwner(values, raw) {
		return values
	}
	omitted := map[string]bool{}
	commands := smbDisplayCommands(values["Commands"])
	if len(commands) > 0 && smbDisplaySameOwner(values, commands[0], raw) {
		first := commands[0]
		snapshot := smbDisplayCopy(values)
		for _, key := range []string{"Commands", "Compound", "Compound Count", "Command Names"} {
			delete(snapshot, key)
		}
		completeCopy := reflect.DeepEqual(snapshot, first)
		for key := range first {
			if key != "Wire Field Ranges" && (completeCopy || smbDisplaySameEntry(values, first, key, raw)) {
				omitted[key] = true
			}
		}
	}
	for _, name := range []string{"Header", "Body"} {
		owner, ok := values[name].(map[string]any)
		if !ok {
			continue
		}
		for key := range owner {
			if key != "Wire Field Ranges" && smbDisplaySameEntry(values, owner, key, raw) {
				omitted[key] = true
			}
		}
	}
	out := smbDisplayWithout(values, omitted)
	if len(commands) > 0 {
		out = smbDisplayCopy(out)
		switch original := values["Commands"].(type) {
		case []map[string]any:
			copy := make([]map[string]any, len(original))
			for i, command := range original {
				copy[i] = smbDisplayPruneAliases(command, raw, depth+1)
			}
			out["Commands"] = copy
		case []any:
			copy := make([]any, len(original))
			for i, command := range commands {
				copy[i] = smbDisplayPruneAliases(command, raw, depth+1)
			}
			out["Commands"] = copy
		}
	}
	return out
}

// Called only while the parent owns the original Go types. In particular a
// uint16/uint32 difference cannot disappear merely because IPC uses JSON numbers.
func smbDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	if e == nil || !smbDetailProtocol(e.Protocol) {
		return e
	}
	out := *e
	fields, session := e.Fields, e.Session
	if smbDisplaySameOwner(fields, session, e.Raw) {
		if reflect.DeepEqual(fields, session) {
			// Session also feeds the existing authentication presentation.
			// Keep that canonical copy so SESSION_SETUP retains its observed
			// SPNEGO/NTLMSSP name and authentication summary through IPC.
			fields = nil
		} else {
			omitted := map[string]bool{}
			for key := range session {
				if key != "Wire Field Ranges" && smbDisplaySameEntry(fields, session, key, e.Raw) {
					omitted[key] = true
				}
			}
			session = smbDisplayWithout(session, omitted)
		}
	}
	out.Fields = smbDisplayPruneAliases(fields, e.Raw, 0)
	out.Session = smbDisplayPruneAliases(session, e.Raw, 0)
	return &out
}

type smbDetailNode struct {
	name  string
	value any
}

var smbDetailBodyKeys = map[string]bool{
	"Word Count": true, "Byte Count": true, "Dialects": true, "Dialect": true,
	"File Name": true, "FileId": true, "Length": true, "Count": true,
	"Authentication": true, "Session Flags": true, "Guest Session": true, "Path": true,
}

// Layout is deliberately independent of deduplication. Workers can apply it
// after IPC without reconsidering equality of values whose types were folded.
func smbDetailSections(values map[string]any, depth int) []smbDetailNode {
	if len(values) == 0 || depth > 32 {
		return nil
	}
	var out []smbDetailNode
	for _, name := range []string{"Direct TCP", "Header"} {
		if value, ok := values[name]; ok {
			out = append(out, smbDetailNode{name, value})
		}
	}
	body, compatibility, context := map[string]any{}, map[string]any{}, map[string]any{}
	for key, value := range values {
		switch key {
		case "Wire Field Ranges", "Direct TCP", "Header", "Body", "Commands":
			continue
		}
		if smbDetailBodyKeys[key] {
			body[key] = value
		} else if smbDisplayRange(values, key) != nil {
			compatibility[key] = value
		} else {
			context[key] = value
		}
	}
	// Each synthetic owner receives only its own original range entries.
	for _, owner := range []map[string]any{body, compatibility} {
		ranges := map[string]any{}
		for key := range owner {
			if span := smbDisplayRange(values, key); span != nil {
				ranges[key] = span
			}
		}
		if len(ranges) != 0 {
			owner["Wire Field Ranges"] = ranges
		}
	}
	if original, ok := values["Body"]; ok {
		if len(body) > 0 {
			out = append(out, smbDetailNode{"Body", map[string]any{"Decoded body": original, "Compatibility fields": body}})
		} else {
			out = append(out, smbDetailNode{"Body", original})
		}
	} else if len(body) > 0 {
		out = append(out, smbDetailNode{"Body", body})
	}
	if len(compatibility) > 0 {
		out = append(out, smbDetailNode{"Compatibility fields", compatibility})
	}
	if value, present := values["Commands"]; present {
		commands := smbDisplayCommands(value)
		if len(commands) == 0 {
			out = append(out, smbDetailNode{"Commands", value})
		} else {
			views := make([]map[string]any, len(commands))
			for i, command := range commands {
				view := map[string]any{}
				for _, node := range smbDetailSections(command, depth+1) {
					view[node.name] = node.value
				}
				views[i] = view
			}
			out = append(out, smbDetailNode{"Commands", views})
		}
	}
	if len(context) > 0 {
		out = append(out, smbDetailNode{"Observed context", context})
	}
	return out
}

func smbDetailNodes(e *pcaputil.ProtocolEvent) []smbDetailNode {
	var nodes []smbDetailNode
	var origins []string
	occurrences := map[string]int{}
	for _, source := range []struct {
		name   string
		values map[string]any
	}{{"Fields", e.Fields}, {"Session", e.Session}} {
		for _, node := range smbDetailSections(source.values, 0) {
			nodes = append(nodes, node)
			origins = append(origins, source.name)
			occurrences[node.name]++
		}
	}
	for i := range nodes {
		// A single canonical tree has protocol names such as Commands[0],
		// not implementation names. Only retained, unproved copies need a
		// source qualifier so one observation cannot hide the other.
		if occurrences[nodes[i].name] > 1 {
			nodes[i].name += " · " + origins[i]
		}
	}
	if len(e.Raw) >= 8 && e.Raw[0] == 0 && int(e.Raw[1])<<16|int(e.Raw[2])<<8|int(e.Raw[3]) == len(e.Raw)-4 {
		magic := binary.LittleEndian.Uint32(e.Raw[4:8])
		present := occurrences["Direct TCP"] != 0
		if !present && (magic == 0x424d53fe || magic == 0x424d53ff || magic == 0x424d53fd) {
			direct := map[string]any{"Message Type": e.Raw[0], "Payload Length": uint32(len(e.Raw) - 4), "Wire Field Ranges": map[string]any{"Message Type": []any{uint64(0), uint64(1)}, "Payload Length": []any{uint64(1), uint64(4)}}}
			nodes = append([]smbDetailNode{{"Direct TCP", direct}}, nodes...)
		}
	}
	// All carrier occurrences lead. Retained/derived context follows every
	// command tree, including an unproved second Fields/Session observation.
	priority := func(name string) int {
		if strings.HasPrefix(name, "Direct TCP") {
			return 0
		}
		if strings.HasPrefix(name, "Observed context") {
			return 2
		}
		return 1
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return priority(nodes[i].name) < priority(nodes[j].name)
	})
	return nodes
}

func smbDetailFieldLess(a, b string, ranges map[string][]int) bool {
	priority := func(key string) int {
		switch key {
		case "Direct TCP":
			return 0
		case "Header":
			return 1
		case "Body", "Decoded body":
			return 2
		case "Commands":
			return 3
		case "Compatibility fields":
			return 4
		case "Observed context":
			return 1000
		}
		if len(ranges[key]) == 2 {
			return 10
		}
		return 100
	}
	left, right := priority(a), priority(b)
	if left != right {
		return left < right
	}
	if sa, sb := ranges[a], ranges[b]; len(sa) == 2 && len(sb) == 2 && sa[0] != sb[0] {
		return sa[0] < sb[0]
	}
	return a < b
}
