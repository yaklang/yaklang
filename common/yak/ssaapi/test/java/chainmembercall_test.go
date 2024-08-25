package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
		prog.SyntaxFlowChain("exec(* #{hook: `*<show>`}->);")
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
