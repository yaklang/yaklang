package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveRecoverySuggestions_VarBlock(t *testing.T) {
	msg := `no viable alternative at input 'var(\nflowCount='`
	line := `    flowCount   = sync.NewMap()`
	sugs := deriveRecoverySuggestions(msg, line)
	require.NotEmpty(t, sugs)
	block := formatRecoveryNextStepBlock(sugs)
	assert.Contains(t, block, "【下一步·强制】")
	assert.Contains(t, block, "grep_yaklang_samples")
	assert.Contains(t, block, "sync")
}

func TestDeriveRecoverySuggestions_ValueUndefined(t *testing.T) {
	sugs := deriveRecoverySuggestions("Value undefined: poc.HTTPEx", "")
	require.NotEmpty(t, sugs)
	kinds := map[string]bool{}
	for _, s := range sugs {
		kinds[s.Kind] = true
	}
	assert.True(t, kinds["grep"])
	assert.True(t, kinds["yakdoc_function"] || kinds["yakdoc_search"])
	block := formatRecoveryNextStepBlock(sugs)
	assert.Contains(t, block, "grep_yaklang_samples")
}

func TestDeriveRecoverySuggestions_ExternLib(t *testing.T) {
	sugs := deriveRecoverySuggestions(`ExternLib [poc] don't has [timeout]`, "")
	require.NotEmpty(t, sugs)
	block := formatRecoveryNextStepBlock(sugs)
	assert.Contains(t, block, "yakdoc_function_details")
	assert.Contains(t, block, `"library":"poc"`)
	assert.Contains(t, block, "grep_yaklang_samples")
}

func TestGetIntelligentErrorHint_IncludesRecovery(t *testing.T) {
	code := `
yakit.AutoInitYakit()
var (
    flowCount = sync.NewMap()
)
`
	errorMsg, blocking := checkCodeAndFormatErrors(code)
	require.True(t, blocking)
	assert.Contains(t, errorMsg, "【下一步·强制】")
	assert.Contains(t, errorMsg, "grep_yaklang_samples")
}

func TestCollectGrepPatternsFromErrMsg(t *testing.T) {
	errMsg := `[Error]: 基础语法错误（Syntax Error）：no viable alternative at input 'var(\nflowCount=' in [3:1 -- 3:8] from compiler
`
	code := "a=1\nvar (\n    flowCount = sync.NewMap()\n)\n"
	patterns := collectGrepPatternsFromErrMsg(errMsg, code)
	require.NotEmpty(t, patterns)
	joined := strings.Join(patterns, " ")
	assert.Contains(t, joined, "sync")
}

func TestDeriveRuntimeRecoverySuggestions_CannotFindMethod(t *testing.T) {
	errText := `cannot find built-in method FirstHTTPRequestBytes of slice type`
	code := `
func runSelfTest() {
    freq = mutate.GetFirstFuzzHTTPRequest(raw)
    b = freq.FirstHTTPRequestBytes
}
`
	sugs := deriveRuntimeRecoverySuggestions(errText, "", code)
	require.NotEmpty(t, sugs)
	block := formatRecoveryNextStepBlock(sugs)
	assert.Contains(t, block, "【下一步·强制】")
	assert.Contains(t, block, "grep_yaklang_samples")
	joined := ""
	for _, s := range sugs {
		joined += s.Pattern + " " + s.Func + " "
	}
	assert.True(t, strings.Contains(joined, "FirstHTTPRequestBytes") || strings.Contains(joined, "GetFirstFuzzHTTPRequest") || strings.Contains(joined, "FuzzHTTP"),
		"expected fuzz-related pattern, got %q", joined)
}

func TestEnrichRunFailureWithRecovery(t *testing.T) {
	base := "YAK_MAIN 自测运行失败。\n--- runtime error ---\ncannot find built-in method RequestRaw of slice type\n"
	code := "func runSelfTest() {\n  x = mutate.GetFirstFuzzHTTPRequest(r)\n  _ = x.RequestRaw\n}\n"
	out := enrichRunFailureWithRecovery(base, code, nil)
	assert.Contains(t, out, "【下一步·强制】")
	assert.Contains(t, out, "grep_yaklang_samples")
}

func TestNeedsSampleResearch_RunFailed(t *testing.T) {
	loop := mapGet{loopVarYakRunOK: "false"}
	assert.True(t, needsSampleResearch(loop))
	covered, _ := GrepAlreadyCovered(loop, "servicescan\\.Scan")
	assert.False(t, covered)
}

type mapGet map[string]string

func (m mapGet) Get(k string) string { return m[k] }
