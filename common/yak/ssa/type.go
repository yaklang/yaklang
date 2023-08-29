package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

type Type interface {
	String() string
}
type Types []Type // each value can have multiple type possible

// return true  if org != typs
// return false if org == typs
func (org Types) Compare(typs Types) bool {
	if len(org) == 0 && len(typs) != 0 {
		return true
	}
	return slices.CompareFunc(org, typs, func(org, typ Type) int {
		if org.String() == typs.String() {
			return 0
		}
		return 1
	}) != 0
}

func (t Types) String() string {
	return strings.Join(
		lo.Map(t, func(typ Type, _ int) string {
			if typ == nil {
				return "nil"
			} else {
				return typ.String()
			}
		}),
		", ",
	)
}

// basic type
type TypeKind int

const (
	Number TypeKind = iota
	String
	Boolean
	UndefineType // undefine is nil in golnag
	Null         //
	Any          // any type
	ErrorType
)

type BasicType struct {
	Kind TypeKind
	name string
}

func (b BasicType) String() string {
	return b.name
}

var BasicTypesKind = []BasicType{
	Number:       {Number, "number"},
	String:       {String, "string"},
	Boolean:      {Boolean, "boolean"},
	UndefineType: {UndefineType, "undefine"},
	Null:         {Null, "null"},
	Any:          {Any, "any"},
	ErrorType:    {ErrorType, "error"},
}

func GetType(i any) Type {
	if typ := GetTypeByStr(reflect.TypeOf(i).String()); typ != nil {
		return typ
	} else {
		panic("undefine type")
	}
}
func GetTypeByStr(typ string) Type {
	switch typ {
	case "uint", "uint8", "byte", "uint16", "uint32", "uint64", "int", "int8", "int16", "int32", "int64":
		return BasicTypesKind[Number]
	case "float", "float32", "float64", "double":
		return BasicTypesKind[Number]
	case "string":
		return BasicTypesKind[String]
	case "bool":
		return BasicTypesKind[Boolean]
	default:
		return nil
	}
}

// ====================== chan type
type ChanType struct {
	elem Types
}

func NewChanType(elem Types) *ChanType {
	return &ChanType{
		elem: elem,
	}
}

func (c ChanType) String() string {
	return fmt.Sprintf("chan %s", c.elem)
}

// ==================== interface type
type InterfaceTypeKind int

const (
	Slice InterfaceTypeKind = iota
	Map
	Struct
)

type InterfaceType struct {
	Kind    InterfaceTypeKind
	Key     []Value
	keyType []Types
	Field   []Types
}

var _ (Type) = (*InterfaceType)(nil)

func NewInterfaceType() *InterfaceType {
	return &InterfaceType{
		Kind:    Struct,
		Key:     make([]Value, 0),
		keyType: make([]Types, 0),
		Field:   make([]Types, 0),
	}
}

// for slice build
func NewSliceType(elem Types) *InterfaceType {
	i := NewInterfaceType()
	i.Kind = Slice
	i.Field = append(i.Field, elem)
	return i
}

func NewMapType(key, field Types) *InterfaceType {
	i := NewInterfaceType()
	i.keyType = append(i.keyType, key)
	i.Field = append(i.Field, field)
	i.Kind = Map
	return i
}

func (itype InterfaceType) String() string {
	ret := ""
	switch itype.Kind {
	case Slice:
		// map[int]T
		if len(itype.Field) == 1 {
			ret += fmt.Sprintf("[]%s", itype.Field[0].String())
		} else {
			panic("this interface type not slice")
		}
	case Map:
		// map[T]U
		if len(itype.keyType) == 1 && len(itype.Field) == 1 {
			ret += fmt.Sprintf("map[%s]%s", itype.keyType[0].String(), itype.Field[0].String())
		} else {
			panic("this interface type not map")
		}
	case Struct:
		// map[string](T/U/xx)
		ret += fmt.Sprintf(
			"struct {%s}",
			strings.Join(
				lo.Map(itype.Field, func(field Types, _ int) string { return field.String() }),
				",",
			),
		)
	}
	return ret
}

// for struct build
func (s *InterfaceType) AddField(key Value, field Types) {
	s.Key = append(s.Key, key)
	s.keyType = append(s.keyType, key.GetType())
	s.Field = append(s.Field, field)
}

func (s *InterfaceType) GetField(key Value) Types {
	switch s.Kind {
	case Slice:
		return s.Field[0]
	case Map:
		return s.Field[0]
	case Struct:
		if index := slices.Index(s.Key, key); index != -1 {
			return s.Field[index]
		}
	}
	return nil
}

// ===================== Finish simply
func (s *InterfaceType) Finish() {
	field := lo.UniqBy(s.Field, func(t Types) string { return t.String() })
	keytype := lo.UniqBy(s.keyType, func(t Types) string { return t.String() })
	if len(field) == 1 {
		if len(keytype) == 1 {
			// map[T]U
			if keytype[0].String() == "number" {
				// map[number]T ==> []T slice
				s.Kind = Slice
			} else {
				// Map
				s.Kind = Map
			}
			s.keyType = keytype
			s.Field = field
		}
	}
}
