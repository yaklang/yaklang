package tests

import (
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func init() {
	test.SetLanguage("php", php2ssa.Builder)
}
