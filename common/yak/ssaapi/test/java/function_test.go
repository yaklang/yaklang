package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestClassMethod(t *testing.T) {
	code := `package A;
class A{
	public void Method(int a){}
	public void XX(int b){}
}
`
	ssatest.CheckSyntaxFlow(t, code, `*Method(* as $params)`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
