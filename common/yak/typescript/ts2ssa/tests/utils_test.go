package tests

import (
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
)

func init() {
	test.SetLanguage("ts", ts2ssa.CreateBuilder)
}
