package ssa

import (
	"fmt"
	"strings"
)

// typeStringer owns recursion state for one rendering. Formatting a shared type
// must never temporarily overwrite its name, even to break recursive graphs.
// Nested function renderings are not cached: their cycle markers depend on the
// current path. FunctionType.String caches only the complete root rendering.
type typeStringer struct{ active []Type }

func (s *typeStringer) format(t Type) string {
	for _, active := range s.active {
		if active == t {
			return "..."
		}
	}
	s.active = append(s.active, t)
	defer func() { s.active = s.active[:len(s.active)-1] }()
	switch t := t.(type) {
	case *ObjectType:
		if t.Combination {
			return joinTypesBounded(t.FieldTypes, ", ", s.format)
		}
		if t.Name != "" {
			return t.Name
		}
		return t.rawString(s.format)
	case *FunctionType:
		if t.Name != "" {
			return t.Name
		}
		return t.rawString(s.format)
	case *ChanType:
		return "chan " + s.format(t.Elem)
	case *OrType:
		parts := make([]string, len(t.types))
		for i, elem := range t.types {
			parts[i] = s.format(elem)
		}
		return strings.Join(parts, "|")
	case *AliasType:
		if t.Name != "" {
			return t.Name
		}
		return fmt.Sprintf("type %s (%s)", t.Name, s.format(t.elem))
	default:
		return t.String()
	}
}
