package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	<c:out value="${userInput}" escapeXml="false" />
<html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()
			rule := `
print?{<typeName>?{have:'javax.servlet.http.HttpServletResponse'}}(* #-> as $out);
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

func Test_Template_To_Spring(t *testing.T) {
	t.Skip("TODO: support spring framework")
	t.Run("test  freemarker", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("application.properties", `
	# application.properties
spring.freemarker.enabled=true
spring.freemarker.suffix=.ABC
spring.freemarker.charset=UTF-8
spring.freemarker.content-type=text/html
spring.freemarker.check-template-location=true
spring.freemarker.cache=false
`)
		vf.AddFile("controller.java", `import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;

@Controller
public class GreetingController {

    @GetMapping("/greeting")
    public String greeting(Model model) {
        model.addAttribute("name", "World");
        return "greeting"; 
    }
}
`)
		vf.AddFile("greeting.ABC", `
<!DOCTYPE html>
<html>
<head>
    <title>Greeting</title>
</head>
<body>
    <h1>Hello, ${name}!</h1>
</body>
</html>
`)
		ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
			prog.Show()

			rule := `
print?{<typeName>?{have:'javax.servlet.http.HttpServletResponse'}}(* #-> as $out);
model?{opcode:param  && <typeName>?{have:'org.springframework.ui.Model'}} as $source;
		$out #{
until:<<<UNTIL
	<self> & $source
UNTIL
}-> as $model
$model.addAttribute(,* as $res)
`
			vals, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
			require.NoError(t, err)
			res := vals.GetValues("res")
			vals.Show()
			require.NotNil(t, res)
			return nil
		})
	})
}
