package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("php", php2ssa.Builder)
}

func CheckPrintTopDef(t *testing.T, code string, wants []string) {
	test.CheckSyntaxFlowContain(t, code,
		`println(* #-> * as $param)`,
		map[string][]string{"param": wants},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
