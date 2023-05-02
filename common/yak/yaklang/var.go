package yaklang

import (
	"reflect"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"
)

func IsYakFunction(i interface{}) bool {
	return IsNewYakFunction(i)
}

func IsNewYakFunction(i interface{}) bool {
	_, ok := i.(*yakvm.Function)
	if ok {
		return true
	}

	return reflect.TypeOf(i).Kind() == reflect.Func
}
