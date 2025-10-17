package test

import (
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func init() {
	test.SetLanguage("yak", yak2ssa.CreateBuilder)
}
