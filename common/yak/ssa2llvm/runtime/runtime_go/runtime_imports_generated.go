package main

import "C"

import (
	_ "unsafe"
	builtin "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"
	poc "github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	yaklib "github.com/yaklang/yaklang/common/yak/yaklib"
)

func init() {}

//export yak_register_globals
func yak_register_globals() {
	runtimeRegisterYaklibGlobals(yaklib.GlobalExport)
	runtimeRegisterYaklibGlobals(builtin.YaklangBaseLib)
	runtimeRegisterYaklibGlobals(map[string]any{
		"len": runtimeYakBuiltinLen,
		"cap": runtimeYakBuiltinCap,
	})
}

//go:linkname poc_init github.com/yaklang/yaklang/common/utils/lowhttp/poc.init
func poc_init()
//export yak_register_module_poc
func yak_register_module_poc() {
	poc_init()
	runtimeRegisterYaklibModule("poc", poc.PoCExports)
}

