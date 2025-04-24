package tests

import (
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"
)

func init() {
	test.SetLanguage("new-js", js2ssa.Builder)
}
