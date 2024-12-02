package tests

import (
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"testing"
)

func TestGetAST(t *testing.T) {
	result, err := jsp.GetAST(`<%@ page language="java" contentType="text/html; charset=UTF-8" pageEncoding="UTF-8"
    import="java.util.*, java.text.*" session="true" buffer="8kb" autoFlush="true" isThreadSafe="true" errorPage="error.jsp" %>

<%@ include file="header.jsp" %>
<%@ taglib prefix="c" uri="http://java.sun.com/jsp/jstl/core" %>

<html>
<head>
    <title>JSP 综合测试页面</title>
</head>
<body>
    <!-- JSP 注释 -->
    <%-- 这是一个 JSP 注释，不会在客户端显示 --%>

    <!-- HTML 注释 -->
    <!-- 这是一个 HTML 注释，会在页面源代码中显示 -->

    <h1>JSP 基本语法测试</h1>

    <%
        // 脚本片段（Scriptlet）
        String name = "World";
        int count = 0;
    %>

    <p>Hello, <%= name %>!</p>

    <%!
        // 声明（Declaration）
        public String formatDate(Date date) {
            SimpleDateFormat sdf = new SimpleDateFormat("yyyy-MM-dd HH:mm:ss");
            return sdf.format(date);
        }
    %>

    <p>当前时间：<%= formatDate(new Date()) %></p>

    <jsp:useBean id="user" class="com.example.User" scope="session" />
    <jsp:setProperty name="user" property="username" value="guest" />

    <p>用户名：<jsp:getProperty name="user" property="username" /></p>

    <jsp:include page="footer.jsp" flush="true">
        <jsp:param name="param1" value="value1" />
    </jsp:include>

    <c:if test="${param.showMessage == 'true'}">
        <p>显示一条消息</p>
    </c:if>

    <c:forEach var="i" begin="1" end="5">
        <p>循环次数：${i}</p>
    </c:forEach>

    <%-- 异常处理演示 --%>
    <%
        try {
            int result = 10 / count;
        } catch (Exception e) {
            out.println("发生异常：" + e.getMessage());
        }
    %>

    <jsp:forward page="nextPage.jsp">
        <jsp:param name="forwardParam" value="forwardValue" />
    </jsp:forward>

    ${2 + 3}

    <%
        // 使用内置对象
        out.println("请求的方法是：" + request.getMethod());
        out.println("会话ID是：" + session.getId());
        application.setAttribute("appName", "JSP Test Application");
    %>

    <p>应用名称：<%= application.getAttribute("appName") %></p>

    <%@ page isELIgnored="false" %>

    <p>使用 EL 表达式显示参数：${param.exampleParam}</p>
</body>
</html>
`)
	if err != nil {
		t.Fatal(err)
	}
	jspEq := result.JspDocuments()
	if jspEq == nil {
		t.Fatal("jspEq is nil")
	}
}
