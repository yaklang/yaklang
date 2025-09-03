package tests

import (
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"
)

func init() {
	test.SetLanguage("js", js2ssa.CreateBuilder)
}
