package pcaputil

import (
	"reflect"
)

// Variable descriptions and addresses are literal selectors/declarations. They
// do not establish a device namespace, memory layout or application execution.
func (c *mmsValueContext) variableEntries(list mmsBER, d *mmsDecoded) ([]map[string]any, error) {
	if list.tag != 0xa0 {
		return nil, protocolError(ErrUnsupportedFeature, "MMS selected listOfVariable")
	}
	entries := make([]map[string]any, 0, len(list.children))
	for _, row := range list.children {
		if row.tag != 0x30 || len(row.children) == 0 {
			return nil, protocolError(ErrMalformedMessage, "MMS variable entry SEQUENCE")
		}
		if len(row.children) != 1 {
			if len(row.children) == 2 && row.children[1].tag == 0xa5 {
				return nil, protocolError(ErrUnsupportedFeature, "MMS alternate access requires selected context")
			}
			return nil, protocolError(ErrMalformedMessage, "MMS variable entry field cardinality")
		}
		v := row.children[0]
		var fields map[string]any
		switch v.tag {
		case 0xa0:
			name, err := mmsSingleChild(v)
			if err != nil {
				return nil, err
			}
			fields, err = mmsNamed(name, true)
			if err != nil {
				return nil, err
			}
		case 0xa1:
			value, err := mmsSingleChild(v)
			if err != nil {
				return nil, err
			}
			address, err := mmsAddress(value)
			if err != nil {
				return nil, err
			}
			fields = map[string]any{"Variable Choice Tag": v.tag, "Address": address}
		case 0xa2:
			// Address and TypeSpecification are positional CHOICE values; their
			// context tags can coincide and must not be indexed as SEQUENCE keys.
			if len(v.children) != 2 {
				return nil, protocolError(ErrMalformedMessage, "MMS variableDescription address/type pair")
			}
			address, err := mmsAddress(v.children[0])
			if err != nil {
				return nil, err
			}
			typed, depth, err := c.typeSpec(v.children[1])
			if err != nil {
				return nil, err
			}
			d.valueDepth = max(d.valueDepth, depth)
			fields = map[string]any{"Variable Choice Tag": v.tag, "Address": address, "Type Specification": typed}
		case 0x84:
			if len(v.body) != 0 {
				return nil, protocolError(ErrMalformedMessage, "MMS invalidated VariableSpecification NULL has content")
			}
			fields = map[string]any{"Variable Choice Tag": v.tag, "Invalidated": true}
		default:
			return nil, protocolError(ErrUnsupportedFeature, "MMS VariableSpecification choice")
		}
		entries = append(entries, fields)
	}
	if len(entries) == 0 {
		return nil, protocolError(ErrMalformedMessage, "MMS empty selected listOfVariable")
	}
	return entries, nil
}

// Comparison ignores permitted string segmentation, not selector identity or
// declaration content. Public projections retain the original encoding markers.
func mmsNormalizedSelector(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			out[k] = mmsNormalizedSelector(child)
		}
		if _, ok := x["Identifier"]; ok {
			if tag, ok := x["Choice Tag"].(byte); ok && (tag == 0xa0 || tag == 0xa2) {
				out["Choice Tag"] = tag &^ byte(32)
			}
			delete(out, "Name Choice Tag")
		}
		if _, ok := x["Domain ID"]; ok {
			delete(out, "Component Tags")
		}
		_, symbolic := x["Symbolic"]
		_, opaque := x["Bytes"]
		if symbolic || opaque {
			if tag, ok := x["Choice Tag"].(byte); ok {
				out["Choice Tag"] = tag &^ byte(32)
			}
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, child := range x {
			out[i] = mmsNormalizedSelector(child)
		}
		return out
	case []byte:
		if len(x) == 0 {
			return []byte{}
		}
		return x // Private accepted/request-decoded bytes are never mutated here.
	default:
		return v
	}
}

func mmsEntriesEqual(a, b map[string]any) bool {
	_, extendedA := a["Variable Choice Tag"]
	_, extendedB := b["Variable Choice Tag"]
	if !extendedA && !extendedB {
		return mmsKey(a) == mmsKey(b)
	}
	return extendedA && extendedB && reflect.DeepEqual(mmsNormalizedSelector(a), mmsNormalizedSelector(b))
}

// These are conservative logical context charges, not heap measurements.
func mmsSelectorStorage(v any) int64 {
	switch x := v.(type) {
	case map[string]any:
		n := int64(256)
		for k, child := range x {
			n += int64(len(k)) + mmsSelectorStorage(child)
		}
		return n
	case []map[string]any:
		n := int64(32 + 16*len(x))
		for _, child := range x {
			n += mmsSelectorStorage(child)
		}
		return n
	case []byte:
		return int64(32 + len(x))
	case string:
		return int64(32 + len(x))
	default:
		return 16
	}
}

func mmsSelectorUnits(v any) int {
	switch x := v.(type) {
	case map[string]any:
		n := 1 + len(x)
		for _, child := range x {
			n += mmsSelectorUnits(child)
		}
		return n
	case []map[string]any:
		n := len(x)
		for _, child := range x {
			n += mmsSelectorUnits(child)
		}
		return n
	case []byte:
		// Address payloads are opaque octets and charge byte budgets. The
		// existing ObjectName component-tag vector charges each tag separately.
		return 0
	default:
		return 0
	}
}

func mmsEntryHistoryUnits(entries []map[string]any) int {
	n := len(entries)
	for _, entry := range entries {
		if _, extended := entry["Variable Choice Tag"]; extended {
			n += mmsSelectorUnits(entry) - 1
		}
	}
	return n
}
