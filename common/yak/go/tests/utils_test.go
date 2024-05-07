package tests

import (
	"github.com/yaklang/yaklang/common/yak/go/go2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func init() {
	test.SetLanguage("go", go2ssa.Build)
}
