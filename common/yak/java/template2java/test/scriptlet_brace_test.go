package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	j "github.com/yaklang/yaklang/common/yak/java/template2java"
)

func TestScriptletOrphanCloseStaysInsideMethod(t *testing.T) {
	src := `<c:out value="a"/>
<%}%>
<p>after</p>`
	info, err := j.ConvertTemplateToJava(j.JSP, src, "orphan.jsp")
	require.NoError(t, err)
	code := info.GetContent()
	_, err = java2ssa.Frontend(code)
	require.NoError(t, err, code)
	require.Contains(t, code, "after")
	// The page must still be inside _JavaTemplateService, not after the class.
	method := strings.Index(code, "_JavaTemplateService")
	after := strings.Index(code, "after")
	require.Greater(t, after, method)
}

func TestCOutLiteralMarkupParses(t *testing.T) {
	for _, src := range []string{
		`<c:out value="<soap:Body>"/>`,
		`<c:out value="</soap:Header>"/>`,
		`<c:out value="****"/>`,
		`<c:out value="${name} - "/>`,
	} {
		info, err := j.ConvertTemplateToJava(j.JSP, src, "lit.jsp")
		require.NoError(t, err, src)
		_, err = java2ssa.Frontend(info.GetContent())
		require.NoError(t, err, src+"\n"+info.GetContent())
	}
}

func TestEmptyJSPIsACompilationUnit(t *testing.T) {
	info, err := j.ConvertTemplateToJava(j.JSP, "", "nav.jsp")
	require.NoError(t, err)
	_, err = java2ssa.Frontend(info.GetContent())
	require.NoError(t, err, info.GetContent())
}

func TestScriptletIfElseStillParses(t *testing.T) {
	src := `<% if (ready) { %>
<p>yes</p>
<% } else { %>
<p>no</p>
<% } %>`
	info, err := j.ConvertTemplateToJava(j.JSP, src, "ifelse.jsp")
	require.NoError(t, err)
	code := info.GetContent()
	_, err = java2ssa.Frontend(code)
	require.NoError(t, err, code)
	require.Contains(t, code, "if (ready)")
	require.Contains(t, code, "else")
}

func TestForEachWithoutVarKeepsBodyParsable(t *testing.T) {
	src := `<c:forEach items="${rows}"><p>kept-row</p></c:forEach>`
	info, err := j.ConvertTemplateToJava(j.JSP, src, "rows.jsp")
	require.NoError(t, err)
	code := info.GetContent()
	require.Contains(t, code, "kept-row")
	require.NotContains(t, code, "for (Object")
	_, err = java2ssa.Frontend(code)
	require.NoError(t, err, code)
}

func TestMissingEqualsParenBeforeOr(t *testing.T) {
	src := `<%
if("int".equals(p.getString("type")) || "long".equals(p.getString("type")|| "double".equals(p.getString("type"))){
}
%>`
	info, err := j.ConvertTemplateToJava(j.JSP, src, "rest.jsp")
	require.NoError(t, err)
	code := info.GetContent()
	require.Contains(t, code, `getString("type"))||`)
	_, err = java2ssa.Frontend(code)
	require.NoError(t, err, code)
}
