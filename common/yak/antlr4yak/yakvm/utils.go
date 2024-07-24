package yakvm

import (
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
			if v1, ok1 := frame.GlobalVariables[name]; ok1 {
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
