package stream_parser

import (
	"fmt"
	"strings"
)

// RFC 2342 namespace descriptors are adjacent, unlike ordinary SP-separated
// IMAP lists. Prefix/extension strings may be quoted or complete literals.
func (r *imapFieldsReader) namespaceGroup(name string) map[string]any {
	if r.err != nil {
		return nil
	}
	if len(r.wire)-r.at >= 3 && strings.EqualFold(string(r.wire[r.at:r.at+3]), "NIL") {
		r.leaf(name+" NIL", "string", r.at, r.at+3)
		r.at += 3
		return map[string]any{"Kind": "nil", "Entries": []map[string]any{}}
	}
	r.exact("Namespace List Open", "(")
	group := tlsCertificateField{Name: name, Start: r.at, List: true}
	entries := make([]map[string]any, 0)
	for r.err == nil && r.peek() != ')' {
		if r.items >= imapFieldsMaxItems {
			r.fail("namespace descriptor resource limit")
			break
		}
		r.items++
		start, fs := r.at, len(r.fields)
		r.exact("Namespace Descriptor Open", "(")
		prefix := r.stringValue("Namespace Prefix", "string")
		if prefix.value == nil {
			prefix.value = []byte{}
		}
		r.sp()
		delimiter := r.stringValue("Namespace Hierarchy Delimiter", "nstring")
		if delimiter.kind != "nil" && (delimiter.kind != "quoted" || len(delimiter.value) != 1) {
			r.fail("namespace delimiter must be one quoted character or NIL")
		}
		entry := map[string]any{"Prefix": prefix.value, "Prefix Encoding": prefix.kind, "Prefix Encoded Byte Ranges": prefix.ranges, "Hierarchy Delimiter": delimiter.value, "Hierarchy Delimiter Kind": delimiter.kind}
		extensions := make([]map[string]any, 0)
		for r.err == nil && r.peek() == ' ' {
			if r.items >= imapFieldsMaxItems {
				r.fail("namespace extension resource limit")
				break
			}
			r.items++
			r.sp()
			extName := r.stringValue("Namespace Extension Name", "string")
			if extName.value == nil {
				extName.value = []byte{}
			}
			r.sp()
			values := r.list("Namespace Extension Values", 1, func() map[string]any {
				v := r.stringValue("Namespace Extension Value", "string")
				if v.value == nil {
					v.value = []byte{}
				}
				return map[string]any{"Value": v.value, "Encoding": v.kind, "Encoded Byte Ranges": v.ranges}
			})
			extensions = append(extensions, map[string]any{"Name": extName.value, "Values": values, "Semantics Validated": false})
		}
		r.exact("Namespace Descriptor Close", ")")
		if r.err != nil {
			break
		}
		entry["Extensions"], entry["Relative Byte Range"] = extensions, [2]int{start, r.at}
		entries = append(entries, entry)
		group.Children = append(group.Children, tlsCertificateField{Name: "Namespace", Start: start, End: r.at, Children: append([]tlsCertificateField(nil), r.fields[fs:]...)})
		r.fields = r.fields[:fs]
	}
	group.End = r.at
	r.fields = append(r.fields, group)
	if len(entries) == 0 {
		r.fail("namespace requires a descriptor or NIL")
	}
	r.exact("Namespace List Close", ")")
	return map[string]any{"Kind": "namespace-list", "Entries": entries}
}

// DecodeIMAPMessageFields is the same explicit bounded grammar as imap_fields.
// This supplies wire fields/metadata, not connection or authentication proof.
func DecodeIMAPMessageFields(wire []byte, profile string) (map[string]any, error) {
	if profile != "command" && profile != "response" {
		return nil, fmt.Errorf("imap-fields: native profile must be command or response")
	}
	decode := structuredNativeDecoder(fmt.Sprintf("err = parseIMAPFields(%q)\nif err != nil { panic(err) }\n", profile))
	fields, info, err := decode(wire)
	if err != nil {
		return nil, err
	}
	info["Wire Fields"] = fields
	return info, nil
}
