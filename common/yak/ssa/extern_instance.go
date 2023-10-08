package ssa

import (
	"fmt"
	"reflect"
	"strings"
)

const (
	MAXTYPELEVEL = 5
)

func IsExternInstanc(v Value) bool {
	if m, ok := v.(*Make); ok && m.buildField != nil {
		return true
	} else {
		return false
	}
}

func (b *FunctionBuilder) WithExternInstance(vs map[string]any) {
	b.buildExtern = func(id string, builder *FunctionBuilder) Value {
		if v, ok := vs[id]; ok {
			return builder.BuildValueFromAny("", id, v)
		}
		return nil
	}
}

func (b *FunctionBuilder) TryBuildExternInstance(id string) Value {
	if b.buildExtern == nil {
		return nil
	}
	return b.buildExtern(id, b)
}

func (b *FunctionBuilder) BuildValueFromAny(libname, id string, v any) (value Value) {
	if value, ok := b.externInstance[libname+id]; ok {
		return value
	}

	itype := reflect.TypeOf(v)
	// ivalue := reflect.ValueOf(v)

	if itype == reflect.TypeOf(make(map[string]interface{})) {
		i := NewMake(nil,
			NewMapType(BasicTypes[String], BasicTypes[Any]),
			nil, nil, nil, nil, nil, b.CurrentBlock)

		vs := v.(map[string]interface{})
		i.buildField = func(key string) Value {
			if v, ok := vs[key]; ok {
				return b.BuildValueFromAny(id, key, v)
			}
			return nil
		}
		str := id
		if libname != "" {
			str = libname + "." + id
		}
		i.SetVariable(str)
		b.externInstance[str] = i
		return i
	} else {
		if itype == nil {
			return nil
		}

		if strings.HasPrefix(id, "$") || strings.HasPrefix(id, "_") {
			return nil
		}

		str := id
		if libname != "" {
			str = libname + "." + id
		}
		switch itype.Kind() {
		case reflect.Func:
			value = NewFunctionWithType(str, b.CoverReflectFunctionType(itype))
		default:
			value = NewParam(str, false, b.Function)
			value.SetType(b.handlerType(itype, 0))
		}
		b.externInstance[str] = value
		return
	}
}

func (f *FunctionBuilder) CoverReflectFunctionType(itype reflect.Type) *FunctionType {
	params := make([]Type, 0)
	returns := make([]Type, 0)
	hasEllipsis := itype.IsVariadic()
	// parameter
	for i := 0; i < itype.NumIn(); i++ {
		params = append(params, f.handlerType(itype.In(i), 0))
	}
	// return
	for i := 0; i < itype.NumOut(); i++ {
		returns = append(returns, f.handlerType(itype.Out(i), 0))
	}
	return NewFunctionType(itype.String(), params, returns, hasEllipsis)
}

func (f *FunctionBuilder) handlerType(typ reflect.Type, level int) Type {
	if level >= MAXTYPELEVEL {
		return NewObjectType()
	} else {
		level += 1
	}
	typStr := typ.String()
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

	// complex type
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		ret = NewSliceType(f.handlerType(typ.Elem(), level))
	case reflect.Map:
		ret = NewMapType(f.handlerType(typ.Key(), level), f.handlerType(typ.Elem(), level))
	case reflect.Struct:
		structType := NewStructType()
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
		ret = f.CoverReflectFunctionType(typ)
	case reflect.Pointer:
		ret = f.handlerType(typ.Elem(), level)
		return ret
	case reflect.UnsafePointer:
		obj := NewObjectType()
		obj.SetName(typStr)
		ret = obj
	case reflect.Interface:
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
	handlerMethod := func(typ reflect.Type, i int) {
		method := typ.Method(i)
		funTyp := f.CoverReflectFunctionType(method.Type)
		funTyp.SetName(method.Name)
		Methods[method.Name] = funTyp
	}

	for i := 0; i < typ.NumMethod(); i++ {
		handlerMethod(typ, i)
	}
	for i := 0; i < pTyp.NumMethod(); i++ {
		handlerMethod(pTyp, i)
	}
	ret.SetMethod(Methods)
	return ret
}
