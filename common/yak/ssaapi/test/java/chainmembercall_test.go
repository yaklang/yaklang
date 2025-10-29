package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestChainMemberCall(t *testing.T) {
	code := `package com.example;
public class CommandInjectionServlet2 extends HttpServlet {
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String userInput = request.getParameter("cmd").concat("a").concat("b").concat("c");
        String command = "cmd.exe /c " + userInput; // 直接使用用户输入
        exec(command);
    }
}
`
	rule := `
getPara*() as $source;
exec(* #{
until: <<<UNTIL
* & $source as $vuln
UNTIL
}->);
$vuln<dataflow(<<<CODE
*?{opcode:call}<getCallee>?{<name>?{have:'concat'}}(,* as $a) as $concatA ;
CODE)>
$concatA<getCall><getCallee>(,* as $b) as $concatB;
$concatB<getCall><getCallee>(,* as $c);
$a + $b + $c as $info;
$concatA + $concatB as $concat;
`

	ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
		"info": {`"a"`, `"b"`, `"c"`},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
