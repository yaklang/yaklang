package test

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
)

func TestJSP2Java_Content(t *testing.T) {
	tests := []struct {
		name    string
		jspCode string
		wants   []string
	}{
		{
			"test  JspElementWithOpenTagOnly pure text  ",
			`<%@ page language="java" contentType="text/html; charset=ISO-8859-1"
           pageEncoding="ISO-8859-1"%>
  <%@ page import="com.sectooladdict.encoders.HtmlEncoder" %>
  <!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">
  <html>
  <head>
      <meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">
      <title>JavaScript Injection in DOM XSS Sink eval()</title>
  </head>
  <body>
  <%
      if (request.getParameter("userinput") == null) {
  %>
  Enter your input:<br><br>
  <form name="frmInput" id="frmInput" action="Case36-InjectionDirectlyInToDomXssSinkEval.jsp" method="GET">
      <input type="text" name="userinput" id="userinput"><br>
      <input type=submit value="submit">
  </form>
  <%
      }
      else {
          try {
              String userinput = request.getParameter("userinput");
              userinput = HtmlEncoder.htmlEncodeAngleBracketsAndQuotes(userinput);
              out.println("<script>\neval(\"" + userinput + "\");</script>");
              out.flush();
          } catch (Exception e) {
              out.println("Exception details: " + e);
          }
      } //end of if/else block
  %>
  </body>
  </html>`,
			[]string{
				"out = response.getWriter();",
			},
		},
		{
			"test JspElementWithClosingTagOnly pure text  ",
			"<html/>",
			[]string{},
		},
		{
			"test JspElementWithTagAndContent pure text  ",
			"<title>hello</title>",
			[]string{},
		},
		{
			"test jsp java script",
			"<%\n    int sum = 5 + 10;\n    out.println(\"Sum is: \" + sum);\n%>",
			[]string{
				"int sum = 5 + 10;",
				"out.println(\"Sum is: \" + sum);",
			},
		},
		{
			"test jsp expression script",
			`<%= request.getParameter("userInput") %>`,
			[]string{
				`out.print(request.getParameter("userInput"));`,
			},
		},
		{
			"test jsp declaration script",
			`<%! int count = 0; %>`,
			[]string{
				`int count = 0;`,
			},
		},
		{
			"test jsp directive script import",
			`<%@ page import="java.util.*, com.example.model.User" %>`,
			[]string{},
		},
		{
			"test el expression in html content",
			`<p>Welcome, Admin! Your user type is: ${sessionScope.userType}</p>`,
			[]string{
				`elExpr.parse("sessionScope.userType"`,
			}},
		// core tag
		// core out tag
		{
			"test jstl-core out tag",
			"<c:out value='${name}'/>",
			[]string{
				`out.printWithEscape(elExpr.parse("name"));`,
			},
		},
		{
			"test jstl-core out tag without escaping",
			"<c:out value='${name}' escapeXml=\"false\"/>",
			[]string{
				`out.print(elExpr.parse("name"));`,
			},
		},
		// core set tag
		{
			"test jstl-core set tag",
			"<c:set var='name' value='John'/>",
			[]string{
				`request.setAttribute("name", John);`,
			},
		},
		// core if tag
		{"test jstl-core if tag 1",
			"<c:if test='${age  <  16 }'>Hello John</c:if>",
			[]string{
				`if (elExpr.parse("age  <  16 "))`,
				`out.write("Hello John");`,
			}},
		{"test jstl-core if tag 2",
			` <c:if test="${sessionScope.userType == 'admin'}">
			 <p>Welcome, Admin! Your user type is: ${sessionScope.userType}</p>
			 </c:if>
        `,
			[]string{
				`if (elExpr.parse("sessionScope.userType == 'admin'")) `,
				`out.write("Welcome, Admin! Your user type is: ");`,
				`out.print(elExpr.parse("sessionScope.userType"));`,
			},
		},
		//core choose tag
		{
			"test jstl-core choose tag ",
			`
			<c:choose>
				<c:when test="${valueToSwitch eq 'case1'}">
					Value is case1
				</c:when>
				<c:when test="${valueToSwitch eq 'case2'}">
					Value is case2
				</c:when>
				<c:when test="${valueToSwitch eq 'case3'}">
					Value is case3
				</c:when>
				<c:otherwise>
					Value does not match any case
				</c:otherwise>
			</c:choose>
`,
			[]string{
				"switch (true) {",
				`case elExpr.parse("valueToSwitch eq 'case1'"):`,
				`out.write("\t\t\t\t\tValue is case1");`,
				"default:"},
		},
		{
			"test jstl-core foreach tag ",
			` 
		<c:forEach var="item" items="${items}">
        <li>${item}</li>
   	 	</c:forEach>`,
			[]string{
				"for (Object item : elExpr.parse(\"items\")) {",
				"out.print(elExpr.parse(\"item\"));",
			},
		},
		{
			"test el expression in jstl ",
			`
        <p>日期：<fmt:formatDate value="${now}" pattern="yyyy-MM-dd HH:mm:ss" /></p>
`,
			[]string{
				"out.print(elExpr.parse(\"now\"));",
			},
		},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := tj.ConvertTemplateToJava(tj.JSP, jspCode, "test.jsp")
		require.NoError(t, err)
		require.NotNil(t, codeInfo)
		checkJavaFront(t, codeInfo.GetContent(), "test.jsp")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(tt.jspCode, tt.wants)
		})
	}
}

//go:embed all:jspcode
var jspDir embed.FS

func validateSavedJSPTemplateFixture(t *testing.T, filePath string, content string) {
	t.Helper()

	start := time.Now()
	codeInfo, err := tj.ConvertTemplateToJava(tj.JSP, content, filePath)
	convertDur := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, codeInfo)
	require.LessOrEqual(t, convertDur, generatedJavaFixtureMaxParseDuration, "template to java took too long for %s", filePath)
	checkJavaFront(t, codeInfo.GetContent(), filePath)
}

func TestAllSavedJSPTemplates(t *testing.T) {
	found := false
	err := fs.WalkDir(jspDir, "jspcode", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filePath)
		if !strings.HasSuffix(ext, ".jsp") && !strings.HasSuffix(ext, ".jspx") {
			return nil
		}

		content, err := fs.ReadFile(jspDir, filePath)
		require.NoError(t, err)
		t.Run(filePath, func(t *testing.T) {
			validateSavedJSPTemplateFixture(t, filePath, string(content))
		})
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no saved jsp templates found")
}
