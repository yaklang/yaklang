package ssa

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

const MAXTypeCompareDepth = 10

func typeEqualEx(t1, t2 Type, depth int) bool {
	t1kind := t1.GetTypeKind()
	t2kind := t2.GetTypeKind()
	if depth == MAXTYPELEVEL {
		return true
	}
	depth += 1

	switch t1kind {
	case FunctionTypeKind:
		t1f, _ := ToFunctionType(t1)
		t2f, _ := ToFunctionType(t2)
		if t1f.IsAnyFunctionType() {
			return t2f.IsAnyFunctionType()
		}

		if t1f.ParameterLen != t2f.ParameterLen {
			return false
		}
		for i := 0; i < t1f.ParameterLen; i++ {
			if !typeEqualEx(t1f.Parameter[i], t2f.Parameter[i], depth) {
				return false
			}
		}
		return typeEqualEx(t1f.ReturnType, t2f.ReturnType, depth)
	case SliceTypeKind:
		t1o, _ := ToObjectType(t1)
		t2o, _ := ToObjectType(t2)
		return typeEqualEx(t1o.FieldType, t2o.FieldType, depth)
	case MapTypeKind:
		t1o, _ := ToObjectType(t1)
		t2o, _ := ToObjectType(t2)
		return typeEqualEx(t1o.FieldType, t2o.FieldType, depth) && typeEqualEx(t1o.KeyTyp, t2o.KeyTyp, depth)
	case StructTypeKind, ObjectTypeKind:
	case BytesTypeKind:
		if t2kind == StringTypeKind {
			return true
		}
	case StringTypeKind:
		if t2kind == BytesTypeKind {
			return true
		}
	case GenericTypeKind:
		if t2kind != GenericTypeKind {
			return false
		}

		return t2.(*GenericType).symbol == t1.(*GenericType).symbol
	case OrTypeKind:
		t1o := t1.(*OrType)
		t2o := t2.(*OrType)
		if len(t1o.types) != len(t2o.types) {
			return false
		}
		for i, t := range t1o.types {
			t2 := t2o.types[i]
			if !typeEqualEx(t, t2, depth) {
				return false
			}
		}
	}

	return t1kind == t2kind
}

func clean(input []string) []string {
	seen := make(map[string]bool)
	var output []string
	for _, name := range input {
		if !seen[name] {
			seen[name] = true
			output = append(output, name)
		}
	}
	return output
}

func TypeEqual(t1, t2 Type) bool {
	return typeEqualEx(t1, t2, 0) || typeEqualEx(t2, t1, 0)
}

func TypeCompare(t1, t2 Type) bool {
	return typeCompareEx(t1, t2, 0) || typeCompareEx(t2, t1, 0)
}

func typeCompareEx(t1, t2 Type, depth int) bool {
	if t1 == nil || t2 == nil {
		return false
	}
	t1kind := t1.GetTypeKind()
	t2kind := t2.GetTypeKind()

	if t1kind == AliasTypeKind {
		t1kind = t1.(*AliasType).GetType().GetTypeKind()
	}
	if t2kind == AliasTypeKind {
		t2kind = t2.(*AliasType).GetType().GetTypeKind()
	}

	if t1kind == AnyTypeKind || t2kind == AnyTypeKind {
		return true
	}
	if t1kind == GenericTypeKind || t2kind == GenericTypeKind {
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
		if t1f.IsAnyFunctionType() {
			return true
		}
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
	case ObjectTypeKind, StructTypeKind:
		o, ok := ToObjectType(t1)
		if !ok {
			break
		}
		o2, ok := ToObjectType(t2)
		if !ok {
			break
		}
		if o.PkgPathString() != o2.PkgPathString() {
			return false
		}
	case BytesTypeKind:
		// string | []number
		if t2kind == StringTypeKind {
			return true
		}
		if t2kind == SliceTypeKind {
			if o, ok := ToObjectType(t2); ok {
				return typeCompareEx(CreateByteType(), o.FieldType, depth)
			}
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

		return t2.(*GenericType).symbol == t1.(*GenericType).symbol
	case OrTypeKind:
		rt1 := t1.(*OrType)
		for _, t := range rt1.types {
			ok := typeCompareEx(t, t2, depth)
			if ok {
				return true
			}
		}
	default:
	}
	return t1kind == t2kind
}

type MethodBuilder interface {
	Build(Type, string) *Function
	GetMethodNames(Type) []string
}

var ExternMethodBuilder MethodBuilder

func GetMethod(t Type, id string, peek ...bool) *Function {
	var f *Function
	if utils.IsNil(t) {
		log.Error("[BUG]: type is nil")
		return f
	}
	if fun, ok := t.GetMethod()[id]; ok {
		f = fun
		// peek is true, don't build
		if len(peek) == 0 || !peek[0] {
			f.Build()
		}
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
	case AliasTypeKind:
		a, _ := ToAliasType(t)
		ret = append(ret, GetAllKey(a.GetType())...)
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
	GetId() int64
	SetId(int64)

	String() string        // only string
	PkgPathString() string // package path string
	RawString() string     // string contain inner information
	GetTypeKind() TypeKind // type kind

	// full type name
	AddFullTypeName(string)
	GetFullTypeNames() []string
	SetFullTypeNames([]string)
	// set/get method, method is a function
	SetMethod(map[string]*Function)
	SetMethodGetter(func() map[string]*Function)
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
	AliasTypeKind
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
	ByteTypeKind
	OrTypeKind

	PointerKind
)

type baseType struct {
	id           int64
	method       map[string]*Function
	methodGetter func() map[string]*Function
	methodOnce   sync.Once
	methodLock   sync.RWMutex
}

func NewBaseType() *baseType {
	return &baseType{}
}

func (b *baseType) SetId(id int64) {
	b.id = id
}
func (b *baseType) GetId() int64 {
	return b.id
}

func (b *baseType) SetMethodGetter(f func() map[string]*Function) {
	b.methodGetter = f
}

func (b *baseType) SetMethod(m map[string]*Function) {
	b.method = m
}

func (b *baseType) AddMethod(id string, f *Function) {
	if b.method == nil {
		// init
		b.GetMethod()
	}
	b.methodLock.Lock()
	defer b.methodLock.Unlock()
	b.method[id] = f
}

func (b *baseType) GetMethod() map[string]*Function {
	if b.method == nil {
		// 防止并发
		b.methodOnce.Do(func() {
			if b.methodGetter == nil {
				b.method = make(map[string]*Function)
			} else {
				b.method = b.methodGetter()
			}
		})
	}
	return b.method
}

func (b *baseType) RangeMethod(f func(string, *Function)) {
	b.methodLock.RLock()
	defer b.methodLock.RUnlock()
	for k, v := range b.method {
		f(k, v)
	}
}

type BasicType struct {
	*baseType
	Kind    TypeKind
	name    string
	pkgPath string

	fullTypeName []string
}

func NewBasicType(kind TypeKind, name string) *BasicType {
	typ := &BasicType{
		baseType:     NewBaseType(),
		Kind:         kind,
		name:         name,
		pkgPath:      name,
		fullTypeName: make([]string, 0),
	}
	return typ
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

func (b *BasicType) AddFullTypeName(name string) {
	if b == nil {
		return
	}

	if !lo.Contains(b.fullTypeName, name) {
		b.fullTypeName = append(b.fullTypeName, name)
	}
}

func (b *BasicType) GetFullTypeNames() []string {
	if b == nil {
		return nil
	}
	return b.fullTypeName
}

func (b *BasicType) SetFullTypeNames(names []string) {
	if b == nil {
		return
	}
	b.fullTypeName = clean(names)
}

func (b *BasicType) IsAny() bool {
	if b == nil {
		return true
	}
	if b.Kind == AnyTypeKind {
		return true
	}
	return false
}

func (b *BasicType) IsNull() bool {
	if b == nil {
		return true
	}
	if b.Kind == NullTypeKind {
		return true
	}
	return false
}

func (b *BasicType) GetName() string {
	return b.name
}

// func (b *BasicType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

func CreateNumberType() Type {
	return NewBasicType(NumberTypeKind, "number")
}

func CreateStringType() Type {
	return NewBasicType(StringTypeKind, "string")
}

func CreateByteType() Type {
	return NewBasicType(ByteTypeKind, "byte")
}

func CreateBytesType() Type {
	return NewBasicType(BytesTypeKind, "bytes")
}

func CreateBooleanType() Type {
	return NewBasicType(BooleanTypeKind, "boolean")
}

func CreateUndefinedType() Type {
	return NewBasicType(UndefinedTypeKind, "undefined")
}

func CreateNullType() Type {
	return NewBasicType(NullTypeKind, "null")
}

func CreateAnyType() Type {
	return NewBasicType(AnyTypeKind, "any")
}

func CreateErrorType() Type {
	ret := NewBasicType(ErrorTypeKind, "error")
	ret.AddMethod("Error", NewFunctionWithType("error.Error", NewFunctionTypeDefine(
		"error.Error",
		[]Type{ret},
		[]Type{CreateStringType()},
		false,
	)))
	return ret
}

func GetType(i any) Type {
	if utils.IsNil(i) {
		return CreateNullType()
	}
	if typ := GetTypeByStr(reflect.TypeOf(i).String()); typ != nil {
		return typ
	} else {
		return CreateAnyType()
	}
}

func GetTypeByStr(typ string) Type {
	switch typ {
	case "uint", "uint8", "byte", "uint16", "uint32", "uint64", "int", "int8", "int16", "int32", "int64", "uintptr":
		return CreateNumberType()
	case "float", "float32", "float64", "double", "complex128", "complex64":
		return CreateNumberType()
	case "string":
		return CreateStringType()
	case "bool":
		return CreateBooleanType()
	case "char":
		return CreateByteType()
	case "bytes", "[]uint8", "[]byte":
		return CreateBytesType()
	case "interface {}", "var", "any":
		return CreateAnyType()
	case "error":
		return CreateErrorType()
	default:
		return nil
	}
}

// ====================== alias type
type AliasType struct {
	*baseType
	elem         Type
	Name         string
	pkgPath      string
	fullTypeName []string
}

var _ Type = (*AliasType)(nil)

func NewAliasType(name, pkg string, elem Type) *AliasType {
	return &AliasType{
		baseType: NewBaseType(),
		elem:     elem,
		Name:     name,
		pkgPath:  pkg,
	}
}

func (a *AliasType) AddFullTypeName(name string) {
	if a == nil {
		return
	}
	if !lo.Contains(a.fullTypeName, name) {
		a.fullTypeName = append(a.fullTypeName, name)
	}
}

func (a *AliasType) GetFullTypeNames() []string {
	if a == nil {
		return nil
	}
	return a.fullTypeName
}

func (a *AliasType) SetFullTypeNames(names []string) {
	if a == nil {
		return
	}
	a.fullTypeName = clean(names)
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
	return AliasTypeKind
}

func (a *AliasType) GetType() Type {
	return a.elem
}

// ====================== interface type
type InterfaceType struct {
	*baseType
	object       map[string]*ObjectType
	name         string
	pkgPath      string
	fullTypeName []string
	parents      []*InterfaceType
	childs       []*InterfaceType
}

func NewInterfaceType(name, pkgPath string) *InterfaceType {
	return &InterfaceType{
		baseType: NewBaseType(),
		name:     name,
		pkgPath:  pkgPath,
	}
}

var _ Type = (*InterfaceType)(nil)

func (i *InterfaceType) AddStructure(name string, o *ObjectType) {
	if i.object == nil {
		i.object = make(map[string]*ObjectType)
	}
	i.object[name] = o
}

func (b *InterfaceType) GetStructure() map[string]*ObjectType {
	return b.object
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

func (i *InterfaceType) AddFatherInterfaceType(parent *InterfaceType) {
	i.parents = append(i.parents, parent)
	parent.AddChildInterfaceType(i)
}

func (i *InterfaceType) AddChildInterfaceType(child *InterfaceType) {
	i.childs = append(i.childs, child)
}

func (i *InterfaceType) AddFullTypeName(name string) {
	if i == nil {
		return
	}
	i.name = name
}

func (i *InterfaceType) GetFullTypeNames() []string {
	if i == nil {
		return nil
	}
	return i.fullTypeName
}

func (i *InterfaceType) SetFullTypeNames(names []string) {
	if i == nil {
		return
	}
	i.fullTypeName = clean(names)
}

// ====================== chan type
type ChanType struct {
	*baseType
	Elem         Type
	fullTypeName []string
}

var _ (Type) = (*ChanType)(nil)

func (c *ChanType) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	if !lo.Contains(c.fullTypeName, name) {
		c.fullTypeName = append(c.fullTypeName, name)
	}
}

func (c *ChanType) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *ChanType) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = clean(names)
}

// func (b *ChanType) GetAllKey() []string {
// 	return lo.Keys(b.method)
// }

func (c *ChanType) GetTypeKind() TypeKind {
	return ChanTypeKind
}

func NewChanType(elem Type) *ChanType {
	return &ChanType{
		baseType: NewBaseType(),
		Elem:     elem,
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
	*baseType
	Name        string
	VerboseName string
	pkgPath     string
	Kind        TypeKind
	Len         int
	Keys        []Value
	keymap      map[string]int // remove duplicate key
	keyTypes    []Type
	FieldTypes  []Type

	AnonymousField map[string]*ObjectType

	Combination bool // function multiple return will combined to struct
	// VariadicPara bool // function last variadic parameter will become slice

	KeyTyp    Type
	FieldType Type

	fullTypeName []string
}

var _ (Type) = (*ObjectType)(nil)

func (i *ObjectType) GetTypeKind() TypeKind {
	return i.Kind
}

func (i *ObjectType) SetTypeKind(t TypeKind) {
	i.Kind = t
}

func (i *ObjectType) AddFullTypeName(name string) {
	if i == nil {
		return
	}
	if !lo.Contains(i.fullTypeName, name) {
		i.fullTypeName = append(i.fullTypeName, name)
	}
}

func (i *ObjectType) GetFullTypeNames() []string {
	if i == nil {
		return nil
	}
	return i.fullTypeName
}

func (i *ObjectType) SetFullTypeNames(names []string) {
	if i == nil {
		return
	}
	i.fullTypeName = clean(names)
}

func (i *ObjectType) SetName(name string) {
	i.Name = name
}

func (i *ObjectType) SetPkgPath(pkg string) {
	i.pkgPath = pkg
}

func (i *ObjectType) GetKeybyName(key string) Value {
	if index, ok := i.keymap[key]; ok {
		return i.Keys[index]
	}
	return nil
}

func NewObjectType() *ObjectType {
	return &ObjectType{
		baseType:   NewBaseType(),
		Kind:       ObjectTypeKind,
		Keys:       make([]Value, 0),
		keymap:     make(map[string]int),
		keyTypes:   make([]Type, 0),
		FieldTypes: make([]Type, 0),

		fullTypeName:   []string{},
		AnonymousField: map[string]*ObjectType{},
	}
}

// for slice build
func NewSliceType(elem Type) *ObjectType {
	i := NewObjectType()
	i.Kind = SliceTypeKind
	i.KeyTyp = CreateNumberType()
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

func NewPointerType() *ObjectType {
	i := NewObjectType()
	i.Kind = PointerKind
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
	itype.Name = "..." // avoid RawString -> String -> RawString loop
	ret := itype.RawString()
	itype.Name = ""
	return ret
}

func (i *ObjectType) PkgPathString() string {
	result := i.pkgPath
	if result == "" {
		result = i.RawString()
	}
	return result
}

func (itype *ObjectType) RawString() string {
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
			keyTyp = CreateAnyType()
		}
		fieldType := itype.FieldType
		if utils.IsNil(fieldType) {
			fieldType = CreateAnyType()
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
	return ret
}

// for struct build
func (s *ObjectType) AddField(key Value, field Type) {
	keyTyp := key.GetType()
	if field == nil {
		field = CreateAnyType()
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
		s.FieldType = CreateAnyType()
	}
	if len(keyTypes) == 1 {
		s.KeyTyp = keyTypes[0]
	} else {
		s.KeyTyp = CreateAnyType()
	}
}

type FunctionType struct {
	*baseType
	Name            string
	pkgPath         string
	This            *Function
	ReturnType      Type
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

	AnnotationFunc    []func(Value)
	fullTypeName      []string
	isAnyFunctionType bool
}

var _ Type = (*FunctionType)(nil)

func (f *FunctionType) SetIsMethod(isMethod bool, obj Type) {
	f.IsMethod = isMethod
	f.ObjectType = obj
}

func (f *FunctionType) AddFullTypeName(name string) {
	if f == nil {
		return
	}
	if !lo.Contains(f.fullTypeName, name) {
		f.fullTypeName = append(f.fullTypeName, name)
	}
}

func (f *FunctionType) GetFullTypeNames() []string {
	if f == nil {
		return nil
	}
	return f.fullTypeName
}

func (f *FunctionType) SetFullTypeNames(names []string) {
	if f == nil {
		return
	}
	f.fullTypeName = clean(names)
}

func (f *FunctionType) SetModifySelf(b bool) { f.IsModifySelf = b }

func CalculateType(ts []Type) Type {
	if len(ts) == 0 {
		return CreateNullType()
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

func NewAnyFunctionType() *FunctionType {
	return &FunctionType{
		baseType:          NewBaseType(),
		isAnyFunctionType: true,
	}
}

func NewFunctionType(name string, Parameter []Type, ReturnType Type, IsVariadic bool) *FunctionType {
	f := &FunctionType{
		baseType:     NewBaseType(),
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
	s.Name = "..." // avoid RawString -> String -> RawString loop
	ret := s.RawString()
	s.Name = ""
	return ret
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
	returnTypeStr := ""
	if s.ReturnType != nil {
		returnTypeStr = s.ReturnType.String()
	}

	return fmt.Sprintf(
		"(%s%s) -> %s",
		strings.Join(
			paras,
			", ",
		),
		variadic,
		returnTypeStr,
	)
}

func (s *FunctionType) IsAnyFunctionType() bool {
	return s.isAnyFunctionType
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
	*baseType
	symbol       string
	fullTypeName []string
}

var _ (Type) = (*GenericType)(nil)

func (c *GenericType) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	if !lo.Contains(c.fullTypeName, name) {
		c.fullTypeName = append(c.fullTypeName, name)
	}
}

func (c *GenericType) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *GenericType) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = clean(names)
}

func (c *GenericType) GetTypeKind() TypeKind {
	return GenericTypeKind
}

var (
	// T is a generic type
	TypeT = NewGenericType("T")
	TypeU = NewGenericType("U")
	TypeK = NewGenericType("K")
	TypeV = NewGenericType("V")
)

func NewGenericType(symbol string) *GenericType {
	return &GenericType{
		baseType: NewBaseType(),
		symbol:   symbol,
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

func isGenericType(t Type) bool {
	return t.GetTypeKind() == GenericTypeKind
}

func GetGenericTypeFromType(t Type) []Type {
	typs := make([]Type, 0)
	switch t.GetTypeKind() {
	case AliasTypeKind:
		alias, _ := ToAliasType(t)
		typs = append(typs, GetGenericTypeFromType(alias.GetType())...)
	case OrTypeKind:
		for _, typ := range t.(*OrType).types {
			typs = append(typs, GetGenericTypeFromType(typ)...)
		}
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
		if obj.IsAnyFunctionType() {
			typs = append(typs, obj)
			break
		}
		for _, typ := range obj.Parameter {
			typs = append(typs, GetGenericTypeFromType(typ)...)
		}
		typs = append(typs, GetGenericTypeFromType(obj.ReturnType)...)
	}
	return typs
}

// Deep copy
func CloneType(t Type) (Type, bool) {
	switch t.GetTypeKind() {
	case GenericTypeKind,
		StringTypeKind, NumberTypeKind, BooleanTypeKind, ByteTypeKind, BytesTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		return t, true
	case ChanTypeKind:
		old := t.(*ChanType)
		clonedElem, ok := CloneType(old.Elem)
		if !ok {
			return nil, false
		}
		return NewChanType(clonedElem), true
	case SliceTypeKind:
		old := t.(*ObjectType)
		clonedField, ok := CloneType(old.FieldType)
		if !ok {
			return nil, false
		}
		return NewSliceType(clonedField), true
	case MapTypeKind:
		old := t.(*ObjectType)
		clonedKey, ok := CloneType(old.KeyTyp)
		if !ok {
			return nil, false
		}
		clonedField, ok := CloneType(old.FieldType)
		if !ok {
			return nil, false
		}
		return NewMapType(clonedKey, clonedField), true
	case TupleTypeKind:
		old := t.(*ObjectType)
		clonedSlices := make([]Type, 0, len(old.FieldTypes))
		for _, t := range old.FieldTypes {
			cloned, ok := CloneType(t)
			if !ok {
				return nil, false
			}
			clonedSlices = append(clonedSlices, cloned)
		}
		return CalculateType(clonedSlices), true
	case FunctionTypeKind:
		old := t.(*FunctionType)
		clonedParameter := make([]Type, 0, len(old.Parameter))
		for _, t := range old.Parameter {
			cloned, ok := CloneType(t)
			if !ok {
				return nil, false
			}
			clonedParameter = append(clonedParameter, cloned)
		}
		clonedReturn, ok := CloneType(old.ReturnType)
		if !ok {
			return nil, false
		}
		return NewFunctionType(old.Name, clonedParameter, clonedReturn, old.IsVariadic), true
	case OrTypeKind:
		old := t.(*OrType)
		clonedTypes := make([]Type, 0, len(old.types))
		for _, typ := range old.types {
			if _, ok := CloneType(typ); !ok {
				return nil, false
			}
			clonedTypes = append(clonedTypes, typ)
		}
		return NewOrType(clonedTypes...), true
	case AliasTypeKind:
		alias := t.(*AliasType)
		clonedElem, ok := CloneType(alias.GetType())
		if !ok {
			return nil, false
		}
		return NewAliasType(alias.Name, alias.PkgPathString(), clonedElem), true
	}
	return nil, false
}

// ====================== or type
type OrType struct {
	*baseType
	types        Types
	fullTypeName []string
}

var _ (Type) = (*OrType)(nil)

func (c *OrType) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *OrType) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = clean(names)
}

func (c *OrType) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	if !lo.Contains(c.fullTypeName, name) {
		c.fullTypeName = append(c.fullTypeName, name)
	}
}

func (c *OrType) GetTypeKind() TypeKind {
	// var typ Type

	// var checkAllTypes func(*OrType) bool
	// checkAllTypes = func(or *OrType) bool {
	// 	for _, t := range or.types {
	// 		switch t := t.(type) {
	// 		case *OrType:
	// 			checkAllTypes(t)
	// 		default:
	// 			if typ == nil {
	// 				typ = t
	// 				continue
	// 			}
	// 			if typ.GetTypeKind() != t.GetTypeKind() {
	// 				return false
	// 			}
	// 		}
	// 	}
	// 	return true
	// }
	// if checkAllTypes(c) {
	// 	return typ.GetTypeKind()
	// } else {
	return OrTypeKind
	// }
}

func (c *OrType) GetTypes() Types {
	var rets Types
	types := make(map[Type]bool)

	var checkAllTypes func(*OrType)
	checkAllTypes = func(or *OrType) {
		for _, t := range or.types {
			switch t := t.(type) {
			case *OrType:
				checkAllTypes(t)
			default:
				if _, ok := types[t]; ok {
					return
				}
				types[t] = true
			}
		}
	}
	checkAllTypes(c)
	for t, _ := range types {
		rets = append(rets, t)
	}

	return rets
	// return c.types
}

func NewOrType(types ...Type) Type {
	if len(types) == 1 {
		return types[0]
	}
	return &OrType{
		baseType: NewBaseType(),
		types:    Types(types),
	}
}

func (c OrType) String() string {
	return strings.Join(lo.Map(c.types, func(t Type, _ int) string { return t.String() }), "|")
}

func (c OrType) PkgPathString() string {
	return strings.Join(lo.Map(c.types, func(t Type, _ int) string { return t.PkgPathString() }), "|")
}

func (c OrType) RawString() string {
	return strings.Join(lo.Map(c.types, func(t Type, _ int) string { return t.RawString() }), "|")
}
