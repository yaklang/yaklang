<%@ page language="java" contentType="text/html; charset=UTF-8" pageEncoding="UTF-8"%>
<%@ taglib uri="http://java.sun.com/jsp/jstl/core" prefix="c" %>
<%@ taglib uri="http://java.sun.com/jsp/jstl/fmt" prefix="fmt" %>
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>JSTL基础演示</title>
    <style>
        .section { margin: 20px; padding: 10px; border: 1px solid #ccc; }
        .code { background: #f5f5f5; padding: 5px; }
    </style>
</head>
<body>
    <h1>JSTL基础演示</h1>

    <div class="section">
        <h2>1. EL表达式基础</h2>
        <%
            // 设置一些属性用于演示
            request.setAttribute("name", "张三");
            request.setAttribute("age", 25);
            session.setAttribute("message", "Hello JSTL!");
        %>
        
        <p>直接输出：${name}</p>
        <p>数字输出：${age}</p>
        <p>session中的值：${message}</p>
        <p>简单计算：${age + 10}</p>
        <p>判断：${age > 18 ? '成年' : '未成年'}</p>
    </div>

    <div class="section">
        <h2>2. c:set使用</h2>
        <c:set var="hobby" value="读书" />
        <p>设置的值：${hobby}</p>
    </div>

    <div class="section">
        <h2>3. c:if使用</h2>
        <c:if test="${age >= 18}">
            <p>已经成年了</p>
        </c:if>
    </div>

    <div class="section">
        <h2>4. c:choose使用</h2>
        <c:choose>
            <c:when test="${age < 18}">
                <p>未成年</p>
            </c:when>
            <c:when test="${age < 60}">
                <p>成年人</p>
            </c:when>
            <c:otherwise>
                <p>老年人</p>
            </c:otherwise>
        </c:choose>
    </div>

    <div class="section">
        <h2>5. c:forEach使用</h2>
        <%
            String[] fruits = {"苹果", "香蕉", "橙子"};
            request.setAttribute("fruits", fruits);
        %>
        <ul>
            <c:forEach items="${fruits}" var="fruit" varStatus="status">
                <li>${status.count}. ${fruit}</li>
            </c:forEach>
        </ul>
    </div>

    <div class="section">
        <h2>6. fmt格式化</h2>
        <p>日期：<fmt:formatDate value="${now}" pattern="yyyy-MM-dd HH:mm:ss" /></p>
        
        <c:set var="money" value="12345.678" />
        <p>货币：<fmt:formatNumber value="${money}" type="currency" /></p>
    </div>

    <div class="section">
        <h2>7. EL运算符</h2>
        <p>数学运算：${10 + 5} = 15</p>
        <p>比较运算：${10 > 5} = true</p>
        <p>逻辑运算：${true && false} = false</p>
        <p>空值判断：${empty name ? '空' : '非空'}</p>
    </div>
</body>
</html> 