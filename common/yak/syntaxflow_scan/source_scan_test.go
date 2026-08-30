package syntaxflow_scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func writeTaskLocalRuleInputFile(
	t *testing.T,
	rules []*ypb.SyntaxFlowRuleInput,
	metadata map[string]ssaconfig.TaskLocalRuleMetadata,
) (path string, sha256Hex string) {
	t.Helper()

	payload, err := json.Marshal(ssaconfig.TaskLocalRuleInputFile{
		Version:  ssaconfig.TaskLocalRuleInputFileVersionV1,
		Rules:    rules,
		Metadata: metadata,
	})
	require.NoError(t, err)

	path = filepath.Join(t.TempDir(), "task-local-rules.json")
	require.NoError(t, os.WriteFile(path, payload, 0o600))
	sum := sha256.Sum256(payload)
	return path, hex.EncodeToString(sum[:])
}

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

func TestInitByConfig_SourceTargetDefaultsToSourceMode(t *testing.T) {
	const sourceRuleName = "source-rule.sf"
	const ssaRuleName = "ssa-rule.sf"
	inputPath, inputSHA := writeTaskLocalRuleInputFile(t,
		[]*ypb.SyntaxFlowRuleInput{
			{
				RuleName: sourceRuleName,
				Content: `desc(mode: "source", language: general, title: src)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
			{
				RuleName: ssaRuleName,
				Content: `desc(mode: "ssa", language: general, title: ssa)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
		},
		map[string]ssaconfig.TaskLocalRuleMetadata{
			sourceRuleName: {AssetID: "asset-source"},
			ssaRuleName:    {AssetID: "asset-ssa"},
		},
	)

	configJSON, err := json.Marshal(map[string]any{
		"Mode": int(ssaconfig.ModeSyntaxFlowScan),
		"SyntaxFlowRule": map[string]any{
			"task_local":              true,
			"task_local_input_file":   inputPath,
			"task_local_input_sha256": inputSHA,
			"task_local_input_count":  2,
		},
	})
	require.NoError(t, err)

	cfg, err := NewConfig(
		ssaconfig.WithJsonRawConfig(configJSON),
		WithSourceFiles("source-only", map[string]string{
			"leak.env": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		}),
	)
	require.NoError(t, err)
	require.Empty(t, cfg.GetRuleFilterMode())

	task, err := createSyntaxflowTaskById(context.Background(), "", uuid.NewString(), cfg)
	require.NoError(t, err)
	require.Equal(t, int64(1), task.GetTotalQuery(),
		"a source target must default task-local rules to source mode")
}

func TestInitByConfig_TaskLocalRulesApplyModeFilter(t *testing.T) {
	const sourceRuleName = "source-rule.sf"
	const ssaRuleName = "ssa-rule.sf"
	inputPath, inputSHA := writeTaskLocalRuleInputFile(t,
		[]*ypb.SyntaxFlowRuleInput{
			{
				RuleName: sourceRuleName,
				Content: `desc(mode: "source", language: general, title: src)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
			{
				RuleName: ssaRuleName,
				Content: `desc(mode: "ssa", language: general, title: ssa)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
		},
		map[string]ssaconfig.TaskLocalRuleMetadata{
			sourceRuleName: {AssetID: "asset-source"},
			ssaRuleName:    {AssetID: "asset-ssa"},
		},
	)

	configJSON, err := json.Marshal(map[string]any{
		"Mode": int(ssaconfig.ModeSyntaxFlowScan),
		"SyntaxFlowRule": map[string]any{
			"task_local":              true,
			"task_local_input_file":   inputPath,
			"task_local_input_sha256": inputSHA,
			"task_local_input_count":  2,
		},
	})
	require.NoError(t, err)

	cfg, err := NewConfig(
		ssaconfig.WithJsonRawConfig(configJSON),
		WithSourceFiles("src-only", map[string]string{
			"leak.env": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		}),
		ssaconfig.WithRuleFilterMode("source"),
	)
	require.NoError(t, err)

	task, err := createSyntaxflowTaskById(context.Background(), "", uuid.NewString(), cfg)
	require.NoError(t, err)
	require.Equal(t, int64(1), task.GetTotalQuery(),
		"task-local source scan should keep only source-mode rules for one source target")
}

func TestStartScan_TaskLocalRulesWithModeFilter(t *testing.T) {
	const sourceRuleName = "source-rule.sf"
	const ssaRuleName = "ssa-rule.sf"
	inputPath, inputSHA := writeTaskLocalRuleInputFile(t,
		[]*ypb.SyntaxFlowRuleInput{
			{
				RuleName: sourceRuleName,
				Content: `desc(mode: "source", language: general, title: src)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
			{
				RuleName: ssaRuleName,
				Content: `desc(mode: "ssa", language: general, title: ssa)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			},
		},
		map[string]ssaconfig.TaskLocalRuleMetadata{
			sourceRuleName: {AssetID: "asset-source"},
			ssaRuleName:    {AssetID: "asset-ssa"},
		},
	)

	configJSON, err := json.Marshal(map[string]any{
		"Mode": int(ssaconfig.ModeSyntaxFlowScan),
		"SyntaxFlowRule": map[string]any{
			"task_local":              true,
			"task_local_input_file":   inputPath,
			"task_local_input_sha256": inputSHA,
			"task_local_input_count":  2,
		},
	})
	require.NoError(t, err)

	var alerts int
	err = StartScan(context.Background(),
		ssaconfig.WithJsonRawConfig(configJSON),
		WithSourceFiles("src-only", map[string]string{
			"leak.env": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		}),
		ssaconfig.WithRuleFilterMode("source"),
		WithScanResultCallback(func(r *ScanResult) {
			if r == nil || r.Result == nil {
				return
			}
			alerts += len(r.Result.GetAlertVariables())
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0, "task-local source-mode rules should produce findings on raw source files")
}
