package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

func TestSyntaxFlowStaticAnalyzeBlankRuleHasNoDiagnostics(t *testing.T) {
	results := yak.StaticAnalyze(" \n\t", yak.WithStaticAnalyzePluginType("syntaxflow"))
	require.Empty(t, results)
}

func TestSyntaxFlowRuleCheckingWithSampleBlankRuleHasNoSyntaxErrors(t *testing.T) {
	checkResult := static_analyzer.SyntaxFlowRuleCheckingWithSample(" \n\t", "", "", "")
	require.Empty(t, checkResult.SyntaxErrors)
	require.Empty(t, checkResult.FormattedErrors)
	require.Nil(t, checkResult.Sample)
}
