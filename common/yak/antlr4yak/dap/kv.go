package dap

import (
	"reflect"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type ScopeKV struct {
	Key   string
	Value *yakvm.Value
}

type MapKey struct {
	Key    reflect.Value
	IKey   interface{}
	KeyStr string
}
