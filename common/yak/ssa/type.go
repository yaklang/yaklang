package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func init() {
	BasicTypes[ErrorType].method["Error"] = NewFunctionTypeDefine(
		"error.Error",
		[]Type{BasicTypes[ErrorType]},
		[]Type{BasicTypes[StringTypeKind]},
		false,
	)
}

const MAXTypeCompareDepth = 10

func TypeCompare(t1, t2 Type) bool {
	return typeCompareEx(t1, t2, 0) || typeCompareEx(t2, t1, 0)
}

func typeCompareEx(t1, t2 Type, depth int) bool {
	t1kind := t1.GetTypeKind()
	t2kind := t2.GetTypeKind()

	if t1kind == AnyTypeKind || t2kind == AnyTypeKind {
		return true
	}

	// TODO: check InterfaceType, compare method function
	if t1kind == InterfaceTypeKind || t2kind == InterfaceTypeKind {
		return true
	}

	if depth == MAXTYPELEVEL {
		return true
	}
	depth += 1

	switch t1kind {
	case FunctionTypeKind:
		t2f, ok := ToFunctionType(t2)
		if !ok {
			break
		}
		t1f, _ := ToFunctionType(t1)
		if len(t1f.Parameter) != len(t2f.Parameter) {
			return false
		}
		for i := 0; i < len(t1f.Parameter); i++ {
			if !typeCompareEx(t1f.Parameter[i], t2f.Parameter[i], depth) {
				return false
			}
		}
		if !typeCompareEx(t1f.ReturnType, t2f.ReturnType, depth) {
			return false
		}
		return true
	case SliceTypeKind:
		t2o, ok := ToObjectType(t2)
		if !ok {
			break
		}
		t1o, _ := ToObjectType(t1)
		return typeCompareEx(t1o.FieldType, t2o.FieldType, depth)
	case MapTypeKind:
		t2o, ok := t2.(*ObjectType)
		if !ok {
			break
		}
		t1o := t1.(*ObjectType)
		return typeCompareEx(t1o.FieldType, t2o.FieldType, depth) && typeCompareEx(t1o.KeyTyp, t2o.KeyTyp, depth)
	case StructTypeKind:
	case ObjectTypeKind:
	case BytesTypeKind:
		// string | []number
		if t2kind == StringTypeKind {
			return true
		}
	case StringTypeKind:
		if t2kind == BytesTypeKind {
			return true
		}
	case NullTypeKind:
		if t2kind == NumberTypeKind || t2kind == BooleanTypeKind || t2kind == StringTypeKind {
			return false
		} else {
			return true
		}
	default:
	}
	return t1kind == t2kind
}

type MethodBuilder interface {
	Build(Type, string) *FunctionType
	GetMethodNames(Type) []string
}

var ExternMethodBuilder MethodBuilder

func GetMethod(t Type, id string) *FunctionType {
	var f *FunctionType
	if fun, ok := t.GetMethod()[id]; ok {
		f = fun
	}

	if f == nil && ExternMethodBuilder != nil {
		f = ExternMethodBuilder.Build(t, id)
		if f != nil {
			t.AddMethod(id, f)
		}
	}
	if f != nil {
		f.IsMethod = true
	}
	return f
}

func GetMethodsName(t Type) []string {
	ret := make([]string, 0)
	ret = append(ret, lo.Keys(t.GetMethod())...)
	if ExternMethodBuilder != nil {
		ret = append(ret, ExternMethodBuilder.GetMethodNames(t)...)
	}
	return ret
}

func GetAllKey(t Type) []string {
	ret := make([]string, 0)
	switch t.GetTypeKind() {
	case FunctionTypeKind:
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, StructTypeKind:
		ot, _ := ToObjectType(t)
		ret = append(ret, lo.Map(ot.Keys, func(v Value, _ int) string { return v.String() })...)
		fallthrough
	default:
		ret = append(ret, GetMethodsName(t)...)
	}
	return ret
}

// func (b *ObjectType) GetAllKey() []string {
// 	return append(lo.Keys(b.method), lo.Map(b.Key, func(v Value, _ int) string { return v.String() })...)
// }

func IsObjectType(t Type) bool {
	switch t.GetTypeKind() {
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, StructTypeKind:
		return true
	default:
		return false
	}
}

type Type interface {
	String() string        // only string
	PkgPathString() string // package path string
	RawString() string
	GetTypeKind() TypeKind

	// set/get method
	SetMethod(map[string]*FunctionType)
	AddMethod(string, *FunctionType)
	GetMethod() map[string]*FunctionType
	// GetAllKey() []string
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
	NumberTypeKind TypeKind = iota
	StringTypeKind
	BytesTypeKind
	BooleanTypeKind
	UndefinedTypeKind // undefined is nil in golang
	NullTypeKind      //
	AnyTypeKind       // any type
	ChanTypeKind
	ErrorType

	ObjectTypeKind
	SliceTypeKind
	MapTypeKind
	StructTypeKind

	InterfaceTypeKind
	FunctionTypeKind
)

type BasicType struct {
	Kind    TypeKind
	name    string
	pkgPath string

	method map[string]*FunctionType
}

func NewBasicType(kind TypeKind, name string) *BasicType {
	return &BasicType{
		Kind:    kind,
		name:    name,
		pkgPath: name,
		method:  map[string]*FunctionType{},
	}
}

func (b *BasicType) String() string {
	return b.name
}

func (b *BasicType) PkgPathString() string {
	result := b.pkgPath
	if result == "" {
		result = b.RawString()
	}
	return result
}

func (b *BasicType) RawString() string {
	return b.name
}

func (b *BasicType) GetTypeKind() TypeKind {
	return b.Kind
}

func (b *BasicType) GetMethod() map[string]*FunctionType {
	return b.method
}

func (b *BasicType) SetMethod(method map[string]*FunctionType) {
	b.method = method
}

func (b *BasicType) AddMethod(id string, f *FunctionType) {
	if b.method == nil {
		b.method = make(map[string]*FunctionType)
	}
	b.method[id] = f
}

// func (b *BasicType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

var _ Type = (*BasicType)(nil)

var BasicTypes = map[TypeKind]*BasicType{
	NumberTypeKind:    NewBasicType(NumberTypeKind, "number"),
	StringTypeKind:    NewBasicType(StringTypeKind, "string"),
	BytesTypeKind:     NewBasicType(BytesTypeKind, "bytes"),
	BooleanTypeKind:   NewBasicType(BooleanTypeKind, "boolean"),
	UndefinedTypeKind: NewBasicType(UndefinedTypeKind, "undefined"),
	NullTypeKind:      NewBasicType(NullTypeKind, "null"),
	AnyTypeKind:       NewBasicType(AnyTypeKind, "any"),
	ErrorType:         NewBasicType(ErrorType, "error"),
}

func GetNumberType() Type {
	return BasicTypes[NumberTypeKind]
}

func GetStringType() Type {
	return BasicTypes[StringTypeKind]
}

func GetBytesType() Type {
	return BasicTypes[BytesTypeKind]
}

func GetBooleanType() Type {
	return BasicTypes[BooleanTypeKind]
}

func GetUndefinedType() Type {
	return BasicTypes[UndefinedTypeKind]
}

func GetNullType() Type {
	return BasicTypes[NullTypeKind]
}

func GetAnyType() Type {
	return BasicTypes[AnyTypeKind]
}

func GetErrorType() Type {
	return BasicTypes[ErrorType]
}

func GetType(i any) Type {
	if typ := GetTypeByStr(reflect.TypeOf(i).String()); typ != nil {
		return typ
	} else {
		panic("undefined type")
	}
}

func GetTypeByStr(typ string) Type {
	switch typ {
	case "uint", "uint8", "byte", "uint16", "uint32", "uint64", "int", "int8", "int16", "int32", "int64", "uintptr":
		return BasicTypes[NumberTypeKind]
	case "float", "float32", "float64", "double", "complex128", "complex64":
		return BasicTypes[NumberTypeKind]
	case "string":
		return BasicTypes[StringTypeKind]
	case "bool":
		return BasicTypes[BooleanTypeKind]
	case "bytes", "[]uint8", "[]byte":
		return BasicTypes[BytesTypeKind]
	case "interface {}", "var", "any":
		return BasicTypes[AnyTypeKind]
	case "error":
		return BasicTypes[ErrorType]
	default:
		return nil
	}
}

// ====================== alias type
type AliasType struct {
	elem    Type
	method  map[string]*FunctionType
	Name    string
	pkgPath string
}

var _ Type = (*AliasType)(nil)

func NewAliasType(name, pkg string, elem Type) *AliasType {
	return &AliasType{
		elem:    elem,
		method:  make(map[string]*FunctionType),
		Name:    name,
		pkgPath: pkg,
	}
}

func (a *AliasType) SetMethod(m map[string]*FunctionType) {
	a.method = m
}

func (b *AliasType) AddMethod(id string, f *FunctionType) {
	if b.method == nil {
		b.method = make(map[string]*FunctionType)
	}
	b.method[id] = f
}

func (a *AliasType) GetMethod() map[string]*FunctionType {
	return a.method
}

// func (b *AliasType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

func (a *AliasType) String() string {
	if a.Name != "" {
		return a.Name
	} else {
		return a.RawString()
	}
}

func (b *AliasType) PkgPathString() string {
	result := b.pkgPath
	if result == "" {
		result = b.RawString()
	}
	return result
}

func (a *AliasType) RawString() string {
	return fmt.Sprintf("type %s (%s)", a.Name, a.elem)
}

func (a *AliasType) GetTypeKind() TypeKind {
	return a.elem.GetTypeKind()
}

// ====================== interface type
type InterfaceType struct {
	method  map[string]*FunctionType
	name    string
	pkgPath string
}

func NewInterfaceType(name, pkgPath string) *InterfaceType {
	return &InterfaceType{
		method:  make(map[string]*FunctionType),
		name:    name,
		pkgPath: pkgPath,
	}
}

var _ Type = (*InterfaceType)(nil)

func (i *InterfaceType) SetMethod(m map[string]*FunctionType) {
	i.method = m
}

func (b *InterfaceType) AddMethod(id string, f *FunctionType) {
	if b.method == nil {
		b.method = make(map[string]*FunctionType)
	}
	b.method[id] = f
}

func (i *InterfaceType) GetMethod() map[string]*FunctionType {
	return i.method
}

// func (b *InterfaceType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

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

func (i *InterfaceType) PkgPathString() string {
	result := i.pkgPath
	if result == "" {
		result = i.RawString()
	}
	return result
}

func (i *InterfaceType) RawString() string {
	return fmt.Sprintf("type %s interface{}", i.name)
}

// ====================== chan type
type ChanType struct {
	Elem   Type
	method map[string]*FunctionType
}

var _ (Type) = (*ChanType)(nil)

func (c *ChanType) SetMethod(m map[string]*FunctionType) {
	c.method = m
}

func (b *ChanType) AddMethod(id string, f *FunctionType) {
	if b.method == nil {
		b.method = make(map[string]*FunctionType)
	}
	b.method[id] = f
}

func (c *ChanType) GetMethod() map[string]*FunctionType {
	return c.method
}

// func (b *ChanType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

func (c *ChanType) GetTypeKind() TypeKind {
	return ChanTypeKind
}

func NewChanType(elem Type) *ChanType {
	return &ChanType{
		Elem: elem,
	}
}

func (c ChanType) String() string {
	return fmt.Sprintf("chan %s", c.Elem)
}

func (c ChanType) PkgPathString() string {
	return fmt.Sprintf("chan %s", c.Elem.PkgPathString())
}

func (c ChanType) RawString() string {
	return c.String()
}

// ==================== interface type
type ObjectType struct {
	Name       string
	pkgPath    string
	Kind       TypeKind
	Len        int
	Keys       []Value
	keyTypes   []Type
	FieldTypes []Type

	AnonymousField []*ObjectType

	Combination bool // function multiple return will combined to struct
	// VariadicPara bool // function last variadic parameter will become slice

	method map[string]*FunctionType

	KeyTyp    Type
	FieldType Type
}

func (i *ObjectType) GetTypeKind() TypeKind {
	return i.Kind
}

func (i *ObjectType) GetMethod() map[string]*FunctionType {
	return i.method
}

func (i *ObjectType) SetMethod(m map[string]*FunctionType) {
	i.method = m
}

func (b *ObjectType) AddMethod(id string, f *FunctionType) {
	if b.method == nil {
		b.method = make(map[string]*FunctionType)
	}
	b.method[id] = f
}

// func (b *ObjectType) GetAllKey() []string {
// 	return append(lo.Keys(b.method), lo.Map(b.Key, func(v Value, _ int) string { return v.String() })...)
// }

var _ (Type) = (*ObjectType)(nil)

func (i *ObjectType) SetName(name string) {
	i.Name = name
}

func NewObjectType() *ObjectType {
	return &ObjectType{
		Kind:       ObjectTypeKind,
		Keys:       make([]Value, 0),
		keyTypes:   make([]Type, 0),
		FieldTypes: make([]Type, 0),
		method:     make(map[string]*FunctionType, 0),
	}
}

// for slice build
func NewSliceType(elem Type) *ObjectType {
	i := NewObjectType()
	i.Kind = SliceTypeKind
	i.KeyTyp = BasicTypes[NumberTypeKind]
	i.FieldType = elem
	return i
}

func NewMapType(key, field Type) *ObjectType {
	i := NewObjectType()
	i.KeyTyp = key
	i.FieldType = field
	i.Kind = MapTypeKind
	return i
}

func NewStructType() *ObjectType {
	i := NewObjectType()
	i.Kind = StructTypeKind
	return i
}

func (itype ObjectType) String() string {
	if itype.Combination {
		return strings.Join(
			lo.Map(
				itype.FieldTypes,
				func(t Type, _ int) string { return t.String() },
			),
			", ",
		)
	}
	if itype.Name != "" {
		return itype.Name
	}
	return itype.RawString()
}

func (i ObjectType) PkgPathString() string {
	result := i.pkgPath
	if result == "" {
		result = i.RawString()
	}
	return result
}

func (itype ObjectType) RawString() string {
	ret := ""
	switch itype.Kind {
	case SliceTypeKind:
		// map[int]T
		if itype.Len == 0 {
			ret += fmt.Sprintf("[]%s", itype.FieldType.String())
		} else {
			ret += fmt.Sprintf("[%d]%s", itype.Len, itype.FieldType.String())
		}
	case MapTypeKind:
		// map[T]U
		// if len(itype.keyType) == 1 && len(itype.Field) == 1 {
		keyTyp := itype.KeyTyp
		if utils.IsNil(keyTyp) {
			keyTyp = BasicTypes[AnyTypeKind]
		}
		fieldType := itype.FieldType
		if utils.IsNil(fieldType) {
			fieldType = BasicTypes[AnyTypeKind]
		}
		ret += fmt.Sprintf("map[%s]%s", keyTyp.String(), fieldType.String())
		// } else {
		// 	panic("this interface type not map")
		// }
	case StructTypeKind:
		// map[string](T/U/xx)
		ret += fmt.Sprintf(
			"struct {%s}",
			strings.Join(
				lo.Map(itype.FieldTypes, func(field Type, _ int) string { return field.String() }),
				",",
			),
		)
	case ObjectTypeKind:
		ret += "object{}"
	}
	return ret
}

// for struct build
func (s *ObjectType) AddField(key Value, field Type) {
	s.Keys = append(s.Keys, key)
	keyTyp := key.GetType()
	s.keyTypes = append(s.keyTypes, keyTyp)
	if field == nil {
		field = BasicTypes[AnyTypeKind]
	}
	s.FieldTypes = append(s.FieldTypes, field)
}

// return (field-type, key-type)
func (s *ObjectType) GetField(key Value) Type {
	switch s.Kind {
	case SliceTypeKind, MapTypeKind:
		if TypeCompare(key.GetType(), s.KeyTyp) {
			return s.FieldType
		}
	case StructTypeKind, ObjectTypeKind:
		getField := func(o *ObjectType) Type {
			if index := slices.IndexFunc(o.Keys, func(v Value) bool { return v.String() == key.String() }); index != -1 {
				return o.FieldTypes[index]
			} else {
				return nil
			}
		}
		if t := getField(s); t != nil {
			return t
		}
		for _, obj := range s.AnonymousField {
			if t := getField(obj); t != nil {
				return t
			}
		}
	}
	return nil
}

// ===================== Finish simply
func (s *ObjectType) Finish() {
	if s.Kind != ObjectTypeKind {
		return
	}
	// TODO: handler this hash later
	fieldTypes := lo.UniqBy(s.FieldTypes, func(t Type) string { return t.String() })
	keyTypes := lo.UniqBy(s.keyTypes, func(t Type) string { return t.String() })
	if len(fieldTypes) == 1 {
		s.FieldType = fieldTypes[0]
	} else {
		s.FieldType = BasicTypes[AnyTypeKind]
	}
	if len(keyTypes) == 1 {
		s.KeyTyp = keyTypes[0]
	} else {
		s.KeyTyp = BasicTypes[AnyTypeKind]
	}
}

type FunctionType struct {
	Name         string
	pkgPath      string
	ReturnType   Type
	Parameter    Types
	FreeValue    []string
	SideEffects  []string
	IsVariadic   bool
	IsMethod     bool
	IsModifySelf bool // if this is method function
}

var _ Type = (*FunctionType)(nil)

func (f *FunctionType) GetMethod() map[string]*FunctionType {
	return nil
}

func (f *FunctionType) SetMethod(m map[string]*FunctionType) {}
func (b *FunctionType) AddMethod(id string, f *FunctionType) {}

func (f *FunctionType) SetModifySelf(b bool) { f.IsModifySelf = b }

// func (b *FunctionType) GetAllKey() []string {
// 	return []string{}
// }

func CalculateType(ts []Type) Type {
	if len(ts) == 0 {
		return BasicTypes[NullTypeKind]
	} else if len(ts) == 1 {
		return ts[0]
	} else {
		i := NewObjectType()
		for index, typ := range ts {
			i.AddField(NewConst(index), typ)
		}
		i.Finish()
		i.Combination = true
		i.Kind = StructTypeKind
		// i.SetLen(NewConst(len(ts)))
		i.Len = len(ts)
		return i
	}
}

func NewFunctionType(name string, Parameter []Type, ReturnType Type, IsVariadic bool) *FunctionType {
	f := &FunctionType{
		Name:       name,
		Parameter:  Parameter,
		IsVariadic: IsVariadic,
		ReturnType: ReturnType,
	}
	return f
}

func NewFunctionTypeDefine(name string, Parameter []Type, ReturnType []Type, IsVariadic bool) *FunctionType {
	return NewFunctionType(name, Parameter, CalculateType(ReturnType), IsVariadic)
}

func (s *FunctionType) SetFreeValue(fv []string) {
	s.FreeValue = fv
}

func (s *FunctionType) SetSideEffect(se []string) {
	s.SideEffects = se
}

func (s *FunctionType) SetName(name string) {
	s.Name = name
}

func (s *FunctionType) PkgPathString() string {
	result := s.pkgPath
	if result == "" {
		result = s.RawString()
	}
	return result
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

func (s *FunctionType) GetParamString() string {
	ret := ""
	for index, t := range s.Parameter {
		if index == len(s.Parameter)-1 {
			if s.IsVariadic {
				if obj, ok := ToObjectType(t); ok && obj.Kind == SliceTypeKind {
					// last
					ret += "..." + obj.FieldType.String()
				}
			} else {
				ret += t.String()
			}
		} else {
			ret += t.String() + ", "
		}
	}
	return ret
}

func (s *FunctionType) GetTypeKind() TypeKind {
	return FunctionTypeKind
}
