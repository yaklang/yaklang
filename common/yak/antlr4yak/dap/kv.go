package dap

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

type KV struct {
	Key   string
	Value *yakvm.Value
}
