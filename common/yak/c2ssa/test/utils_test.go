package test

import (
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("c", c2ssa.CreateBuilder)
}
