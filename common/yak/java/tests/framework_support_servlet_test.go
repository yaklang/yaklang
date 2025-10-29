package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
			res := prog.SyntaxFlowChain("printWithEscape()?{<typeName>?{have:'javax.servlet.http.HttpServletResponse'}} as $print")
			require.Contains(t, res.String(), "printWithEscape(ParameterMember-parameter[2].getWriter(Parameter-response),Undefined-elExpr.parse(Undefined-elExpr,\"message\"))")
			return nil
		})
	})
}

func TestJsp_To_Java_Range(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("test.jsp", `
<%@ page language="java" contentType="text/html; charset=UTF-8" pageEncoding="UTF-8"%>
    <%@ page import="java.util.*, com.example.model.User" %>
<%@ taglib uri="http://java.sun.com/jsp/jstl/core" prefix="c" %>

<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>JSP 特性演示</title>
    <style>
        .section { margin: 20px; padding: 10px; border: 1px solid #ccc; }
    </style>
</head>
<body>
    <div class="section">
        <p>当前时间：<%= new Date() %></p>
        <p>服务器信息：<%= application.getServerInfo() %></p>
    </div>
	 <div class="section">
        <h2>3. c:if使用</h2>
        <c:if test="${age1 >= 18}">
            <p>已经成年了</p>
        </c:if>
    </div>
    <div class="section">
        <h2>4. c:choose使用</h2>
        <c:choose>
            <c:when test="${age2 < 18}">
                <p>未成年</p>
            </c:when>
            <c:when test="${age2 < 60}">
                <p>成年人</p>
            </c:when>
            <c:otherwise>
                <p>老年人</p>
            </c:otherwise>
        </c:choose>
    </div>
</body>
</html>
`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		prog.Show()
		vals, err := prog.SyntaxFlowWithError(`
application.getServerInfo() as  $res;
out.write(,*?{have:'已经成年了'} as $ifStmt) ;
out.write(,*?{have:'老年人'} as $switchStmt) ;
`)
		require.NoError(t, err)
		res := vals.GetValues("res")
		require.NotNil(t, res)
		res.ShowWithSource()
		require.Contains(t, res.StringEx(1), `<%= application.getServerInfo() %>`)

		ifStmt := vals.GetValues("ifStmt")
		require.NotNil(t, ifStmt)
		ifStmt.ShowWithSource()
		require.Contains(t, ifStmt.StringEx(1), `已经成年了</p>`)

		switchStmt := vals.GetValues("switchStmt")
		require.NotNil(t, switchStmt)
		switchStmt.ShowWithSource()
		require.Contains(t, switchStmt.StringEx(1), `老年人</p>`)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
