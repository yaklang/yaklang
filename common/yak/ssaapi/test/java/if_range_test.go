package java

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/groovy_eval_with_if.java
var groovy_eval_with_if string

func TestIfRange(t *testing.T) {
	ssatest.Check(t, groovy_eval_with_if, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
