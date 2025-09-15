package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestChainMemberCall(t *testing.T) {
	ssatest.Check(t, `
package com.example;
public class CommandInjectionServlet2 extends HttpServlet {
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String userInput = request.getParameter("cmd").concat("a").concat("b").concat("c");
        String command = "cmd.exe /c " + userInput; // 直接使用用户输入
        exec(command);
    }
}
`, func(prog *ssaapi.Program) error {
		rule := `
getPara*() as $source;
exec(* #{
until: <<<UNTIL
* ?{<self> & $source} as $vuln
UNTIL
}->);
$vuln<dataflow(<<<CODE
*?{opcode:call}<getCallee>?{<name>?{have:'concat'}}(,* as $a) as $concatA ;
CODE)>
$concatA<getCall><getCallee>(,* as $b) as $concatB;
$concatB<getCall><getCallee>(,* as $c);
$a + $b + $c as $info;
$concatA + $concatB as $concat;
check $concat; check $info; $info<show>
`
		result, err := prog.SyntaxFlowWithError(rule)
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		results := result.GetValues("info")
		assert.Equal(t, results.Len(), 3)
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
