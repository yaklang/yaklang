package sfvm_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func CheckFormatDesc(t *testing.T, input string, expected string) {
	result, err := sfvm.FormatRule(input, sfvm.RuleFormatWithRuleID("id"))
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
func TestCheckFormatRule(t *testing.T) {
	content := "desc(\n\ttitle: \"Audit Golang HTTP Source\"\n\ttype: audit\n\tlevel: info\n\tlib: \"golang-http-source\"\n\tdesc: <<<DESC\n### 规则目的\n\n该规则旨在审计Golang应用程序中与处理HTTP响应输出相关的代码。\n\n### 规则详细\n\n1.  **作为基础审计库**\n    属于 `golang-http-source` 库规则（`lib` 类型），通常配合其他规则（如数据源跟踪规则）共同审计潜在的Web漏洞，提供关键HTTP请求输入的识别能力。\nDESC\n\ttitle_zh: \"审计Golang HTTP输入点\"\n\tsolution: <<<SOLUTION\nnone\nSOLUTION\n\treference: <<<REFERENCE\nnone\nREFERENCE\n\trule_id: \"e5a96a40-e3fb-4903-8d27-281d77a5b753\"\n)\n\n\n\nhttp.Request.URL.Query().Get() as $output \n\n\nalert $output\ndesc(\n\tlang: golang\n\talert_min: 1\n\t'file://http_net.go': <<<PARAM\npackage main\n\nimport (\n\t\"net/http\"\n\t\"html/template\"\n)\n\nfunc handler(w http.ResponseWriter, r *http.Request) {\n\t// 从查询参数中获取用户输入\n\tname := r.URL.Query().Get(\"name\")\n\n\t// 直接将用户输入插入到 HTML 中\n\ttmpl := \"<h1>Hello,\" + name + \"!</h1>\"\n\tw.Write([]byte(tmpl))\n}\n\nfunc main() {\n\thttp.HandleFunc(\"/\", handler)\n\thttp.ListenAndServe(\":8080\", nil)\n}\n\nPARAM\n)\n"
	rule, err := sfvm.FormatRule(content)
	require.NoError(t, err)
	_, err = sfvm.CompileRule(rule)
	require.NoError(t, err)
}

func TestFormatDesc(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	t.Run("test empty description", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
`,
			`
desc(
	rule_id: "id"
)
`,
		)
	})

	t.Run("test description ", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
	Title: "title",
)
`,
			`
desc(
	Title: "title"
	rule_id: "id"
)
`,
		)
	})

	t.Run("test description with rule id not add ", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
	Title: "title",
	rule_id: "id",
)
`,
			`
desc(
	Title: "title"
	rule_id: "id"
)
`,
		)
	})

	t.Run("test normal", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
a #-> *as  b
`,
			`
desc(
	rule_id: "id"
)
a #-> *as  b
`,
		)
	})

	t.Run("multiple desc", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
desc()
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})

	t.Run("multiple desc with {}", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
desc{}
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})

	t.Run("multiple desc line in empty desc", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
)
desc{
}
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})
}
func TestName(t *testing.T) {

}

func TestRealFormatCheck(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	t.Run("duplicate desc", func(t *testing.T) {

	})

	t.Run("with format string", func(t *testing.T) {
		rule := `
desc(
	rule_id: "id"
	desc: <<<CODE
Test Rule ` + "`%s`" + ` t
CODE
)
`
		CheckFormatDesc(t, rule, rule)
	})

}

func TestAlert(t *testing.T) {
	t.Run("normal alert with desc", func(t *testing.T) {
		CheckFormatDesc(t, `
desc()
alert $output
`, `
desc(
	rule_id: "id"
)
alert $output
`)
	})

	t.Run("alert with desc sort", func(t *testing.T) {
		rule := `
desc(
	rule_id: "id"
)

alert $output for {
	title: "a",
	level: "high",
	desc: <<<DESC
This is a test alert description .
DESC
}
`
		CheckFormatDesc(t, rule, rule)
	})

}

func Test_FormatString(t *testing.T) {
	t.Run("alert with desc with format string  ", func(t *testing.T) {
		rule := `
desc(
	rule_id: "id"
)

alert $output for {
	title: "a",
	level: "high",
	desc: <<<DESC
description: %s .
DESC
}
`
		CheckFormatDesc(t, rule, rule)
	})
}
