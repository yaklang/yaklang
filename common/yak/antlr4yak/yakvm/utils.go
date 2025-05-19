package yakvm

import (
	"reflect"
	"unsafe"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor/nasl_type"
)

func GetNaslValueBySymbolId(symbol int, frame *Frame) *Value {
	id := symbol
	//table := frame.vm.globalVar["__nasl_global_var_table"].(map[int]*Value)
	table, err := frame.vm.GetNaslGlobalVarTable()
	if err != nil {
		log.Error(err)
		return GetUndefined()
	}
	if val, ok := table[id]; ok {
		return val
	}

	name, ok := frame.CurrentScope().GetSymTable().GetNameByVariableId(id)
	if ok && name == "_FCT_ANON_ARGS" {
		if val, ok := frame.contextData["argument"]; ok {
			return NewAutoValue(val.(*nasl_type.NaslArray))
		}
	}
	// 尝试在作用域获取值
	val, ok := frame.CurrentScope().GetValueByID(id)
	if !ok {
		name, ok1 := frame.CurrentScope().GetSymTable().GetNameByVariableId(id)
		if ok1 {
			// 使用名字在全局变量中查找
			if v1, ok1 := frame.GlobalVariables.Load(name); ok1 {
				val = NewValue("function", v1, name)
				ok = true
			} else if v1, ok2 := frame.CurrentScope().GetValueByName(name + "s"); ok2 && v1.IsYakFunction() {
				v1.AddExtraInfo("getOne", true)
				val = v1
				ok = true
			} else {
				if frame.CurrentScope().GetSymTable().IdIsInited(id) {
					val = GetUndefined()
					ok = true
				}
			}
		}
		if !ok {
			return GetUndefined()
			//panic("cannot found value by variable name:[" + name + "]")
		}
	} else {
		val1 := *val // nasl里函数参数和形参名是绑定的，这里需要拷贝一份
		val = &val1
	}
	if !ok {
		return GetUndefined()
		//panic("BUG: cannot found value by symbol:[" + fmt.Sprint(id) + "]")
	}
	if val.Value == nil {
		val = NewUndefined(id)
	}
	val.SymbolId = id
	return val
}

func IsDefinedType(t reflect.Type) bool {
	return t.Name() != ""
}

func TypeUnderlying(t reflect.Type) (ret reflect.Type) {
	// 如果类型没有名称，则它是一个未命名类型，底层类型就是它自己
	if t.Name() == "" {
		return t
	}

	// 获取kind并使用适当的方法创建底层类型
	kind := t.Kind()

	// 处理基本类型
	switch kind {
	case reflect.Bool:
		return reflect.TypeOf(false)
	case reflect.Int:
		return reflect.TypeOf(int(0))
	case reflect.Int8:
		return reflect.TypeOf(int8(0))
	case reflect.Int16:
		return reflect.TypeOf(int16(0))
	case reflect.Int32:
		return reflect.TypeOf(int32(0))
	case reflect.Int64:
		return reflect.TypeOf(int64(0))
	case reflect.Uint:
		return reflect.TypeOf(uint(0))
	case reflect.Uint8:
		return reflect.TypeOf(uint8(0))
	case reflect.Uint16:
		return reflect.TypeOf(uint16(0))
	case reflect.Uint32:
		return reflect.TypeOf(uint32(0))
	case reflect.Uint64:
		return reflect.TypeOf(uint64(0))
	case reflect.Uintptr:
		return reflect.TypeOf(uintptr(0))
	case reflect.Float32:
		return reflect.TypeOf(float32(0))
	case reflect.Float64:
		return reflect.TypeOf(float64(0))
	case reflect.Complex64:
		return reflect.TypeOf(complex64(0))
	case reflect.Complex128:
		return reflect.TypeOf(complex128(0))
	case reflect.String:
		return reflect.TypeOf("")
	case reflect.UnsafePointer:
		return reflect.TypeOf(unsafe.Pointer(nil))

	// 复合类型
	case reflect.Array:
		return reflect.ArrayOf(t.Len(), t.Elem())
	case reflect.Chan:
		return reflect.ChanOf(t.ChanDir(), t.Elem())
	case reflect.Map:
		return reflect.MapOf(t.Key(), t.Elem())
	case reflect.Ptr:
		return reflect.PointerTo(t.Elem())
	case reflect.Slice:
		return reflect.SliceOf(t.Elem())
	case reflect.Func:
		// 获取参数和返回值类型
		numIn := t.NumIn()
		numOut := t.NumOut()
		in := make([]reflect.Type, numIn)
		out := make([]reflect.Type, numOut)

		for i := 0; i < numIn; i++ {
			in[i] = t.In(i)
		}
		for i := 0; i < numOut; i++ {
			out[i] = t.Out(i)
		}

		return reflect.FuncOf(in, out, t.IsVariadic())
	}

	// 默认返回原始类型
	return t
}
