package sfpattern

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// runSourceRule compiles a mode=source rule and runs it over files.
func runSourceRule(t *testing.T, ruleContent string, files map[string]string) *sfvm.SFFrameResult {
	t.Helper()
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))
	result, err := ExecFrame(frame, files)
	require.NoError(t, err)
	return result
}

func alertValues(t *testing.T, result *sfvm.SFFrameResult) []string {
	t.Helper()
	var out []string
	if result == nil || result.AlertSymbolTable == nil {
		return out
	}
	result.AlertSymbolTable.ForEach(func(_ string, vals sfvm.Values) bool {
		_ = vals.Recursive(func(v sfvm.ValueOperator) error {
			out = append(out, v.String())
			return nil
		})
		return true
	})
	return out
}

const insideRule = `
desc(
	mode: "source",
	language: "general",
	title: "inside test",
	alert_min: 1,
)
${*}.pattern_regex(/(?m)^env:[\s\S]*?(?=^[^\s#]|\z)/) as $e
${*}.pattern_regex(/\$\{\{\s*secrets\./) as $t
$t inside $e as $hit
alert $hit for {
	level: "middle",
	message: "secret in env block",
}
`

func TestInsideOperator_Containment(t *testing.T) {
	files := map[string]string{
		"workflow.yml": `env:
  # ruleid
  TOKEN: ${{ secrets.MY_TOKEN }}

jobs:
  build:
    env:
      # ok: inside jobs, not env
      JOB_TOKEN: ${{ secrets.JOB_SECRET }}
`,
	}
	result := runSourceRule(t, insideRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Contains(t, vals[0], "secrets.")
}

func TestInsideOperator_NoMatch(t *testing.T) {
	files := map[string]string{
		"workflow.yml": `jobs:
  build:
    env:
      JOB_TOKEN: ${{ secrets.JOB_SECRET }}
`,
	}
	result := runSourceRule(t, insideRule, files)
	require.False(t, HasAlert(result))
}

const notInsideRule = `
desc(
	mode: "source",
	language: "general",
	title: "not-inside test",
	alert_min: 1,
)
${*}.pattern_regex(/\bws:\/\//) as $t
${*}.pattern_regex(/\bws:\/\/localhost.*/) as $j
$t not_inside $j as $hit
alert $hit for {
	level: "middle",
	message: "insecure websocket",
}
`

func TestNotInsideOperator(t *testing.T) {
	files := map[string]string{
		"app.js": `var a = "ws://evil.example/";
var b = "ws://localhost:27017/x";
var c = "wss://secure/";
`,
	}
	result := runSourceRule(t, notInsideRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Equal(t, "ws://", vals[0])
}

const andRule = `
desc(
	mode: "source",
	language: "general",
	title: "and test",
	alert_min: 1,
)
${*}.pattern_regex(/\bAP[\dABCDEF][a-zA-Z0-9]{8,}/) as $p0
${*}.pattern_regex(/(?i).*arti[-_]?factory.*/) as $p1
$p0 & $p1 as $hit
alert $hit for {
	level: "middle",
	message: "artifactory token",
}
`

func TestAmpOverlap_AND(t *testing.T) {
	files := map[string]string{
		"leak.txt": `artifactory token: AP1234567890abcdef
`,
		"other.txt": `token: AP1234567890abcdef
`,
	}
	result := runSourceRule(t, andRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Contains(t, vals[0], "AP1234567890abcdef")
}

const minusRule = `
desc(
	mode: "source",
	language: "general",
	title: "minus test",
	alert_min: 1,
)
${*}.pattern_regex(/\bAKIA[0-9A-Z]{16}\b/) as $p0
${*}.pattern_regex(/(?i)example|sample|test|fake/) as $p1
$p0 - $p1 as $hit
alert $hit for {
	level: "middle",
	message: "aws key",
}
`

func TestMinusOverlap_NotRegex(t *testing.T) {
	files := map[string]string{
		"leak.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
`,
		"real.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7REALXYZ
`,
	}
	result := runSourceRule(t, minusRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Contains(t, vals[0], "AKIAIOSFODNN7REALXYZ")
}

const doubleInsideRule = `
desc(
	mode: "source",
	language: "general",
	title: "double inside test",
	alert_min: 1,
)
${*}.pattern_regex(/sql"[^"]*"/) as $sql
${*}.pattern_regex(/import\s+slick\.[\s\S]*/) as $imp
${*}.pattern_regex(/\#\$/) as $t
$t inside $sql as $a
$a inside $imp as $hit
alert $hit for {
	level: "middle",
	message: "slick sql non-literal",
}
`

func TestInsideChain_ANDContexts(t *testing.T) {
	files := map[string]string{
		"app.scala": `import slick.jdbc.H2Profile.api._

class Foo {
  def f(name: String) = {
    val a = sql"select * from #$name".as[Int]
    val b = sql"select * from $name".as[Int]
  }
}
`,
	}
	result := runSourceRule(t, doubleInsideRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Contains(t, vals[0], "#$")
}
