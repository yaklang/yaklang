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
	RawString() string
	GetTypeKind() TypeKind
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

func (t Types) Equal(typs Types) bool {
	if len(t) != len(typs) {
		return false
	}
	return reflect.DeepEqual(t, typs)
}

func (t Types) Contains(typ Types) bool {
	if len(t) == 0 {
		return false
	}
	targetMap := lo.SliceToMap(typ, func(typ Type) (Type, struct{}) {
		return typ, struct{}{}
	})
	for _, tt := range t {
		if _, ok := targetMap[tt]; ok {
			return true
		}
	}
	return false
}

func (t Types) IsType(kind TypeKind) bool {
	for _, typ := range t {
		if typ == BasicTypes[kind] {
			return true
		}
	}
	return false
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
	InterfaceTypeKind
	FunctionTypeKind
)

type BasicType struct {
	Kind TypeKind
	name string
}

func (b BasicType) String() string {
	return b.name
}

func (b BasicType) RawString() string {
	return b.name
}

func (b BasicType) GetTypeKind() TypeKind {
	return b.Kind
}

var BasicTypes = []BasicType{
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
		return BasicTypes[Number]
	case "float", "float32", "float64", "double":
		return BasicTypes[Number]
	case "string":
		return BasicTypes[String]
	case "bool":
		return BasicTypes[Boolean]
	case "interface {}":
		return BasicTypes[Any]
	case "error":
		return BasicTypes[ErrorType]
	default:
		return nil
	}
}

// ====================== chan type
type ChanType struct {
	elem Type
}

func NewChanType(elem Type) *ChanType {
	return &ChanType{
		elem: elem,
	}
}

func (c ChanType) String() string {
	return fmt.Sprintf("chan %s", c.elem)
}

// ==================== interface type
type InterfaceKind int

const (
	None InterfaceKind = iota
	Slice
	Map
	Struct
)

type InterfaceType struct {
	Name    string
	Kind    InterfaceKind
	Len     int
	Key     []Value
	keyType []Type
	Field   []Type
}

func (i InterfaceType) GetTypeKind() TypeKind {
	return InterfaceTypeKind
}

var _ (Type) = (*InterfaceType)(nil)

func (i *InterfaceType) SetName(name string) {
	i.Name = name
}

func NewInterfaceType() *InterfaceType {
	return &InterfaceType{
		Kind:    None,
		Key:     make([]Value, 0),
		keyType: make([]Type, 0),
		Field:   make([]Type, 0),
	}
}

// for slice build
func NewSliceType(elem Type) *InterfaceType {
	i := NewInterfaceType()
	i.Kind = Slice
	i.Field = append(i.Field, elem)
	return i
}

func NewMapType(key, field Type) *InterfaceType {
	i := NewInterfaceType()
	i.keyType = append(i.keyType, key)
	i.Field = append(i.Field, field)
	i.Kind = Map
	return i
}

func (itype InterfaceType) String() string {
	if itype.Name != "" {
		return itype.Name
	}
	return itype.RawString()
}

func (itype InterfaceType) RawString() string {
	ret := ""
	switch itype.Kind {
	case Slice:
		// map[int]T
		if itype.Len == 0 {
			ret += fmt.Sprintf("[]%s", itype.Field[0].String())
		} else {
			ret += fmt.Sprintf("[%d]%s", itype.Len, itype.Field[0].String())
		}
	case Map:
		// map[T]U
		// if len(itype.keyType) == 1 && len(itype.Field) == 1 {
		ret += fmt.Sprintf("map[%s]%s", itype.keyType[0].String(), itype.Field[0].String())
		// } else {
		// 	panic("this interface type not map")
		// }
	case Struct:
		// map[string](T/U/xx)
		ret += fmt.Sprintf(
			"struct {%s}",
			strings.Join(
				lo.Map(itype.Field, func(field Type, _ int) string { return field.String() }),
				",",
			),
		)
	case None:
		ret += "interface{}"
	}
	return ret
}

// for struct build
func (s *InterfaceType) AddField(key Value, field Type) {
	s.Key = append(s.Key, key)
	keytyp := key.GetType()
	if keytyp == nil {
		keytyp = BasicTypes[Any]
	}
	s.keyType = append(s.keyType, keytyp)
	if field == nil {
		field = BasicTypes[Any]
	}
	s.Field = append(s.Field, field)
}

// return (field-type, key-type)
func (s *InterfaceType) GetField(key Value) (Type, Type) {
	switch s.Kind {
	case Slice:
		return s.Field[0], BasicTypes[Number]
	case Map:
		return s.Field[0], s.keyType[0]
	case Struct:
		if index := slices.Index(s.Key, key); index != -1 {
			return s.Field[index], key.GetType()
		}
	}
	return nil, nil
}

// ===================== Finish simply
func (s *InterfaceType) Finish() {
	field := lo.UniqBy(s.Field, func(t Type) string { return t.String() })
	keytype := lo.UniqBy(s.keyType, func(t Type) string { return t.String() })
	if len(keytype) == 1 {
		if len(field) == 1 {
			// map[T]U
			if keytype[0].String() == "number" {
				// map[number]T ==> []T slice
				// TODO: check increasing
				s.Kind = Slice
			} else {
				// Map
				s.Kind = Map
			}
			// s.keyType = keytype
			// s.Field = field
		} else {
			s.Kind = Struct
		}
	}
}

type FunctionType struct {
	Name       string
	ReturnType Type
	Parameter  []Type
	IsVariadic bool
}

func CalculateType(ts []Type) Type {
	if len(ts) == 0 {
		return BasicTypes[Null]
	} else if len(ts) == 1 {
		return ts[0]
	} else {
		i := NewInterfaceType()
		for index, typ := range ts {
			i.AddField(NewConst(index), typ)
		}
		i.Finish()
		// i.SetLen(NewConst(len(ts)))
		i.Len = len(ts)
		return i
	}
}

func NewFunctionType(name string, Parameter []Type, ReturnType []Type, IsVariadic bool) *FunctionType {
	f := &FunctionType{
		Name:       name,
		Parameter:  Parameter,
		IsVariadic: IsVariadic,
	}
	f.ReturnType = CalculateType(ReturnType)
	return f
}

func (s *FunctionType) SetName(name string) {
	s.Name = name
}

func (s *FunctionType) String() string {
	if s.Name != "" {
		return s.Name
	}
	return s.RawString()
}

func (s *FunctionType) RawString() string {
	str := ""
	if s.IsVariadic {
		str += "..."
	}

	return fmt.Sprintf(
		"(%s %s) -> %s",
		strings.Join(
			lo.Map(s.Parameter, func(t Type, _ int) string { return t.String() }),
			",",
		),
		str,
		s.ReturnType,
	)
}

func (s *FunctionType) GetTypeKind() TypeKind {
	return FunctionTypeKind
}
