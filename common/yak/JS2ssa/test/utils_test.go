package test

import (
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("js", js2ssa.Builder)
}