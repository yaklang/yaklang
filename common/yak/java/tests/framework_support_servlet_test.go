package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_Template_To_JAVA_Servlet(t *testing.T) {
	t.Run("test HookServletMemberCallMethod", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("servlet.java", `package com.example.servlet;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.servlet.http.HttpSession;

@WebServlet("/demo")
public class DemoServlet extends HttpServlet {
    
    @Override
    protected void doGet(HttpServletRequest request, HttpServletResponse response) 
            throws ServletException, IOException {
        request.setAttribute("message", "this is message");
        request.getRequestDispatcher("/WEB-INF/jsp/demo.jsp").forward(request, response);
    }
} `)
		vf.AddFile("src\\main\\webapp\\WEB-INF\\jsp\\demo.jsp", `
<html>
	<c:out value="${message}" />
<html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()
			res := prog.SyntaxFlowChain("print()?{<typeName>?{have:'syntaxflow.template.java.HttpServletRequest'}} as $print")
			require.Contains(t, res.String(), `Undefined-out.print(ParameterMember-parameter[1].getOut(Parameter-request),Undefined-escapeHtml(ParameterMember-parameter[1].getAttribute(Parameter-request,"message")))`)
			return nil
		})
	})
}
