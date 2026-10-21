package test

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
)

// javaFromJSP mirrors the compile prepare step: structured JSP translation, then
// the linear translation, and the result must be Java that java2ssa accepts.
func javaFromJSP(t *testing.T, content, path string) string {
	t.Helper()
	info, err := tj.ConvertTemplateToJava(tj.JSP, content, path)
	require.NoError(t, err)
	require.NotNil(t, info)
	src := info.GetContent()
	if _, err := java2ssa.Frontend(src); err != nil {
		linear, lerr := tj.ConvertJSPLinear(content, path)
		require.NoError(t, lerr)
		require.NotNil(t, linear)
		src = linear.GetContent()
	}
	_, err = java2ssa.Frontend(src)
	require.NoError(t, err, "jsp java did not parse: %s", path)
	require.Contains(t, src, "void _JavaTemplateService(")
	return src
}

func TestJSPPatternsParseWithJava2SSA(t *testing.T) {
	cases := []struct {
		name string
		jsp  string
	}{
		{
			name: "el function call",
			jsp:  `<input value="${fn:escapeXml(!empty param.password ? param.password : param.pass)}">`,
		},
		{
			name: "literal set and raw test",
			jsp: `<c:set var="skipKeys" value="|uid|user|password|"/>
<c:if test="ajc.result.code == 'FA_SECRITY'">
  <c:redirect url="${ajc.exitURL}"/>
</c:if>`,
		},
		{
			name: "choose when otherwise",
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
			name: "include then custom tag",
			jsp: `<%@ include file="/WEB-INF/support/XJS/mainPrelude.jspf" %>
<wm:i18n category="main"/>
<c:if test="${panelHTML == null}">standalone</c:if>`,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			javaFromJSP(t, tt.jsp, tt.name+".jsp")
		})
	}
}

func TestCoremailJSPFilesParseWithJava2SSA(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("YAK_JSP_CORPUS"))
	list := strings.TrimSpace(os.Getenv("YAK_JSP_LIST"))
	if root == "" || list == "" {
		t.Skip("YAK_JSP_CORPUS and YAK_JSP_LIST are not set")
	}
	raw, err := os.ReadFile(list)
	require.NoError(t, err)
	var failed []string
	checked := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		checked++
		path := filepath.Join(root, line)
		body, err := os.ReadFile(path)
		if err != nil {
			failed = append(failed, line+": read: "+err.Error())
			continue
		}
		info, err := tj.ConvertTemplateToJava(tj.JSP, string(body), line)
		if err != nil || info == nil {
			failed = append(failed, line+": convert")
			continue
		}
		src := info.GetContent()
		if _, err := java2ssa.Frontend(src); err != nil {
			linear, lerr := tj.ConvertJSPLinear(string(body), line)
			if lerr != nil || linear == nil {
				failed = append(failed, line+": linear")
				continue
			}
			src = linear.GetContent()
		}
		if _, err := java2ssa.Frontend(src); err != nil {
			msg := err.Error()
			if len(msg) > 160 {
				msg = msg[:160]
			}
			failed = append(failed, line+": java: "+msg)
		}
	}
	require.Greater(t, checked, 0)
	if len(failed) > 0 {
		limit := failed
		if len(limit) > 20 {
			limit = limit[:20]
		}
		t.Fatalf("%d/%d jsp files did not parse with java2ssa:\n%s", len(failed), checked, strings.Join(limit, "\n"))
	}
}

func TestJarClassesParseWithJava2SSA(t *testing.T) {
	jarPath := strings.TrimSpace(os.Getenv("YAK_JAR_SAMPLE"))
	if jarPath == "" {
		jarPath = "/home/wlz/Target/coremail/webapp/WEB-INF/lib/jstl-api-1.2.jar"
	}
	if _, err := os.Stat(jarPath); err != nil {
		t.Skip(jarPath)
	}
	reader, err := zip.OpenReader(jarPath)
	require.NoError(t, err)
	defer reader.Close()

	checked := 0
	var failed []string
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(file.Name, ".class") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			failed = append(failed, file.Name+": open")
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			failed = append(failed, file.Name+": read")
			continue
		}
		class, err := javaclassparser.Parse(body)
		if err != nil {
			failed = append(failed, file.Name+": parse class: "+err.Error())
			continue
		}
		source, err := class.Dump()
		if err != nil {
			failed = append(failed, file.Name+": dump: "+err.Error())
			continue
		}
		checked++
		if _, err := java2ssa.Frontend(source); err != nil {
			msg := err.Error()
			if len(msg) > 160 {
				msg = msg[:160]
			}
			failed = append(failed, file.Name+": java: "+msg)
		}
	}
	require.Greater(t, checked, 0)
	if len(failed) > 0 {
		limit := failed
		if len(limit) > 15 {
			limit = limit[:15]
		}
		t.Fatalf("%d jar classes did not parse with java2ssa (%d checked):\n%s", len(failed), checked, strings.Join(limit, "\n"))
	}
	t.Logf("parsed %d classes from %s", checked, jarPath)
}
