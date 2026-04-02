package test

import (
	"embed"
	"github.com/stretchr/testify/require"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"io/fs"
	"strings"
	"testing"
	"time"
)

//go:embed all:testdata
var testdataFS embed.FS

func TestFreeMarker2Java_Content(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		wants []string
	}{
		{"test  freemarker pure html  ", "<body>\n <h1> hello </h1>\n</body>", []string{
			"out.write(\"<body>\");",
			"out.write(\"<h1> hello </h1>\");",
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
		{"test quoted raw text", "return \"??/query\";\n@RequestMapping(value = \"/\")", []string{
			"out.write(\"return \\\"??/query\\\";\");",
			"out.write(\"@RequestMapping(value = \\\"/\\\")\");",
		}},
		{"test quoted inline expression and single quoted condition", "<#if prop.name != 'createdBy'>\n${r\"#{item}\"}\n</#if>", []string{
			"if (prop.name!=\"createdBy\") {",
			"out.print(elExpr.parse(\"${r\\\"#{item}\\\"}\"));",
		}},
		{"test if elseif else bodies", "<#if status == \"active\">A<#elseif status == \"inactive\">B<#else>C</#if>", []string{
			"if (status==\"active\") {",
			"out.write(\"A\");",
			"} else if (status==\"inactive\") {",
			"out.write(\"B\");",
			"} else {",
			"out.write(\"C\");",
		}},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := tj.ConvertTemplateToJava(tj.Freemarker, jspCode, "test.ftl")
		require.NoError(t, err)
		require.NotNil(t, codeInfo)
		require.NotEqual(t, 0, len(wants))
		checkJavaFront(t, codeInfo.GetContent(), "test.ftl")
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

var freemarkerFixtureAssertions = map[string][]string{
	"testdata/mapper_where_clause.ftl": {
		`if (prop.name=="createdDate") {`,
		`if ((prop_index>1)) {`,
		`} else {`,
	},
	"testdata/skyeye_help.ftl": {
		`if (elExpr.parse("cookieMap?exists&&cookieMap[\"xxljob_adminlte_settings\"]?exists&&\"off\"==cookieMap[\"xxljob_adminlte_settings\"].value")) {`,
	},
}

func validateSavedFreeMarkerFixture(t *testing.T, filePath string, content string) {
	t.Helper()

	start := time.Now()
	codeInfo, err := tj.ConvertTemplateToJava(tj.Freemarker, content, filePath)
	convertDur := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, codeInfo)
	require.LessOrEqual(t, convertDur, generatedJavaFixtureMaxParseDuration, "template to java took too long for %s", filePath)
	checkJavaFront(t, codeInfo.GetContent(), filePath)
	for _, want := range freemarkerFixtureAssertions[filePath] {
		require.Contains(t, codeInfo.GetContent(), want, "want: %s", want)
	}
}

func TestAllSavedFreeMarkerTemplates(t *testing.T) {
	found := false
	err := fs.WalkDir(testdataFS, "testdata", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(filePath), ".ftl") {
			return nil
		}

		raw, err := testdataFS.ReadFile(filePath)
		require.NoError(t, err)
		t.Run(filePath, func(t *testing.T) {
			validateSavedFreeMarkerFixture(t, filePath, string(raw))
		})
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no saved freemarker templates found")
}
