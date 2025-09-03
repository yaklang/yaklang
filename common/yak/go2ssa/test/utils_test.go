package test

import (
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("go", go2ssa.CreateBuilder)
}
