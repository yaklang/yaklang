package ssa

import (
	"reflect"
	"strings"
)

func (b *FunctionBuilder) WithUndefineHijack(vs map[string]any) {
	b.undefineHijack = func(id string) Value {
		if v, ok := vs[id]; ok {
			return b.BuildValueFromAny("", id, v)
		}
		return nil
	}
}

func (b *FunctionBuilder) UndefineHijack(id string) Value {
	if b.undefineHijack == nil {
		return nil
	}
	return b.undefineHijack(id)
}

func (b *FunctionBuilder) BuildValueFromAny(libname, id string, v any) (value Value) {
	if value, ok := b.externInstance[libname+id]; ok {
		return value
	}

	itype := reflect.TypeOf(v)
	// ivalue := reflect.ValueOf(v)

	if itype == reflect.TypeOf(make(map[string]interface{})) {
		i := NewInterface(nil,
			NewMapType(BasicTypes[String], BasicTypes[Any]),
			nil, nil, nil, nil, nil, b.CurrentBlock)

		vs := v.(map[string]interface{})
		i.buildField = func(key string) Value {
			if v, ok := vs[key]; ok {
				return b.BuildValueFromAny(id, key, v)
			}
			return nil
		}
		i.SetVariable(id)
		b.externInstance[libname+id] = i
		return i
	} else {
		if itype == nil {
			return nil
		}

		if strings.HasPrefix(id, "$") || strings.HasPrefix(id, "_") {
			return nil
		}

		switch itype.Kind() {
		case reflect.Func:
			value = NewFunctionWithType(id, b.CoverReflectFunctionType(itype))
			b.externInstance[libname+id] = value
			return
		default:
		}
	}
	return nil
}

func (f *FunctionBuilder) CoverReflectFunctionType(itype reflect.Type) *FunctionType {
	params := make([]Type, 0)
	returns := make([]Type, 0)
	hasEllipsis := itype.IsVariadic()
	// parameter
	for i := 0; i < itype.NumIn(); i++ {
		params = append(params, f.handlerType(itype.In(i)))
	}
	// return
	for i := 0; i < itype.NumOut(); i++ {
		returns = append(returns, f.handlerType(itype.Out(i)))
	}
	return NewFunctionType(itype.String(), params, returns, hasEllipsis)
}

func (f *FunctionBuilder) handlerType(typ reflect.Type) Type {
	typStr := typ.String()
	if hijackType, ok := f.externType[typStr]; ok {
		return hijackType
	}

	if t := GetTypeByStr(typStr); t != nil {
		return t
	}

	var ret Type
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		ret = NewSliceType(f.handlerType(typ.Elem()))
	case reflect.Map:
		ret = NewMapType(f.handlerType(typ.Key()), f.handlerType(typ.Elem()))
	case reflect.Struct:
		structType := NewInterfaceType()
		for i := 0; i < typ.NumField(); i++ {
			structType.AddField(NewConst(typ.Field(i).Name), f.handlerType(typ.Field(i).Type))
		}
		structType.Finish()
		ret = structType
	case reflect.Func:
		ret = f.CoverReflectFunctionType(typ)
	case reflect.Pointer, reflect.UnsafePointer:
		ret = f.handlerType(typ.Elem())
	default:
		panic("con't handler this type:" + typStr)
	}

	if ret != nil {
		f.externType[typStr] = ret
		if ityp, ok := ret.(*InterfaceType); ok {
			ityp.SetName(typStr)
		}
	}

	return ret
}
