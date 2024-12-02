package test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"testing"
)

func TestFreeMarker2Java_Content(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		wants []string
	}{
		{"test  freemarker pure html  ", "<body>\n <h1> hello </h1>\n</body>", []string{
			"out.write(\"<body>\");",
			"out.write(\" <h1> hello </h1>\")",
			"out.write(\"</body>\");"},
		},
		{"test freemarker el expression", "<h1>${name}</h1>", []string{
			"out.write(\"<h1>\");",
			"out.print(elExpr.parse(\"${name}\"));",
		}},
		{"test freemarker el expression with escape", "<h1>${name?html}</h1>", []string{
			"out.write(\"<h1>\");",
			"out.printWithEscape(elExpr.parse(\"${name?html}\"));",
		}},
		{"test the freemarker if stmt ", "<#if user.isAdmin>\n  <p>${user.name} 是管理员。</p></#if>", []string{
			"if (user.isAdmin) {",
			"out.print(elExpr.parse(\"${user.name}\"));",
		}},
		{"test the freemarker if else stmt ", "<#if user.isAdmin>\n  <p>${user.name} 是管理员。</p>\n<#else>\n  <p>${user.name} 不是管理员。</p>\n</#if>", []string{
			"if (user.isAdmin) {",
			"out.print(elExpr.parse(\"${user.name}\"));",
			"else",
		}},
		{"test freemarker if else-if stmt", "<#if status == \"active\">\n  <p>账户状态：活跃</p>\n<#elseif status == \"inactive\">\n  <p>账户状态：不活跃</p>\n<#elseif status == \"suspended\">\n  <p>账户状态：已挂起</p>\n<#else>\n  <p>账户状态：未知</p>\n</#if>", []string{
			"if (status==\"active\") {",
			"} else if (status==\"inactive\") {",
			"} else {",
		}},
		{"test list tag", "<#list users as user>\n  <p>${user.name}</p>\n</#list>", []string{
			"for ( Object user : elExpr.parse(\"users\")) {",
			"out.print(elExpr.parse(\"${user.name}\"));",
		}},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := tj.ConvertTemplateToJava(tj.Freemarker, jspCode, "test.ftl")
		require.NoError(t, err)
		require.NotNil(t, codeInfo)
		require.NotEqual(t, 0, len(wants))
		fmt.Print(codeInfo.GetContent())
		checkJavaFront(t, codeInfo.GetContent())
		for _, want := range wants {
			require.Contains(t, codeInfo.GetContent(), want, "want: %s", want)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(tt.code, tt.wants)
		})
	}
}
