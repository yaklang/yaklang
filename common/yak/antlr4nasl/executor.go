package antlr4nasl

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func Exec(code string, init ...bool) {
	_Exec(false, code, init...)
}

func DebugExec(code string, init ...bool) {
	_Exec(true, code, init...)
}

func _Exec(debug bool, code string, init ...bool) {
	engine := New()
	//engine.vm.GetConfig().SetStopRecover(true)
	if len(init) == 0 {
		engine.InitBuildInLib()
	}
	err := engine.Eval(code)
	if debug {
		yakvm.ShowOpcodes(engine.compiler.GetCodes())
	}
	if yakvm.GetUndefined().Value != nil {
		panic("undefined value")
	}

	if err != nil {
		panic(err)
	}
	return
}
func ExecFile(path string) error {
	engine := New()
	engine.InitBuildInLib()
	return engine.RunFile(path)
}
