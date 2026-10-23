package jsp

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestFullPageScriptletSurvivesNestedHTML(t *testing.T) {
	src := `
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
`
	ast, err := Front(src)
	if err != nil {
		t.Fatal(err)
	}
	visitor := NewJSPVisitor()
	visitor.Editor = memedit.NewMemEditor(src)
	if err := visitJspDocumentsSafely(visitor, ast); err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, instruction := range visitor.GetInstructions() {
		code.WriteString(instruction.Text)
		code.WriteByte('\n')
	}
	got := code.String()
	if !strings.Contains(got, "getServerInfo") || !strings.Contains(got, "已经成年了") || !strings.Contains(got, "老年人") {
		tree := ast.ToStringTree(nil, ast.GetParser())
		if len(tree) > 6000 {
			tree = tree[:6000]
		}
		t.Fatalf("page body dropped:\n%s\nAST:\n%s", got, tree)
	}
}

func TestSelfClosingChooseIsNotTagText(t *testing.T) {
	src := `<c:choose/><c:when test="${ok}"/>`
	ast, err := Front(src)
	if err != nil {
		t.Fatal(err)
	}
	visitor := NewJSPVisitor()
	visitor.Editor = memedit.NewMemEditor(src)
	if err := visitJspDocumentsSafely(visitor, ast); err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, instruction := range visitor.GetInstructions() {
		code.WriteString(instruction.Text)
		code.WriteByte('\n')
	}
	if strings.Contains(code.String(), "/>") || strings.Contains(code.String(), "switch") {
		t.Fatalf("self-closing choose/when was emitted as markup or a switch:\n%s", code.String())
	}
}

func TestPairedCoreTagsPreserveBody(t *testing.T) {
	src := `<c:forTokens items="${items}" delims="," var="item"><c:out value="${item}"/></c:forTokens>`
	ast, err := Front(src)
	if err != nil {
		t.Fatal(err)
	}
	visitor := NewJSPVisitor()
	visitor.Editor = memedit.NewMemEditor(src)
	if err := visitJspDocumentsSafely(visitor, ast); err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, instruction := range visitor.GetInstructions() {
		code.WriteString(instruction.Text)
		code.WriteByte('\n')
	}
	if !strings.Contains(code.String(), "for (String item") || !strings.Contains(code.String(), `elExpr.parse("item")`) {
		t.Fatalf("body disappeared:\n%s\nAST:\n%s", code.String(), ast.ToStringTree(nil, ast.GetParser()))
	}
}
