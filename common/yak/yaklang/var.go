package yaklang

import (
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"reflect"
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
