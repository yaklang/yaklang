package tests

import (
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func init() {
	test.SetLanguage("java", java2ssa.Build)
}
