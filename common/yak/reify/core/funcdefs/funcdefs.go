// Package funcdefs stores C function signatures used to type calls and
// arguments during SSA conversion. One file per language keeps the tables
// explicit and easy to extend.
package funcdefs

// Kind is a C-level type category.
type Kind string

const (
	Void    Kind = "void"
	Int     Kind = "int"
	UInt    Kind = "uint"
	Size    Kind = "size"
	Pointer Kind = "pointer"
	String  Kind = "string"
	Struct  Kind = "struct"
	Float   Kind = "float"
)

// Type describes a C parameter or return type. Bits is the integer or float
// width. Name is used for struct and named pointer types.
type Type struct {
	Kind Kind
	Bits int
	Name string
}

// Param is one named C parameter.
type Param struct {
	Name string
	Type Type
}

// Definition is one C function signature.
type Definition struct {
	Name     string
	Params   []Param
	Return   Type
	Variadic bool
}

// IsString reports whether a value is a C string (char *).
func (t Type) IsString() bool { return t.Kind == String }

// IsVoid reports whether the return type is void.
func (t Type) IsVoid() bool { return t.Kind == Void }

// IsPointer reports whether t is a C pointer type.
func (t Type) IsPointer() bool { return t.Kind == Pointer }

// CType renders the type as C source.
func (t Type) CType() string {
	switch t.Kind {
	case Void:
		return "void"
	case Int:
		if t.Bits == 64 {
			return "int64_t"
		}
		return "int32_t"
	case UInt:
		if t.Bits == 64 {
			return "uint64_t"
		}
		return "uint32_t"
	case Size:
		return "size_t"
	case Pointer:
		return "void *"
	case String:
		return "char *"
	case Struct:
		if t.Name == "" {
			return "void *"
		}
		return "struct " + t.Name + " *"
	case Float:
		if t.Bits == 32 {
			return "float"
		}
		return "double"
	default:
		return "void *"
	}
}

// Lookup returns a definition by C symbol name.
func Lookup(name string) (Definition, bool) {
	definition, ok := cFunctions[name]
	return definition, ok
}

// All returns every registered C definition.
func All() []Definition {
	result := make([]Definition, 0, len(cFunctions))
	for _, definition := range cFunctions {
		result = append(result, definition)
	}
	return result
}
