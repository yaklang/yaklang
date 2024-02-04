package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	MAXTYPELEVEL = 15
)

func (b *FunctionBuilder) WithExternValue(vs map[string]any) {
	b.ExternInstance = vs
}

func (b *FunctionBuilder) WithExternLib(lib map[string]map[string]any) {
	b.ExternLib = lib
}

func (b *FunctionBuilder) WithExternMethod(builder MethodBuilder) {
	ExternMethodBuilder = builder
}

func (b *FunctionBuilder) WithDefineFunction(defineFunc map[string]any) {
	b.DefineFunc = defineFunc
}

func TryGetSimilarityKey(table []string, name string) string {
	var score float64
	var ret string
	for _, libKey := range table {
		if strings.EqualFold(libKey, name) {
			// if strings.ToLower(libKey) == strings.ToLower(name) {
			return libKey
		}
		s := utils.CalcSimilarity(utils.UnsafeStringToBytes(name), utils.UnsafeStringToBytes(libKey))
		if score < s {
			score = s
			ret = libKey
		}
	}
	return ret
}

func (b *FunctionBuilder) TryGetSimilarityKey(name, key string) string {
	if b.ExternLib == nil {
		return ""
	}
	var ret string
	if table, ok := b.ExternLib[name]; ok {
		ret = TryGetSimilarityKey(lo.Keys(table), key)
	}
	return ret
}

func (ex *ExternLib) BuildField(key string) Value {
	if ret, ok := ex.MemberMap[key]; ok {
		return ret
	}
	b := ex.builder
	if v, ok := ex.table[key]; ok {
		v := b.BuildValueFromAny(ex.GetName()+"."+key, v)
		ex.MemberMap[key] = v
		ex.Member = append(ex.Member, v)
		return v
	}
	return nil
}

func (b *FunctionBuilder) TryBuildExternValue(id string) Value {
	getExternValue := func(id string) Value {
		if v, ok := b.cacheExternInstance[id]; ok {
			return v
		}

		if b.ExternInstance == nil {
			return nil
		}
		v, ok := b.ExternInstance[id]
		if !ok {
			return nil
		}
		ret := b.BuildValueFromAny(id, v)
		b.cacheExternInstance[id] = ret
		return ret
	}

	getExternLib := func(id string) *ExternLib {
		if v, ok := b.cacheExternInstance[id]; ok {
			if ex, ok := v.(*ExternLib); ok {
				return ex
			}
			return nil
		}

		if b.ExternLib == nil {
			return nil
		}
		table, ok := b.ExternLib[id]
		if !ok {
			return nil
		}
		ex := NewExternLib(id, b, table)
		ex.SetExtern(true)
		b.cacheExternInstance[id] = ex
		return ex
	}

	getExternInstance := func(id string) Value {
		if ret := getExternValue(id); ret != nil {
			return ret
		}
		if ret := getExternLib(id); ret != nil {
			return ret
		}
		return nil
	}

	getExternField := func(lib string, key string) Value {
		if v, ok := b.cacheExternInstance[id]; ok {
			return v
		}

		extern := getExternLib(lib)
		if extern == nil {
			return nil
		}
		ret := extern.BuildField(key)
		if ret != nil {
			b.cacheExternInstance[id] = ret
		}
		return ret
	}

	if str := strings.Split(id, "."); len(str) == 1 {
		return getExternInstance(id)
	} else if len(str) == 2 {
		return getExternField(str[0], str[1])
	} else {
		return nil
	}
}

func (b *FunctionBuilder) BuildValueFromAny(id string, v any) (value Value) {

	itype := reflect.TypeOf(v)
	if itype == nil {
		return nil
	}

	if strings.HasPrefix(id, "$") || strings.HasPrefix(id, "_") {
		return nil
	}

	str := id
	switch itype.Kind() {
	case reflect.Func:
		f := NewFunctionWithType(str, b.CoverReflectFunctionType(itype, 0))
		f.SetRange(b.CurrentRange)
		value = f
	default:
		value = NewParam(str, false, b)
		value.SetType(b.handlerType(itype, 0))
	}
	value.SetExtern(true)
	b.GetProgram().SetInstructionWithName(str, value)
	return
}

func (f *FunctionBuilder) CoverReflectFunctionType(itype reflect.Type, level int) *FunctionType {
	params := make([]Type, 0)
	returns := make([]Type, 0)
	isVariadic := itype.IsVariadic()
	// parameter
	for i := 0; i < itype.NumIn(); i++ {
		params = append(params, f.handlerType(itype.In(i), level))
	}
	// return
	for i := 0; i < itype.NumOut(); i++ {
		returns = append(returns, f.handlerType(itype.Out(i), level))
	}
	return NewFunctionTypeDefine(itype.String(), params, returns, isVariadic)
}

func (f *FunctionBuilder) handlerType(typ reflect.Type, level int) Type {
	if level >= MAXTYPELEVEL {
		return NewObjectType()
	} else {
		level += 1
	}
	Name := typ.String()
	if Name == "[]uint8" {
		Name = "bytes"
	}

	// base type
	if t := GetTypeByStr(Name); t != nil {
		return t
	}

	var PkgPath string
	typKind := typ.Kind()
	if typKind == reflect.Struct || typKind == reflect.Interface {
		pkg := typ.PkgPath()
		name := typ.Name()
		PkgPath = fmt.Sprintf("%s.%s", pkg, name)
	} else if typKind == reflect.Ptr {
		pkg := typ.Elem().PkgPath()
		name := typ.Elem().Name()
		PkgPath = fmt.Sprintf("%s.%s", pkg, name)
	}

	if hijackType, ok := f.externType[Name]; ok {
		return hijackType
	}

	var ret Type

	// alias type
	if t := GetTypeByStr(typ.Kind().String()); t != nil {
		ret = NewAliasType(Name, PkgPath, t)
	}

	isInterface := false
	// complex type
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		ret = NewSliceType(f.handlerType(typ.Elem(), level))
	case reflect.Map:
		ret = NewMapType(f.handlerType(typ.Key(), level), f.handlerType(typ.Elem(), level))
	case reflect.Struct:
		structType := NewStructType()
		structType.Name = Name
		structType.pkgPath = PkgPath
		f.externType[Name] = structType
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fieldType := f.handlerType(field.Type, level)
			structType.AddField(NewConst(field.Name), fieldType)
			if field.Anonymous && IsObjectType(fieldType) {
				structType.AnonymousField = append(structType.AnonymousField, fieldType.(*ObjectType))
			}
		}
		structType.Finish()
		ret = structType
	case reflect.Func:
		ret = f.CoverReflectFunctionType(typ, level)
	case reflect.Pointer:
		ret = f.handlerType(typ.Elem(), level)
		return ret
	case reflect.UnsafePointer:
		obj := NewObjectType()
		obj.SetName(Name)
		ret = obj
	case reflect.Interface:
		isInterface = true
		ret = NewInterfaceType(Name, PkgPath)
	case reflect.Chan:
		ret = NewChanType(f.handlerType(typ.Elem(), level))
	default:
		if ret == nil {
			fmt.Println("cannot handler this type:" + typ.Kind().String())
			ret = NewObjectType()
		}
	}

	if ret != nil {
		f.externType[Name] = ret
		if ityp, ok := ret.(*ObjectType); ok {
			ityp.SetName(Name)
		}
	}

	// handler method
	pTyp := reflect.PointerTo(typ)
	Methods := make(map[string]*FunctionType, typ.NumMethod()+pTyp.NumMethod())
	handlerMethod := func(typ reflect.Type) {
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			funTyp := f.CoverReflectFunctionType(method.Type, level)
			if isInterface {
				funTyp.Parameter = utils.InsertSliceItem(funTyp.Parameter, ret, 0)
			}
			funTyp.SetName(fmt.Sprintf("%s.%s", PkgPath, method.Name))
			// funTyp.SetName(PkgPath)
			Methods[method.Name] = funTyp
		}
	}

	handlerMethod(typ)
	handlerMethod(pTyp)
	ret.SetMethod(Methods)
	return ret
}
