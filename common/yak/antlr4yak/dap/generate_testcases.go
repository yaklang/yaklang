package dap

import (
	"path"
	"runtime"
)

type GernerateFuncTyp func() (path string)

var (
	SimpleYakTestCase = "simple.yak"
	FuncCallTestcase  = "func_call.yak"
	IncrementTestcase = "increment.yak"
)

func GetYakTestCasePath(p string) string {
	_, file, _, _ := runtime.Caller(0)
	dirPath := path.Dir(file)
	return path.Join(dirPath, "_fixup", p)
}
