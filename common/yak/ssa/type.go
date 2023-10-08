package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type Type interface {
	String() string
	RawString() string
	GetTypeKind() TypeKind

	// set/get method
	SetMethod(map[string]*FunctionType)
	GetMethod(id string) *FunctionType
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
		if typ.GetTypeKind() == kind {
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
	ChanTypeKind
	ErrorType
	ObjectTypeKind
	InterfaceTypeKind
	FunctionTypeKind
)

type BasicType struct {
	Kind TypeKind
	name string

	method map[string]*FunctionType
}

func (b *BasicType) String() string {
	return b.name
}

func (b *BasicType) RawString() string {
	return b.name
}

func (b *BasicType) GetTypeKind() TypeKind {
	return b.Kind
}
func (b *BasicType) GetMethod(id string) *FunctionType {
	if v, ok := b.method[id]; ok {
		return v
	} else {
		return nil
	}
}
func (b *BasicType) SetMethod(method map[string]*FunctionType) {
	b.method = method
}

var _ Type = (*BasicType)(nil)

var BasicTypes = []*BasicType{
	Number:       {Number, "number", make(map[string]*FunctionType, 0)},
	String:       {String, "string", make(map[string]*FunctionType, 0)},
	Boolean:      {Boolean, "boolean", make(map[string]*FunctionType, 0)},
	UndefineType: {UndefineType, "undefine", make(map[string]*FunctionType, 0)},
	Null:         {Null, "null", make(map[string]*FunctionType, 0)},
	Any:          {Any, "any", make(map[string]*FunctionType, 0)},
	ErrorType:    {ErrorType, "error", make(map[string]*FunctionType, 0)},
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
	case "uint", "uint8", "byte", "uint16", "uint32", "uint64", "int", "int8", "int16", "int32", "int64", "uintptr":
		return BasicTypes[Number]
	case "float", "float32", "float64", "double", "complex128", "complex64":
		return BasicTypes[Number]
	case "string":
		return BasicTypes[String]
	case "bool":
		return BasicTypes[Boolean]
	case "interface {}", "var":
		return BasicTypes[Any]
	case "error":
		return BasicTypes[ErrorType]
	default:
		return nil
	}
}

// ====================== alias type
type AliasType struct {
	elem   Type
	method map[string]*FunctionType
	Name   string
}

var _ Type = (*AliasType)(nil)

func NewAliasType(name string, elem Type) *AliasType {
	return &AliasType{
		elem:   elem,
		method: make(map[string]*FunctionType),
		Name:   name,
	}
}

func (a *AliasType) SetMethod(m map[string]*FunctionType) {
	a.method = m
}

func (a *AliasType) GetMethod(id string) *FunctionType {
	if v, ok := a.method[id]; ok {
		return v
	} else {
		return nil
	}
}

func (a *AliasType) String() string {
	if a.Name != "" {
		return a.Name
	} else {
		return a.RawString()
	}
}

func (a *AliasType) RawString() string {
	return fmt.Sprintf("type %s (%s)", a.Name, a.elem)
}

func (a *AliasType) GetTypeKind() TypeKind {
	return a.elem.GetTypeKind()
}

// ====================== interface type
type InterfaceType struct {
	method map[string]*FunctionType
	name   string
}

func NewInterfaceType(name string) *InterfaceType {
	return &InterfaceType{
		method: make(map[string]*FunctionType),
		name:   name,
	}
}

var _ Type = (*InterfaceType)(nil)

func (i *InterfaceType) SetMethod(m map[string]*FunctionType) {
	i.method = m
}

func (i *InterfaceType) GetMethod(id string) *FunctionType {
	if v, ok := i.method[id]; ok {
		return v
	} else {
		return nil
	}
}

func (i *InterfaceType) GetTypeKind() TypeKind {
	return InterfaceTypeKind
}

func (i *InterfaceType) String() string {
	if i.name != "" {
		return i.name
	} else {
		return i.RawString()
	}
}

func (i *InterfaceType) RawString() string {
	return fmt.Sprintf("type %s interface{}", i.name)
}

// ====================== chan type
type ChanType struct {
	elem   Type
	method map[string]*FunctionType
}

var _ (Type) = (*ChanType)(nil)

func (c *ChanType) SetMethod(m map[string]*FunctionType) {
	c.method = m
}
func (c *ChanType) GetMethod(id string) *FunctionType {
	return c.method[id]
}

func (c *ChanType) GetTypeKind() TypeKind {
	return ChanTypeKind
}

func NewChanType(elem Type) *ChanType {
	return &ChanType{
		elem: elem,
	}
}

func (c ChanType) String() string {
	return fmt.Sprintf("chan %s", c.elem)
}

func (c ChanType) RawString() string {
	return c.String()
}

// ==================== interface type
type ObjectKind int

const (
	None ObjectKind = iota
	Slice
	Map
	Struct
)

type ObjectType struct {
	Name       string
	Kind       ObjectKind
	Len        int
	Key        []Value
	keyTypes   []Type
	FieldTypes []Type

	Combination bool

	method map[string]*FunctionType

	KeyTyp    Type
	FieldType Type
}

func (i *ObjectType) GetTypeKind() TypeKind {
	return ObjectTypeKind
}

func (i *ObjectType) GetMethod(id string) *FunctionType {
	if v, ok := i.method[id]; ok {
		return v
	} else {
		return nil
	}
}

func (i *ObjectType) SetMethod(m map[string]*FunctionType) {
	i.method = m
}

var _ (Type) = (*ObjectType)(nil)

func (i *ObjectType) SetName(name string) {
	i.Name = name
}

func NewObjectType() *ObjectType {
	return &ObjectType{
		Kind:       None,
		Key:        make([]Value, 0),
		keyTypes:   make([]Type, 0),
		FieldTypes: make([]Type, 0),
		method:     make(map[string]*FunctionType, 0),
	}
}

// for slice build
func NewSliceType(elem Type) *ObjectType {
	i := NewObjectType()
	i.Kind = Slice
	i.KeyTyp = BasicTypes[Number]
	i.FieldType = elem
	return i
}

func NewMapType(key, field Type) *ObjectType {
	i := NewObjectType()
	i.KeyTyp = key
	i.FieldType = field
	i.Kind = Map
	return i
}

func NewStructType() *ObjectType {
	i := NewObjectType()
	i.Kind = Struct
	return i
}

func (itype ObjectType) String() string {
	if itype.Name != "" {
		return itype.Name
	}
	return itype.RawString()
}

func (itype ObjectType) RawString() string {
	ret := ""
	if itype.Combination {
		// ret += itype.fieldType.String()
		// for index := range itype.FieldTypes {
		// 	ret += fmt.Sprintf(", %s")
		// }
		ret += strings.Join(
			lo.Map(
				itype.FieldTypes,
				func(t Type, _ int) string { return t.String() },
			),
			", ",
		)
	} else {
		switch itype.Kind {
		case Slice:
			// map[int]T
			if itype.Len == 0 {
				ret += fmt.Sprintf("[]%s", itype.FieldType.String())
			} else {
				ret += fmt.Sprintf("[%d]%s", itype.Len, itype.FieldType.String())
			}
		case Map:
			// map[T]U
			// if len(itype.keyType) == 1 && len(itype.Field) == 1 {
			keyTyp := itype.KeyTyp
			if utils.IsNil(keyTyp) {
				keyTyp = BasicTypes[Any]
			}
			fieldType := itype.FieldType
			if utils.IsNil(fieldType) {
				fieldType = BasicTypes[Any]
			}
			ret += fmt.Sprintf("map[%s]%s", keyTyp.String(), fieldType.String())
			// } else {
			// 	panic("this interface type not map")
			// }
		case Struct:
			// map[string](T/U/xx)
			ret += fmt.Sprintf(
				"struct {%s}",
				strings.Join(
					lo.Map(itype.FieldTypes, func(field Type, _ int) string { return field.String() }),
					",",
				),
			)
		case None:
			ret += "object{}"
		}
	}
	return ret
}

// for struct build
func (s *ObjectType) AddField(key Value, field Type) {
	s.Key = append(s.Key, key)
	keytyp := key.GetType()
	s.keyTypes = append(s.keyTypes, keytyp)
	if field == nil {
		field = BasicTypes[Any]
	}
	s.FieldTypes = append(s.FieldTypes, field)
}

// return (field-type, key-type)
func (s *ObjectType) GetField(key Value) (Type, Type) {
	switch s.Kind {
	case Slice, Map:
		return s.FieldType, s.KeyTyp
	case Struct:
		if index := slices.Index(s.Key, key); index != -1 {
			return s.FieldTypes[index], key.GetType()
		}
	}
	return nil, nil
}

// ===================== Finish simply
func (s *ObjectType) Finish() {
	if s.Kind != None {
		return
	}
	fieldTypes := lo.UniqBy(s.FieldTypes, func(t Type) TypeKind { return t.GetTypeKind() })
	keytypes := lo.UniqBy(s.keyTypes, func(t Type) TypeKind { return t.GetTypeKind() })
	if len(keytypes) == 1 {
		if len(fieldTypes) == 1 {
			// map[T]U
			if keytypes[0].GetTypeKind() == Number {
				// map[number]T ==> []T slice
				// TODO: check increasing
				s.Kind = Slice
				s.KeyTyp = BasicTypes[Number]
				s.FieldType = fieldTypes[0]
			} else {
				// Map
				s.Kind = Map
				s.KeyTyp = keytypes[0]
				s.FieldType = fieldTypes[0]
			}
			// s.keyType = keytype
			// s.Field = field
		} else if keytypes[0].GetTypeKind() == String || keytypes[0].GetTypeKind() == Number {
			s.Kind = Struct
			s.KeyTyp = BasicTypes[String]
			s.FieldType = BasicTypes[Any]
		}
	}
}

type FunctionType struct {
	Name       string
	ReturnType Type
	Parameter  []Type
	IsVariadic bool
}

var _ Type = (*FunctionType)(nil)

func (f *FunctionType) GetMethod(string) *FunctionType {
	return nil
}

func (f *FunctionType) SetMethod(m map[string]*FunctionType) {
}

func CalculateType(ts []Type) Type {
	if len(ts) == 0 {
		return BasicTypes[Null]
	} else if len(ts) == 1 {
		return ts[0]
	} else {
		i := NewObjectType()
		for index, typ := range ts {
			i.AddField(NewConst(index), typ)
		}
		i.Finish()
		i.Combination = true
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
