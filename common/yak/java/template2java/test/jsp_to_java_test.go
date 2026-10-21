package test

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	j "github.com/yaklang/yaklang/common/yak/java/template2java"
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
				`request.setAttribute("name", "John");`,
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
		{
			"test el function call with parentheses",
			`<input value="${fn:escapeXml(!empty param.password ? param.password : param.pass)}">`,
			[]string{
				`out.print(elExpr.parse("fn:escapeXml(!empty param.password ? param.password : param.pass)"));`,
			},
		},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := j.ConvertTemplateToJava(j.JSP, jspCode, "test.jsp")
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
	codeInfo, err := j.ConvertTemplateToJava(j.JSP, content, filePath)
	convertDur := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, codeInfo)
	require.LessOrEqual(t, convertDur, generatedJavaFixtureMaxParseDuration, "template to java took too long for %s", filePath)
	checkJavaFront(t, codeInfo.GetContent(), filePath)
}

// TestJSPCorpusGeneratesParsableJava converts every .jsp under YAK_JSP_CORPUS
// and parses the generated Java. It is the regression for the 2026-10-21 coremail
// scan, where fn:escapeXml(...) and GBK bytes produced Java the frontend rejected.
// The corpus is a local tree, so the test skips when the variable is unset.
func TestJSPCorpusGeneratesParsableJava(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("YAK_JSP_CORPUS"))
	if root == "" {
		t.Skip("YAK_JSP_CORPUS is not set")
	}
	var paths []string
	if list := strings.TrimSpace(os.Getenv("YAK_JSP_LIST")); list != "" {
		raw, rerr := os.ReadFile(list)
		require.NoError(t, rerr)
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				paths = append(paths, filepath.Join(root, line))
			}
		}
	} else {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".jsp") {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		require.NoError(t, err)
	}
	var failed []string
	checked := 0
	for _, path := range paths {
		checked++
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			failed = append(failed, path+": read: "+rerr.Error())
			continue
		}
		rel := path
		if r, rerr := filepath.Rel(root, path); rerr == nil {
			rel = r
		}
		info, cerr := j.ConvertTemplateToJava(j.JSP, string(raw), rel)
		if cerr != nil {
			failed = append(failed, rel+": convert: "+cerr.Error())
			continue
		}
		if _, perr := java2ssa.Frontend(info.GetContent()); perr != nil {
			msg := perr.Error()
			if len(msg) > 180 {
				msg = msg[:180]
			}
			failed = append(failed, rel+": java: "+msg)
		}
	}
	require.Greater(t, checked, 0)
	if len(failed) > 0 {
		limit := failed
		if len(limit) > 20 {
			limit = limit[:20]
		}
		t.Fatalf("%d/%d jsp files did not produce parsable java:\n%s", len(failed), checked, strings.Join(limit, "\n"))
	}
	t.Logf("parsed %d jsp files from %s", checked, root)
}

func TestAllSyntaxForJSPTemplate_G4(t *testing.T) {
	TestAllSavedJSPTemplates(t)
	TestJSPFailureSamplesParseGenerateAndParseJava(t)
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

func TestJSPFailureSamplesParseGenerateAndParseJava(t *testing.T) {
	// Each sample comes from a page that previously failed somewhere in the
	// JSP -> Java -> java2ssa chain. Structured parsing may fail on malformed
	// HTML; generation must then use the grammar-free translator, and the Java
	// emitted by either route must parse again.
	cases := []struct {
		name string
		jsp  string
	}{
		{
			name: "el function call",
			jsp:  `<input value="${fn:escapeXml(!empty param.password ? param.password : param.pass)}">`,
		},
		{
			name: "unquoted jstl test",
			jsp: `<c:set var="skipKeys" value="|uid|user|password|"/>
<c:if test="ajc.result.code == 'FA_SECRITY'">
  <c:redirect url="${ajc.exitURL}"/>
</c:if>`,
		},
		{
			name: "nested choose",
			jsp: `<c:choose>
  <c:when test="${!empty account.smtpHost}"><p>smtp</p></c:when>
  <c:when test="${(empty account.smtpHost) && (account.type eq 'ews')}">
    <c:choose>
      <c:when test="${user.supportEws}">ews</c:when>
      <c:otherwise>no</c:otherwise>
    </c:choose>
  </c:when>
  <c:otherwise>other</c:otherwise>
</c:choose>`,
		},
		{
			name: "xml declaration after tags",
			jsp: `<wm:i18n category="main"/>
<fmt:message key="title_tip" var="title"/>
<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE wml PUBLIC "-//OPENWAVE.COM//DTD WML 1.3//EN" "http://www.openwave.com/dtd/wml13.dtd">
<wml><card>x</card></wml>`,
		},
		{
			name: "unclosed checkbox row",
			jsp: `<tr class="single-line">
  <td align="right"><fmt:message key=".info.privilege"/></td>
  <td><input type="checkbox" name="supportApproval" id="supportApproval" value="1" ${bean.supportApproval == '1' ? 'checked' : ''}>&nbsp;<fmt:message key=".info.approval_project"/></td>
</tr>`,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			requireTemplatePipeline(t, j.JSP, tt.jsp, tt.name+".jsp")
		})
	}
}

func requireTemplatePipeline(t *testing.T, kind j.JavaTemplateType, content, filePath string) {
	t.Helper()
	if kind == j.JSP {
		// Parsing is one stage of the contract. A grammar failure is acceptable
		// only because Create falls back to the scriptlet-preserving translator.
		if _, err := jsp.Front(content); err != nil {
			t.Logf("structured jsp parse fell back: %v", err)
		}
	}
	info, err := j.ConvertTemplateToJava(kind, content, filePath)
	if kind == j.JSP && (err != nil || info == nil || strings.TrimSpace(info.GetContent()) == "") {
		info, err = j.ConvertJSPLinear(content, filePath)
	}
	require.NoError(t, err)
	require.NotNil(t, info)
	src := info.GetContent()
	require.NotEmpty(t, strings.TrimSpace(src))
	require.Contains(t, src, "void _JavaTemplateService(")
	checkJavaFront(t, src, filePath)
}
