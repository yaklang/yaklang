package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_Template_To_JAVA_Servlet(t *testing.T) {
	t.Run("test simple servlet xss", func(t *testing.T) {
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
        String userInput = request.getParameter("input");
        request.setAttribute("userInput", userInput);
        request.getRequestDispatcher("/WEB-INF/jsp/demo.jsp").forward(request, response);
    }
} `)
		vf.AddFile("src\\main\\webapp\\WEB-INF\\jsp\\demo.jsp", `
<html>
	<c:out value="${userInput}" />
<html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()
			rule := `
print?{<typeName>?{have:'syntaxflow.template.java.HttpServletRequest'}}(,* #-> as $out);
request?{opcode:param  && <typeName>?{have:'javax.servlet.http.HttpServlet'}} as $source;
		$out #{
until:<<<UNTIL
	<self> & $source
UNTIL
}-> as $request
$request.setAttribute(,,* as $result)
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			res := vals.GetValues("result")
			require.NotNil(t, res)
			require.Contains(t, res.String(), "ParameterMember-parameter[1].getParameter(Parameter-request,\"input\")")
			res.Show()
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
