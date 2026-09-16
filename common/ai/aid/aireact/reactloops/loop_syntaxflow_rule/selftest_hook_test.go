package loop_syntaxflow_rule

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	sfdoc "github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc/doc"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

type selfTestLoopStore struct {
	vars map[string]any
}

func (s *selfTestLoopStore) Set(k string, v any) {
	if s.vars == nil {
		s.vars = make(map[string]any)
	}
	s.vars[k] = v
}

func (s *selfTestLoopStore) Get(k string) string {
	if s.vars == nil {
		return ""
	}
	v, ok := s.vars[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func TestResetSfVerifyMatchedAfterCodeChange(t *testing.T) {
	st := &selfTestLoopStore{vars: map[string]any{
		loopVarSfVerifyMatched:      "true",
		loopVarSfVerifyLastFeedback: "old",
	}}
	resetSfVerifyMatchedAfterCodeChange(st)
	require.Equal(t, "", st.Get(loopVarSfVerifyMatched))
	require.Equal(t, "", st.Get(loopVarSfVerifyLastFeedback))
	resetSfVerifyMatchedAfterCodeChange(nil) // no panic
}

func TestApplySampleSelfTestResult_MatchedTrue(t *testing.T) {
	st := &selfTestLoopStore{}
	feedback, matched := applySampleSelfTestResult(st, &sfanalysis.SampleVerificationResult{
		Matched: true,
		Message: "ok",
	})
	require.True(t, matched)
	require.Empty(t, feedback)
	require.Equal(t, "true", st.Get(loopVarSfVerifyMatched))
	require.Equal(t, "", st.Get(loopVarSfVerifyLastFeedback))
}

func TestApplySampleSelfTestResult_MatchedFalse(t *testing.T) {
	st := &selfTestLoopStore{}
	feedback, matched := applySampleSelfTestResult(st, &sfanalysis.SampleVerificationResult{
		Matched:        false,
		Message:        "no alert",
		DiagnosticHint: "$source:0 → include missing as",
		ResultVarsDiagnostic: map[string]int{
			"$source": 0,
			"$gin":    0,
		},
		Suggestion: "fix include as $gin",
	})
	require.False(t, matched)
	require.Contains(t, feedback, "diagnostic_hint")
	require.Contains(t, feedback, "result_vars_diagnostic")
	require.Contains(t, feedback, "$source: 0")
	require.Equal(t, "false", st.Get(loopVarSfVerifyMatched))
	require.Equal(t, feedback, st.Get(loopVarSfVerifyLastFeedback))
}

func TestApplySampleSelfTestResult_NilSample(t *testing.T) {
	st := &selfTestLoopStore{}
	feedback, matched := applySampleSelfTestResult(st, nil)
	require.False(t, matched)
	require.NotEmpty(t, feedback)
	require.Equal(t, "false", st.Get(loopVarSfVerifyMatched))
}

func TestFormatSampleSelfTestFailure_Empty(t *testing.T) {
	msg := formatSampleSelfTestFailure(nil)
	require.Contains(t, msg, "正例自检未通过")
}

func TestSyntaxFlowRuleCheckingWithSample_WiresIntoApply(t *testing.T) {
	// Blank rule + sample → Sample.Matched=false; still exercises apply path.
	rule := " \n\t"
	sample := "package main\nfunc main() {}\n"
	res := static_analyzer.SyntaxFlowRuleCheckingWithSample(rule, sample, "main.go", "golang")
	require.Empty(t, res.SyntaxErrors)
	require.NotNil(t, res.Sample)
	require.False(t, res.Sample.Matched)

	st := &selfTestLoopStore{}
	feedback, matched := applySampleSelfTestResult(st, res.Sample)
	require.False(t, matched)
	require.NotEmpty(t, feedback)
	require.Equal(t, "false", st.Get(loopVarSfVerifyMatched))
}

func TestSyntaxflowdocEmbedSmoke(t *testing.T) {
	require.True(t, sfdoc.IsDocumentAvailable())
	h := sfdoc.GetDefaultDocumentHelper()
	require.Greater(t, h.Total(), 100)
	require.NotEmpty(t, h.ListNativeCallNames())
	require.NotEmpty(t, h.ListBuiltinLibNames())
	require.NotNil(t, h.GetNativeCall("include"))
	require.NotNil(t, h.GetBuiltinLib("golang-gin-context"))

	hits := sfdoc.SearchDocument("gin context include", 8, "")
	require.NotEmpty(t, hits)
}
