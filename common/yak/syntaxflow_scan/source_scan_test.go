package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestStartScan_SourceMode_NoSSACompile(t *testing.T) {
	ruleContent := `
desc(
	mode: "source",
	language: "general",
	title: "aws akia test",
	alert_min: 1,
)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit for {
	level: "critical",
	title: "AWS key",
}
`
	files := map[string]string{
		"leak.env": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		"ok.env":   "AWS_ACCESS_KEY_ID=${FROM_VAULT}\n",
	}

	var alerts int
	err := StartScan(context.Background(),
		WithSourceFiles("src-only", files),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content:  ruleContent,
			Language: string(ssaconfig.General),
		}),
		WithScanResultCallback(func(r *ScanResult) {
			if r == nil || r.Result == nil {
				return
			}
			alerts += len(r.Result.GetAlertVariables())
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0, "expected alerts from source-mode scan without SSA")
}

func TestSourceQueryTarget_ExecRule(t *testing.T) {
	target := ssaapi.NewSourceQueryTarget("t", map[string]string{
		"a.env": "AKIAIOSFODNN7EXAMPLE",
	})
	res, err := target.SyntaxFlowWithError(`
desc(mode: "source", language: general, title: t)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit
`)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetAlertVariables())
}
