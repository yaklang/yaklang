package ssa4yak

import "github.com/yaklang/yaklang/common/yak/ssa"

// vm buildin function
var buildin = make(map[string]*ssa.Function)

func init() {
	// print(...any) nil
	buildin["print"] = ssa.NewFunctionDefine("print", nil, nil, true)
}
