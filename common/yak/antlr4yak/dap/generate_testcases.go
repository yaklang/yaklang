package dap

import (
	"path"
	"runtime"
)

type GernerateFuncTyp func() (path string)

var (
	SimpleYakTestCase          = "simple.yak"
	FuncCallTestcase           = "func_call.yak"
	IncrementTestcase          = "increment.yak"
	GoroutineTestcase          = "goroutine.yak"
	VariablesTestcase          = "variables.yak"
	StepAndNExtTestcase        = "step_and_next.yak"
	HardCodeBreakPointTestcase = "hardcode_breakpoint.yak"
)

func GetYakTestCasePath(p string) string {
	_, file, _, _ := runtime.Caller(0)
	dirPath := path.Dir(file)
	return path.Join(dirPath, "_fixup", p)
}
