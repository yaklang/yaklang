package test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"testing"
)

func TestJSP2Java_Content(t *testing.T) {
	tests := []struct {
		name    string
		jspCode string
		wants   []string
	}{
		{"test  JspElementWithOpenTagOnly pure text  ", "<html>", []string{"out = request.getOut(); ", `out.write("<html>")`}},
		{"test JspElementWithClosingTagOnly pure text  ", "<html/>", []string{`out.write("<html/>");`}},
		{"test JspElementWithTagAndContent pure text  ", "<title>hello</title>", []string{`out.write("<title>hello</title>");`}},
		{"test jstl-core out tag", "<c:out value='${name}'/>", []string{`name = request.getAttribute("name");`, `out.print(escapeHtml(name));`}},
		{"test jstl-core out tag without escaping", "<c:out value='${name}' escapeXml=\"false\"/>", []string{`name = request.getAttribute("name");`, `out.print(name);`}},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := tj.ConvertTemplateToJava(tj.JSP, jspCode, "test.jsp")
		require.NoError(t, err)
		require.NotNil(t, codeInfo)
		require.NotEqual(t, 0, len(wants))
		fmt.Print(codeInfo.GetContent())
		checkJavaFront(t, codeInfo.GetContent())
		for _, want := range wants {
			require.Contains(t, codeInfo.GetContent(), want)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(tt.jspCode, tt.wants)
		})
	}
}
