package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Java_Unescape_Output(t *testing.T) {
	t.Run("get the unescaped output", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("xss_demo.jsp", `
<!-- xss_example.jsp -->
<%@ page contentType="text/html;charset=UTF-8" language="java" %>
<html>
<head>
    <title>XSS Vulnerability Example</title>
</head>
<body>
    <h2>User Input:</h2>
    <div>${sessionScope.userInput}</div>
</body>
</html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()

			vals, err := prog.SyntaxFlowWithError(`<javaUnescapeOutput> as $res `)
			require.NoError(t, err)
			require.NotNil(t, vals)

			res := vals.GetValues("res").Show()
			require.Contains(t, res.String(), "sessionScope.userInput")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("simple xss demo", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("XSSExampleServlet.java", `
import java.io.*;
import javax.servlet.*;
import javax.servlet.http.*;

public class XSSVulnerableServlet extends HttpServlet {
    protected void doPost(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String userInput = request.getParameter("input");
        request.setAttribute("userInput", userInput);
		request.getRequestDispatcher("/xss-vulnerable.jsp").forward(request, response);
    }
}`)
		vf.AddFile("src/main/webapp/jsp/xss-vulnerable.jsp", `
<%@ page contentType="text/html;charset=UTF-8" language="java" %>
<html>
<head>
    <title>XSS Vulnerable Page</title>
</head>
<body>
    <h2>User Input:</h2>
    <!-- 直接显示用户输入，没有进行转义，存在XSS风险 -->
    <div>${requestScope.userInput}</div>
</body>
</html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()

			rule := `
<javaUnescapeOutput> as $sink;
request?{opcode:param  && <typeName>?{have:'javax.servlet.http.HttpServlet'}} as $source;
$sink #{
	include:<<<EOF
<self> & $source
EOF
}-> as $request;
$request.setAttribute(,,* as $res)
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			require.NotNil(t, vals)
			vals.Show()

			res := vals.GetValues("res")
			require.Contains(t, res.String(), "ParameterMember-parameter[1].getParameter(Parameter-request,\"input\")")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test unescape out", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("/jsp/messages/seemessages.jsp", `
<%@ page contentType="text/html;charset=UTF-8" language="java" %>
<%@page import="java.util.Iterator" %>
<%@page import="java.util.ArrayList" %>
<%@page import="entity.Message" %>
<%@page import="java.util.ArrayList" %>
<html>
<head>
    <title>showmessages</title>
</head>
<body>
<h2>Show Messages</h2>
<table border=1 cellspacing="0">
    <tr>
        <th>留言人姓名</th>
        <th>留言时间</th>
        <th>留言标题</th>
        <th>留言内容</th>
    </tr>
    <%
        ArrayList<Message> all = new ArrayList();
        all = (ArrayList) session.getAttribute("all_messages");
        if (all != null) {
            Iterator it = all.iterator();
            while (it.hasNext()) {
                Message ms = (Message) it.next();
    %>
    <tr>
        <td><%= ms.getUsername() %>
        </td>
        <td><%= ms.getTime().toString() %>
        </td>
        <td><%= ms.getTitle() %>
        </td>
        <td><%= ms.getMessage() %>
        </td>
    </tr>
    <%
            }
        }
    %>
</table>
</body>
</html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			programs.Show()
			rule := `
<javaUnescapeOutput> as $sink;
`
			result, err := programs.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			res := result.GetValues("sink")
			res.Show()
			require.NotNil(t, res)
			require.Contains(t, res.String(), "getUsername")
			require.Contains(t, res.String(), "getTitle")
			require.Contains(t, res.String(), "getMessage")
			require.Contains(t, res.String(), "toString")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
