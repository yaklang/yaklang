package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	MAXTYPELEVEL = 15
)

func (b *FunctionBuilder) WithExternValue(vs map[string]any) {
	if vs == nil {
		return
	}
	b.buildExternValue = func(id string, builder *FunctionBuilder) Value {
		if v, ok := builder.externInstance[id]; ok {
			return v
		}
		if v, ok := vs[id]; ok {
			return builder.BuildValueFromAny(id, v)
		}
		return nil
	}
}

func (b *FunctionBuilder) WithExternLib(lib map[string]map[string]any) {
	b.buildExternLib = func(name string, builder *FunctionBuilder) func(string) Value {
		if table, ok := lib[name]; ok {
			return func(key string) Value {
				if v, ok := table[key]; ok {
					return builder.BuildValueFromAny(name+"."+key, v)
				} else {
					return nil
				}
			}
		}
		return nil
	}
}

func (b *FunctionBuilder) TryBuildExternValue(id string) Value {
	if b.buildExternValue == nil {
		return nil
	}
	return b.buildExternValue(id, b)
}

func (b *FunctionBuilder) TryBuildExternLib(name string) func(string) Value {
	if b.buildExternLib == nil {
		return nil
	}
	return b.buildExternLib(name, b)
}

func (b *FunctionBuilder) BuildValueFromAny(id string, v any) (value Value) {
	if value, ok := b.externInstance[id]; ok {
		return value
	}

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
		f.SetPosition(b.CurrentPos)
		value = f
	default:
		value = NewParam(str, false, b.Function)
		value.SetType(b.handlerType(itype, 0))
	}
	b.externInstance[str] = value
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
	if isVariadic {
		if t, ok := params[len(params)-1].(*ObjectType); ok {
			t.VariadicPara = true
		} else {
			// error
		}
	}
	return NewFunctionType(itype.String(), params, returns, isVariadic)
}

func (f *FunctionBuilder) handlerType(typ reflect.Type, level int) Type {
	if level >= MAXTYPELEVEL {
		return NewObjectType()
	} else {
		level += 1
	}
	typStr := typ.String()
	if typStr == "[]uint8" {
		typStr = "bytes"
	}
	if hijackType, ok := f.externType[typStr]; ok {
		return hijackType
	}

	// base type
	if t := GetTypeByStr(typStr); t != nil {
		return t
	}

	var ret Type

	// alias type
	if t := GetTypeByStr(typ.Kind().String()); t != nil {
		ret = NewAliasType(typStr, t)
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
		f.externType[typStr] = structType
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fieldType := f.handlerType(field.Type, level)
			structType.AddField(NewConst(field.Name), fieldType)
			if field.Anonymous && fieldType.GetTypeKind() == ObjectTypeKind {
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
		obj.SetName(typStr)
		ret = obj
	case reflect.Interface:
		isInterface = true
		ret = NewInterfaceType(typStr)
	case reflect.Chan:
		ret = NewChanType(f.handlerType(typ.Elem(), level))
	default:
		if ret == nil {
			fmt.Println("con't handler this type:" + typ.Kind().String())
			ret = NewObjectType()
		}
	}

	if ret != nil {
		f.externType[typStr] = ret
		if ityp, ok := ret.(*ObjectType); ok {
			ityp.SetName(typStr)
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
			funTyp.SetName(method.Name)
			Methods[method.Name] = funTyp
		}
	}

	handlerMethod(typ)
	handlerMethod(pTyp)
	ret.SetMethod(Methods)
	return ret
}
