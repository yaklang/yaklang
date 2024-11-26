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
    <h1>JSP 特性演示</h1>

    <!-- 1. JSP声明 -->
    <div class="section">
        <h2>1. JSP声明示例</h2>
        <%!
            private int count = 0;
            public String getGreeting() {
                return "你好，访客！";
            }
        %>
        <p><%= getGreeting() %></p>
    </div>

    <!-- 2. JSP脚本片段 -->
    <div class="section">
        <h2>2. JSP脚本片段示例</h2>
        <%
            count++;
            out.println("这是第 " + count + " 次访问此页面");
        %>
    </div>

    <!-- 3. JSP表达式 -->
    <div class="section">
        <h2>3. JSP表达式示例</h2>
        <p>当前时间：<%= new Date() %></p>
        <p>服务器信息：<%= application.getServerInfo() %></p>
    </div>

    <!-- 4. 内置对象使用 -->
    <div class="section">
        <h2>4. 内置对象示例</h2>
        <p>Request属性：<%= request.getAttribute("message") %></p>
        <p>Session属性：<%= session.getAttribute("sessionMessage") %></p>
        <p>Application属性：<%= application.getAttribute("appMessage") %></p>
    </div>

    <!-- 5. 循环和条件判断 -->
    <div class="section">
        <h2>5. 循环和条件判断示例</h2>
        <% List<User> users = (List<User>)request.getAttribute("users"); %>
        <% if(users != null && !users.isEmpty()) { %>
            <table border="1">
                <tr>
                    <th>姓名</th>
                    <th>年龄</th>
                    <th>邮箱</th>
                </tr>
                <% for(User user : users) { %>
                    <tr>
                        <td><%= user.getName() %></td>
                        <td><%= user.getAge() %></td>
                        <td><%= user.getEmail() %></td>
                    </tr>
                <% } %>
            </table>
        <% } %>
    </div>

    <!-- 6. JSTL标签使用 -->
    <div class="section">
        <h2>6. JSTL标签示例</h2>
        <c:forEach var="user" items="${users}">
            <p>
                姓名：${user.name},
                年龄：${user.age},
                邮箱：${user.email}
            </p>
        </c:forEach>
    </div>

    <!-- 7. 异常处理 -->
    <div class="section">
        <h2>7. 异常处理示例</h2>
        <%
            try {
                int result = 10 / 0; // 故意制造异常
            } catch(Exception e) {
                out.println("捕获到异常：" + e.getMessage());
            }
        %>
    </div>

    <!-- 8. 页面信息 -->
    <div class="section">
        <h2>8. 页面信息</h2>
        <p>客户端IP地址：<%= request.getRemoteAddr() %></p>
        <p>服务器名称：<%= request.getServerName() %></p>
        <p>服务器端口：<%= request.getServerPort() %></p>
    </div>
</body>
</html>