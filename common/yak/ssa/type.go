package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func init() {
	BasicTypes[ErrorTypeKind].method["Error"] = NewFunctionWithType("error.Error", NewFunctionTypeDefine(
		"error.Error",
		[]Type{BasicTypes[ErrorTypeKind]},
		[]Type{BasicTypes[StringTypeKind]},
		false,
	))
}

const MAXTypeCompareDepth = 10

func TypeCompare(t1, t2 Type) bool {
	return typeCompareEx(t1, t2, 0) || typeCompareEx(t2, t1, 0)
}

func typeCompareEx(t1, t2 Type, depth int) bool {
	if t1 == nil || t2 == nil {
		return false
	}
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
		if t1f.ParameterLen != t2f.ParameterLen {
			return false
		}
		for i := 0; i < t1f.ParameterLen; i++ {
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
	case GenericTypeKind:
		if t2kind != GenericTypeKind {
			return false
		}

		if t2.(*GenericType).symbol == t1.(*GenericType).symbol {
			return true
		}
		return false
	default:
	}
	return t1kind == t2kind
}

type MethodBuilder interface {
	Build(Type, string) *Function
	GetMethodNames(Type) []string
}

var ExternMethodBuilder MethodBuilder

func GetMethod(t Type, id string) *Function {
	var f *Function
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
		f.SetMethod(true, t)
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
	if t == nil {
		return []string{}
	}
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
	RawString() string     // string contain inner information
	GetTypeKind() TypeKind // type kind

	// set/get method, method is a function
	SetMethod(map[string]*Function)
	AddMethod(string, *Function)
	GetMethod() map[string]*Function
}

type Types []Type

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

// TypeKind is a Kind of ssa.type
type TypeKind int

const (
	// NumberTypeKind is all number type, int*/uint*/float/double/complex
	NumberTypeKind TypeKind = iota
	StringTypeKind
	BytesTypeKind
	BooleanTypeKind
	UndefinedTypeKind // undefined is nil in golang
	NullTypeKind      //
	AnyTypeKind       // any type
	ChanTypeKind
	ErrorTypeKind

	ObjectTypeKind
	SliceTypeKind  // slice
	MapTypeKind    // map
	StructTypeKind // struct
	TupleTypeKind  //  slice has fixed length

	InterfaceTypeKind
	FunctionTypeKind

	ClassBluePrintTypeKind
	GenericTypeKind
)

type BasicType struct {
	Kind    TypeKind
	name    string
	pkgPath string

	method map[string]*Function
}

func NewBasicType(kind TypeKind, name string) *BasicType {
	return &BasicType{
		Kind:    kind,
		name:    name,
		pkgPath: name,
		method:  make(map[string]*Function),
	}
}

var _ Type = (*BasicType)(nil)

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

func (b *BasicType) GetMethod() map[string]*Function {
	return b.method
}

func (b *BasicType) SetMethod(method map[string]*Function) {
	b.method = method
}

func (b *BasicType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

// func (b *BasicType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

var BasicTypes = map[TypeKind]*BasicType{
	NumberTypeKind:    NewBasicType(NumberTypeKind, "number"),
	StringTypeKind:    NewBasicType(StringTypeKind, "string"),
	BytesTypeKind:     NewBasicType(BytesTypeKind, "bytes"),
	BooleanTypeKind:   NewBasicType(BooleanTypeKind, "boolean"),
	UndefinedTypeKind: NewBasicType(UndefinedTypeKind, "undefined"),
	NullTypeKind:      NewBasicType(NullTypeKind, "null"),
	AnyTypeKind:       NewBasicType(AnyTypeKind, "any"),
	ErrorTypeKind:     NewBasicType(ErrorTypeKind, "error"),
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
	return BasicTypes[ErrorTypeKind]
}

func GetType(i any) Type {
	if typ := GetTypeByStr(reflect.TypeOf(i).String()); typ != nil {
		return typ
	} else {
		panic("undefined type: " + spew.Sdump(i))
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
		return BasicTypes[ErrorTypeKind]
	default:
		return nil
	}
}

// ====================== alias type
type AliasType struct {
	elem    Type
	method  map[string]*Function
	Name    string
	pkgPath string
}

var _ Type = (*AliasType)(nil)

func NewAliasType(name, pkg string, elem Type) *AliasType {
	return &AliasType{
		elem:    elem,
		method:  make(map[string]*Function),
		Name:    name,
		pkgPath: pkg,
	}
}

func (a *AliasType) SetMethod(m map[string]*Function) {
	a.method = m
}

func (b *AliasType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

func (a *AliasType) GetMethod() map[string]*Function {
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
	method  map[string]*Function
	name    string
	pkgPath string
}

func NewInterfaceType(name, pkgPath string) *InterfaceType {
	return &InterfaceType{
		method:  make(map[string]*Function),
		name:    name,
		pkgPath: pkgPath,
	}
}

var _ Type = (*InterfaceType)(nil)

func (i *InterfaceType) SetMethod(m map[string]*Function) {
	i.method = m
}

func (b *InterfaceType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

func (i *InterfaceType) GetMethod() map[string]*Function {
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
	method map[string]*Function
}

var _ (Type) = (*ChanType)(nil)

func (c *ChanType) SetMethod(m map[string]*Function) {
	c.method = m
}

func (b *ChanType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

func (c *ChanType) GetMethod() map[string]*Function {
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
	keymap     map[string]int // remove duplicate key
	keyTypes   []Type
	FieldTypes []Type

	AnonymousField []*ObjectType

	Combination bool // function multiple return will combined to struct
	// VariadicPara bool // function last variadic parameter will become slice

	method map[string]*Function

	KeyTyp    Type
	FieldType Type
}

var _ (Type) = (*ObjectType)(nil)

func (i *ObjectType) GetTypeKind() TypeKind {
	return i.Kind
}

func (i *ObjectType) SetTypeKind(t TypeKind) {
	i.Kind = t
}

func (i *ObjectType) GetMethod() map[string]*Function {
	return i.method
}

func (i *ObjectType) SetMethod(m map[string]*Function) {
	i.method = m
}

func (b *ObjectType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

func (i *ObjectType) SetName(name string) {
	i.Name = name
}

func (i *ObjectType) SetPkgPath(pkg string) {
	i.pkgPath = pkg
}

func NewObjectType() *ObjectType {
	return &ObjectType{
		Kind:       ObjectTypeKind,
		Keys:       make([]Value, 0),
		keymap:     make(map[string]int),
		keyTypes:   make([]Type, 0),
		FieldTypes: make([]Type, 0),
		method:     make(map[string]*Function, 0),
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

func (itype *ObjectType) String() string {
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

func (i *ObjectType) PkgPathString() string {
	result := i.pkgPath
	if result == "" {
		result = i.RawString()
	}
	return result
}

func (itype *ObjectType) RawString() string {
	itype.Name = "..."
	ret := ""
	switch itype.Kind {
	case SliceTypeKind:
		if itype.FieldType != nil {
			// map[int]T
			if itype.Len == 0 {
				ret += fmt.Sprintf("[]%s", itype.FieldType.String())
			} else {
				ret += fmt.Sprintf("[%d]%s", itype.Len, itype.FieldType.String())
			}
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
	case TupleTypeKind:
		// [T,U,V]
		ret += fmt.Sprintf(
			"[%s]", strings.Join(
				lo.Map(itype.FieldTypes, func(field Type, _ int) string { return field.String() }),
				",",
			),
		)
	}
	itype.Name = ""
	return ret
}

// for struct build
func (s *ObjectType) AddField(key Value, field Type) {
	keyTyp := key.GetType()
	if field == nil {
		field = BasicTypes[AnyTypeKind]
	}

	if index, ok := s.keymap[key.String()]; ok {
		s.keyTypes[index] = keyTyp
		s.Keys[index] = key
		s.FieldTypes[index] = field
		return
	}

	s.Keys = append(s.Keys, key)
	s.keyTypes = append(s.keyTypes, keyTyp)
	s.FieldTypes = append(s.FieldTypes, field)

	s.keymap[key.String()] = len(s.Keys) - 1
}

// return (field-type, key-type)
func (s *ObjectType) GetField(key Value) Type {
	getField := func(o *ObjectType) Type {
		if index := slices.IndexFunc(o.Keys, func(v Value) bool { return v.String() == key.String() }); index != -1 {
			return o.FieldTypes[index]
		} else {
			return nil
		}
	}

	switch s.Kind {
	case SliceTypeKind, MapTypeKind:
		if TypeCompare(key.GetType(), s.KeyTyp) {
			if t := getField(s); t != nil {
				return t
			}
			return s.FieldType
		}
	case StructTypeKind, ObjectTypeKind, TupleTypeKind:
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
	Name            string
	pkgPath         string
	This            *Function
	ReturnType      Type
	ReturnValue     []*Return
	Parameter       Types
	ParameterLen    int
	ParameterValue  []*Parameter
	FreeValue       []*Parameter
	ParameterMember []*ParameterMember
	SideEffects     []*FunctionSideEffect
	IsVariadic      bool
	IsMethod        bool
	ObjectType      Type
	IsModifySelf    bool // if this is method function

	AnnotationFunc []func(Value)
}

var _ Type = (*FunctionType)(nil)

func (f *FunctionType) GetMethod() map[string]*Function {
	return nil
}

func (f *FunctionType) SetMethod(m map[string]*Function) {}
func (b *FunctionType) AddMethod(id string, f *Function) {}

func (f *FunctionType) SetModifySelf(b bool) { f.IsModifySelf = b }

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
		i.Kind = TupleTypeKind
		// i.SetLen(NewConst(len(ts)))
		i.Len = len(ts)
		return i
	}
}

func NewFunctionType(name string, Parameter []Type, ReturnType Type, IsVariadic bool) *FunctionType {
	f := &FunctionType{
		Name:         name,
		Parameter:    Parameter,
		ParameterLen: len(Parameter),
		IsVariadic:   IsVariadic,
		ReturnType:   ReturnType,
	}
	return f
}

func NewFunctionTypeDefine(name string, Parameter []Type, ReturnType []Type, IsVariadic bool) *FunctionType {
	return NewFunctionType(name, Parameter, CalculateType(ReturnType), IsVariadic)
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
	variadic := ""
	if s.IsVariadic {
		variadic += "..."
	}

	paras := make([]string, 0, s.ParameterLen)
	for i := 0; i < s.ParameterLen; i++ {
		paras = append(paras, s.Parameter[i].String())
	}

	return fmt.Sprintf(
		"(%s %s) -> %s",
		strings.Join(
			paras,
			",",
		),
		variadic,
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

func (f *FunctionType) AddAnnotationFunc(handler ...func(Value)) {
	f.AnnotationFunc = append(f.AnnotationFunc, handler...)
}

// ====================== generic type
type GenericType struct {
	method map[string]*Function
	symbol string
}

var _ (Type) = (*GenericType)(nil)

func (c *GenericType) SetMethod(m map[string]*Function) {
	c.method = m
}

func (b *GenericType) AddMethod(id string, f *Function) {
	if b.method == nil {
		b.method = make(map[string]*Function)
	}
	b.method[id] = f
}

func (c *GenericType) GetMethod() map[string]*Function {
	return c.method
}

func (c *GenericType) GetTypeKind() TypeKind {
	return GenericTypeKind
}

var (
	// T is a generic type
	TypeT = NewGenericType("T")
	TypeU = NewGenericType("U")
)

func NewGenericType(symbol string) *GenericType {
	return &GenericType{
		symbol: symbol,
		method: make(map[string]*Function),
	}
}

func (c GenericType) String() string {
	return fmt.Sprintf("%s", c.symbol)
}

func (c GenericType) PkgPathString() string {
	return fmt.Sprintf("%s", c.symbol)
}

func (c GenericType) RawString() string {
	return c.String()
}

func isSameGenericType(t1, t2 Type) bool {
	if t1.GetTypeKind() != GenericTypeKind || t2.GetTypeKind() != GenericTypeKind {
		return false
	}
	return t1.(*GenericType).symbol == t2.(*GenericType).symbol
}

func GetGenericTypeFromType(t Type) []Type {
	typs := make([]Type, 0)
	switch t.GetTypeKind() {
	case GenericTypeKind:
		typs = append(typs, t)
	case ChanTypeKind:
		typs = append(typs, GetGenericTypeFromType(t.(*ChanType).Elem)...)
	case SliceTypeKind:
		typs = append(typs, GetGenericTypeFromType(t.(*ObjectType).FieldType)...)
	case TupleTypeKind:
		obj := t.(*ObjectType)
		for _, typ := range obj.FieldTypes {
			typs = append(typs, GetGenericTypeFromType(typ)...)
		}
	case MapTypeKind:
		obj := t.(*ObjectType)
		typs = append(typs, GetGenericTypeFromType(obj.KeyTyp)...)
		typs = append(typs, GetGenericTypeFromType(obj.FieldType)...)
	case FunctionTypeKind:
		obj := t.(*FunctionType)
		for _, typ := range obj.Parameter {
			typs = append(typs, GetGenericTypeFromType(typ)...)
		}
		typs = append(typs, GetGenericTypeFromType(obj.ReturnType)...)
	}
	return typs
}

func (c *GenericType) Binding(real, generic Type, symbolsTypeMap map[string]Type) (errMsg string) {
	setBinding := func(typ Type) {
		if existed, ok := symbolsTypeMap[c.symbol]; ok {
			if !TypeCompare(existed, typ) {
				errMsg = GenericTypeError(c, generic, existed, typ)
			}
		} else {
			symbolsTypeMap[c.symbol] = typ
		}
	}

	switch real.GetTypeKind() {
	case GenericTypeKind,
		StringTypeKind, NumberTypeKind, BooleanTypeKind, BytesTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		if isSameGenericType(generic, c) {
			setBinding(real)
		}
	case ChanTypeKind:
		if t, ok := generic.(*ChanType); ok && isSameGenericType(t.Elem, c) {
			setBinding(real.(*ChanType).Elem)
		}
	case SliceTypeKind:
		if t, ok := generic.(*ObjectType); ok && isSameGenericType(t.FieldType, c) {
			setBinding(real.(*ObjectType).FieldType)
		}
	case MapTypeKind:
		if t, ok := generic.(*ObjectType); ok && isSameGenericType(t.KeyTyp, c) {
			setBinding(real.(*ObjectType).KeyTyp)
		}
		if t, ok := generic.(*ObjectType); ok && isSameGenericType(t.FieldType, c) {
			setBinding(real.(*ObjectType).FieldType)
		}
	case TupleTypeKind:
		if t, ok := generic.(*ObjectType); ok && isSameGenericType(t.FieldType, c) {
			setBinding(real.(*ObjectType).FieldType)
		}
	case FunctionTypeKind:
		if t, ok := generic.(*FunctionType); ok {
			rt := real.(*FunctionType)
			for i, typ := range t.Parameter {
				if !isSameGenericType(typ, c) {
					continue
				}
				setBinding(rt.Parameter[i])
			}
			if isSameGenericType(t.ReturnType, c) {
				setBinding(rt.ReturnType)
			}
		}
	}
	return
}

func (c *GenericType) Apply(raw Type, symbolsTypeMap map[string]Type) Type {
	new, ok := CloneType(raw)
	if !ok {
		return raw
	}

	switch raw.GetTypeKind() {
	case GenericTypeKind:
		if TypeCompare(new, c) {
			if new, ok := CloneType(symbolsTypeMap[c.symbol]); ok {
				return new
			}
		}
	case ChanTypeKind:
		t := new.(*ChanType)
		t.Elem = c.Apply(t.Elem, symbolsTypeMap)
		return new
	case SliceTypeKind:
		t := new.(*ObjectType)
		t.FieldType = c.Apply(t.FieldType, symbolsTypeMap)
	case MapTypeKind:
		t := new.(*ObjectType)
		t.KeyTyp = c.Apply(t.KeyTyp, symbolsTypeMap)
		t.FieldType = c.Apply(t.FieldType, symbolsTypeMap)
	case TupleTypeKind:
		t := new.(*ObjectType)
		t.FieldType = c.Apply(t.FieldType, symbolsTypeMap)
	case FunctionTypeKind:
		t := new.(*FunctionType)
		for i, typ := range t.Parameter {
			t.Parameter[i] = c.Apply(typ, symbolsTypeMap)
		}
		t.ReturnType = c.Apply(t.ReturnType, symbolsTypeMap)
	}

	return new
}

// Shallow copy
func CloneType(t Type) (Type, bool) {
	switch t.GetTypeKind() {
	case GenericTypeKind,
		StringTypeKind, NumberTypeKind, BooleanTypeKind, BytesTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		return t, true
	case ChanTypeKind:
		old := t.(*ChanType)
		return NewChanType(old.Elem), true
	case SliceTypeKind:
		old := t.(*ObjectType)
		return NewSliceType(old.FieldType), true
	case MapTypeKind:
		old := t.(*ObjectType)
		return NewMapType(old.KeyTyp, old.FieldType), true
	case TupleTypeKind:
		old := t.(*ObjectType)
		return CalculateType(old.FieldTypes), true
	case FunctionTypeKind:
		old := t.(*FunctionType)
		return NewFunctionType(old.Name, old.Parameter, old.ReturnType, old.IsVariadic), true
	}
	return nil, false
}
