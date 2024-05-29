package script_core

import (
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func Exec(code string, init ...bool) {
	_Exec(false, code, init...)
}

func DebugExec(code string, init ...bool) {
	_Exec(true, code, init...)
}

func _Exec(debug bool, code string, init ...bool) {
	exec := executor.NewNaslExecutor()
	//engine.vm.GetConfig().SetStopRecover(true)
	//if len(init) == 0 {
	//	engine.InitBuildInLib()
	//}
	err := exec.Exec(code, "test-file")
	if debug {
		yakvm.ShowOpcodes(exec.Compiler.GetCodes())
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
	engine := executor.NewNaslExecutor()
	//engine.InitBuildInLib()
	return engine.RunFile(path)
}
