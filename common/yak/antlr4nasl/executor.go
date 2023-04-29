package antlr4nasl

import "yaklang/common/yak/antlr4yak/yakvm"

func Exec(code string, init ...bool) {
	_Exec(false, code, init...)
}

func DebugExec(code string, init ...bool) {
	_Exec(true, code, init...)
}

func _Exec(debug bool, code string, init ...bool) {
	engine := New()
	if len(init) == 0 {
		engine.Init()
	}
	err := engine.SafeEval(code)
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
	engine.Init()
	return engine.RunFile(path)
}
