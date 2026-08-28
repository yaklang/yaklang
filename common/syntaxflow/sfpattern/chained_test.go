package sfpattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const chainedRule = `
desc(
	mode: "source",
	language: "general",
	title: "chained test",
	alert_min: 1,
)
${*}.pattern_regex(/(?m)^env:[\s\S]*?(?=^[^\s#]|\z)/) as $e
$e.regexp(/\$\{\{\s*secrets\./) as $hit
alert $hit for {
	level: "middle",
	message: "secret in env file",
}
`

func TestChainedSearch_SameFileContext(t *testing.T) {
	files := map[string]string{
		"workflow.yml": `env:
  TOKEN: ${{ secrets.MY_TOKEN }}
`,
		"other.yml": `jobs:
  build:
    env:
      JOB_TOKEN: ${{ secrets.JOB_SECRET }}
`,
	}
	result := runSourceRule(t, chainedRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Contains(t, vals[0], "secrets.")
}

func TestChainedSearch_NoContextNoHit(t *testing.T) {
	files := map[string]string{
		"other.yml": `jobs:
  build:
    env:
      JOB_TOKEN: ${{ secrets.JOB_SECRET }}
`,
	}
	result := runSourceRule(t, chainedRule, files)
	require.False(t, HasAlert(result))
}

const chainedNotRule = `
desc(
	mode: "source",
	language: "general",
	title: "chained not test",
	alert_min: 1,
)
${*}.pattern_regex(/\bws:\/\//) as $t
$t.pattern_regex_not(/\bws:\/\/localhost.*/) as $hit
alert $hit for {
	level: "middle",
	message: "insecure websocket",
}
`

func TestChainedSearch_WithNegatives(t *testing.T) {
	files := map[string]string{
		"app.js": `var a = "ws://evil.example/";
var b = "ws://localhost:27017/x";
`,
	}
	result := runSourceRule(t, chainedNotRule, files)
	require.True(t, HasAlert(result))
	vals := alertValues(t, result)
	require.Len(t, vals, 1)
	require.Equal(t, "ws://", vals[0])
}
