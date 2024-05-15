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
	b.GetProgram().ExternInstance = vs
}

func (b *FunctionBuilder) WithExternLib(lib map[string]map[string]any) {
	b.GetProgram().ExternLib = lib
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

func (prog *Program) TryGetSimilarityKey(name, key string) string {
	if prog.ExternLib == nil {
		return ""
	}
	var ret string
	if table, ok := prog.ExternLib[name]; ok {
		ret = TryGetSimilarityKey(lo.Keys(table), key)
	}
	return ret
}

func (prog *Program) TryBuildExternValue(b *FunctionBuilder, id string) Value {
	getExternValue := func(id string) Value {
		if v, ok := prog.cacheExternInstance[id]; ok {
			return v
		}

		if prog.ExternInstance == nil {
			return nil
		}
		v, ok := prog.ExternInstance[id]
		if !ok {
			return nil
		}
		ret := prog.BuildValueFromAny(b, id, v)
		prog.cacheExternInstance[id] = ret
		return ret
	}

	getExternLib := func(id string) *ExternLib {
		if v, ok := prog.cacheExternInstance[id]; ok {
			if ex, ok := v.(*ExternLib); ok {
				return ex
			}
			return nil
		}

		if prog.ExternLib == nil {
			return nil
		}
		table, ok := prog.ExternLib[id]
		if !ok {
			return nil
		}
		ex := NewExternLib(id, b, table)
		ex.SetExtern(true)
		prog.cacheExternInstance[id] = ex
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
		if v, ok := prog.cacheExternInstance[id]; ok {
			return v
		}

		extern := getExternLib(lib)
		if extern == nil {
			return nil
		}
		ret := extern.BuildField(key)
		if ret != nil {
			prog.cacheExternInstance[id] = ret
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

func (prog *Program) BuildValueFromAny(b *FunctionBuilder, id string, v any) (value Value) {
	itype := reflect.TypeOf(v)
	if itype == nil {
		return nil
	}
	str := id
	switch itype.Kind() {
	case reflect.Func:
		f := NewFunctionWithType(str, prog.CoverReflectFunctionType(itype, 0))
		f.SetRange(b.CurrentRange)
		value = f
	default:
		value = NewParam(str, false, b)
		value.SetType(prog.handlerType(itype, 0))
	}
	value.SetExtern(true)
	prog.SetVirtualRegister(value)
	prog.SetInstructionWithName(str, value)
	return
}

func (prog *Program) CoverReflectFunctionType(itype reflect.Type, level int) *FunctionType {
	params := make([]Type, 0)
	returns := make([]Type, 0)
	isVariadic := itype.IsVariadic()
	// parameter
	for i := 0; i < itype.NumIn(); i++ {
		params = append(params, prog.handlerType(itype.In(i), level))
	}
	// return
	for i := 0; i < itype.NumOut(); i++ {
		returns = append(returns, prog.handlerType(itype.Out(i), level))
	}
	return NewFunctionTypeDefine(itype.String(), params, returns, isVariadic)
}

func (prog *Program) handlerType(typ reflect.Type, level int) Type {
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

	if hijackType, ok := prog.externType[Name]; ok {
		return hijackType
	}

	var ret Type

	// alias type
	if t := GetTypeByStr(typ.Kind().String()); t != nil {
		ret = NewAliasType(Name, PkgPath, t)
	}

	// before this check, code will not recursive.
	// check level
	if level >= MAXTYPELEVEL {
		return GetAnyType()
	}
	level++
	// below this code, will recursive

	isInterface := false
	// complex type
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		ret = NewSliceType(prog.handlerType(typ.Elem(), level))
	case reflect.Map:
		ret = NewMapType(prog.handlerType(typ.Key(), level), prog.handlerType(typ.Elem(), level))
	case reflect.Struct:
		structType := NewStructType()
		structType.Name = Name
		structType.pkgPath = PkgPath
		prog.externType[Name] = structType
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fieldType := prog.handlerType(field.Type, level)
			structType.AddField(NewConst(field.Name), fieldType)
			if field.Anonymous && IsObjectType(fieldType) {
				structType.AnonymousField = append(structType.AnonymousField, fieldType.(*ObjectType))
			}
		}
		structType.Finish()
		ret = structType
	case reflect.Func:
		ret = prog.CoverReflectFunctionType(typ, level)
	case reflect.Pointer:
		ret = prog.handlerType(typ.Elem(), level)
		return ret
	case reflect.UnsafePointer:
		obj := NewObjectType()
		obj.SetName(Name)
		ret = obj
	case reflect.Interface:
		isInterface = true
		ret = NewInterfaceType(Name, PkgPath)
	case reflect.Chan:
		ret = NewChanType(prog.handlerType(typ.Elem(), level))
	default:
		if ret == nil {
			fmt.Println("cannot handler this type:" + typ.Kind().String())
			ret = NewObjectType()
		}
	}

	if ret != nil {
		prog.externType[Name] = ret
		if ityp, ok := ret.(*ObjectType); ok {
			ityp.SetName(Name)
		}
	}

	// handler method
	pTyp := reflect.PointerTo(typ)
	Methods := make(map[string]*Function, typ.NumMethod()+pTyp.NumMethod())
	handlerMethod := func(typ reflect.Type) {
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			funTyp := prog.CoverReflectFunctionType(method.Type, level)
			if isInterface {
				funTyp.Parameter = utils.InsertSliceItem(funTyp.Parameter, ret, 0)
			}
			funTyp.SetName(fmt.Sprintf("%s.%s", PkgPath, method.Name))
			Methods[method.Name] = NewFunctionWithType(method.Name, funTyp)
		}
	}

	handlerMethod(typ)
	handlerMethod(pTyp)
	ret.SetMethod(Methods)
	return ret
}

func (b *FunctionBuilder) TryGetSimilarityKey(name, key string) string {
	return b.GetProgram().TryGetSimilarityKey(name, key)
}

func (b *FunctionBuilder) TryBuildExternValue(id string) Value {
	return b.GetProgram().TryBuildExternValue(b, id)
}

func (b *FunctionBuilder) BuildValueFromAny(id string, v any) (value Value) {
	return b.GetProgram().BuildValueFromAny(b, id, v)
}

func (b *FunctionBuilder) CoverReflectFunctionType(itype reflect.Type, level int) *FunctionType {
	return b.GetProgram().CoverReflectFunctionType(itype, level)
}

func (b *FunctionBuilder) handlerType(typ reflect.Type, level int) Type {
	return b.GetProgram().handlerType(typ, level)
}
