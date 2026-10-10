package go2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type goFieldSelector struct {
	path      []string
	ambiguous bool
	unknown   bool
}

func goPointerType(target ssa.Type) *ssa.ObjectType {
	typ := ssa.NewPointerType()
	typ.SetName("Pointer")
	typ.FieldType = target
	return typ
}

func goSelectorObjectType(typ ssa.Type) *ssa.ObjectType {
	for {
		alias, ok := typ.(*ssa.AliasType)
		if !ok {
			object, _ := ssa.ToObjectType(typ)
			return object
		}
		typ = alias.GetType()
	}
}

// Resolve the shallowest unique field and retain its embedding path. A type
// alone cannot tell the frontend which object to read or update. Count duplicate
// paths even when they reach the same type (diamonds), and visit each depth in
// full before dropping recursive types.
func resolveGoFieldSelector(root *ssa.ObjectType, name string) goFieldSelector {
	type entry struct {
		path  []string
		count int
	}
	current := map[*ssa.ObjectType]entry{root: {count: 1}}
	visited := make(map[*ssa.ObjectType]bool)
	for len(current) != 0 {
		var found goFieldSelector
		matches := 0
		unknown := false
		next := make(map[*ssa.ObjectType]entry)
		for typ, item := range current {
			visited[typ] = true
			// An imported embedded type without source has an unknown field
			// set. Keep the external-member fallback instead of declaring its
			// promoted fields missing or choosing a deeper known field.
			if _, ok := typ.FieldType.(*ssa.Blueprint); ok && typ.GetTypeKind() == ssa.ObjectTypeKind {
				unknown = true
				continue
			}
			if key := typ.GetKeybyName(name); !utils.IsNil(key) {
				matches += item.count
				found.path = append(append([]string(nil), item.path...), name)
			}
			for embeddedName, embedded := range typ.AnonymousField {
				if embedded == nil {
					continue
				}
				child := next[embedded]
				if child.count == 0 {
					child.path = append(append([]string(nil), item.path...), embeddedName)
				}
				child.count = min(2, child.count+item.count)
				next[embedded] = child
			}
		}
		if matches > 1 {
			return goFieldSelector{ambiguous: true}
		}
		if unknown {
			return goFieldSelector{unknown: true}
		}
		if matches == 1 {
			return found
		}
		for typ := range next {
			if visited[typ] {
				delete(next, typ)
			}
		}
		current = next
	}
	return goFieldSelector{}
}

// Always dereference the original target, not a copy of a pointer's @value
// snapshot, so a promoted write and an explicit write share object identity.
func (b *astbuilder) dereferenceGoSelector(value ssa.Value) ssa.Value {
	seen := make(map[ssa.Value]bool)
	for !utils.IsNil(value) {
		typ := goSelectorObjectType(value.GetType())
		// Slice element types may describe the pointee while the member value
		// still carries the pointer representation. Its reserved @pointer
		// member preserves that identity independently of the declared type.
		pointer := b.ReadMemberCallValueByName(value, "@pointer")
		if typ == nil || (typ.GetTypeKind() != ssa.PointerKind && utils.IsNil(pointer)) || seen[value] {
			break
		}
		seen[value] = true
		value = b.GetOriginValue(value)
		if !utils.IsNil(value) && typ.GetTypeKind() == ssa.PointerKind && typ.FieldType != nil && value.GetType().GetTypeKind() == ssa.AnyTypeKind {
			value.SetType(typ.FieldType)
		}
	}
	return value
}

// handled distinguishes a known struct selector from a package, method, or an
// unknown external type. Invalid struct selectors never fall back to creating a
// new field on the outer object.
func (b *astbuilder) resolveGoSelector(object ssa.Value, name string, wantMethod bool) (owner ssa.Value, key ssa.Value, handled, valid bool) {
	object = b.dereferenceGoSelector(object)
	if utils.IsNil(object) {
		return nil, nil, false, false
	}
	typ := goSelectorObjectType(object.GetType())
	if typ == nil || typ.GetTypeKind() != ssa.StructTypeKind {
		return object, nil, false, false
	}
	selector := resolveGoFieldSelector(typ, name)
	if selector.ambiguous {
		objectName := object.GetVerboseName()
		if objectName == "" {
			objectName = typ.VerboseName
		}
		if objectName == "" {
			objectName = typ.String()
		}
		b.NewError(ssa.Error, TAG, fmt.Sprintf("ambiguous selector %s.%s", objectName, name))
		return object, nil, true, false
	}
	if selector.unknown {
		return object, nil, false, false
	}
	if len(selector.path) == 0 {
		if wantMethod || typ.GetMethod()[name] != nil {
			return object, nil, false, false
		}
		b.NewError(ssa.Error, TAG, fmt.Sprintf("type %s has no field %s", typ.String(), name))
		return object, nil, true, false
	}
	for _, field := range selector.path[:len(selector.path)-1] {
		object = b.ReadMemberCallValue(object, b.EmitConstInstPlaceholder(field))
		object = b.dereferenceGoSelector(object)
	}
	return object, b.EmitConstInstPlaceholder(name), true, true
}
