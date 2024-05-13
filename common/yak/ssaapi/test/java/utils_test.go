package java

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func checkTop(t *testing.T, code, syntaxFlow, expect string, isSink bool) {
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		results, err := prog.SyntaxFlowWithError(syntaxFlow)
		if err != nil {
			t.Fatal(err)
		}
		v, ok := results["*"]
		if !ok {
			t.Fatalf("not result data, got %v", results)
		}
		topDef := v.GetTopDefs(ssaapi.WithAllowCallStack(true))
		topDef.ShowWithSource()

		count := strings.Count(topDef.StringEx(0), expect)
		if isSink {
			if count != 1 {
				return utils.Errorf("want to get source [%v],but got [%v].", expect, topDef.StringEx(0))
			}
		} else {
			if count != 0 {
				return utils.Errorf("want to get source [%v],but got [%v].", expect, topDef.StringEx(0))
			}
		}
		return nil
	}, ssaapi.JAVA)
}

func testExecTopDef(t *testing.T, code string, expect string, isSink bool) {
	checkTop(t, code, "Runtime.getRuntime().exec()", expect, isSink)
}
func testRequestTopDef(t *testing.T, code string, expect string, isSink bool) {
	checkTop(t, code, ".createDefault().execute()", expect, isSink)
}
