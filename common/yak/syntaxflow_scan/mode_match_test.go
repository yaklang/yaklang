package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRuleMatchesQueryTarget(t *testing.T) {
	sourceTarget := ssaapi.NewSourceQueryTarget("src", map[string]string{"a.env": "x"})
	prog, err := ssaapi.Parse("a = 1")
	require.NoError(t, err)

	sourceRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SOURCE}
	structRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_STRUCT}
	ssaRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SSA}

	require.True(t, ruleMatchesQueryTarget(sourceRule, sourceTarget))
	require.False(t, ruleMatchesQueryTarget(structRule, sourceTarget))
	require.False(t, ruleMatchesQueryTarget(ssaRule, sourceTarget))
	// A Program is a multi-mode target: Program.Query dispatches a rule by the
	// mode it declares, so every mode is accepted there.
	require.True(t, ruleMatchesQueryTarget(sourceRule, prog))
	require.True(t, ruleMatchesQueryTarget(structRule, prog))
	require.True(t, ruleMatchesQueryTarget(ssaRule, prog))
}

func TestStartScan_SkipsSourceRuleOnSSAProgram(t *testing.T) {
	prog, err := ssaapi.Parse("a = 1")
	require.NoError(t, err)

	failed := false
	err = StartScan(context.Background(),
		WithPrograms(prog),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "skip source on ssa")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		WithErrorCallback(func(taskid, status, format string, args ...any) {
			failed = true
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.False(t, failed, "source rule on SSA program must be skipped, not failed")
}

func TestDeterministicRuleSkip(t *testing.T) {
	sourceTarget := ssaapi.NewSourceQueryTarget("src", map[string]string{"a.env": "x"})
	prog, err := ssaapi.Parse("a = 1")
	require.NoError(t, err)

	structRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_STRUCT, RuleName: "struct"}
	reason, skip := deterministicRuleSkip(false, structRule, sourceTarget)
	require.True(t, skip)
	require.Equal(t, "skipped: target-mode mismatch", reason)

	javaRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SSA, Language: ssaconfig.JAVA, RuleName: "java"}
	reason, skip = deterministicRuleSkip(false, javaRule, prog)
	require.True(t, skip)
	require.Equal(t, "skipped: language mismatch", reason)
	_, skip = deterministicRuleSkip(true, javaRule, prog)
	require.False(t, skip, "ignore-language keeps a cross-language SSA rule runnable")

	sourceRule := &schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SOURCE, Language: ssaconfig.JAVA, RuleName: "source"}
	_, skip = deterministicRuleSkip(false, sourceRule, sourceTarget)
	require.False(t, skip, "source rules stay language-agnostic and are not pre-skipped")
	_, skip = deterministicRuleSkip(false, nil, prog)
	require.False(t, skip)
}

func TestRuleFilterModesInfersFromTargets(t *testing.T) {
	cfg, err := NewConfig(WithSourceFiles("src", map[string]string{"a.txt": "x"}))
	require.NoError(t, err)
	require.Equal(t, []string{string(schema.SFR_MODE_SOURCE)}, ruleFilterModes(cfg))

	prog, err := ssaapi.Parse("a = 1")
	require.NoError(t, err)
	cfg, err = NewConfig(WithPrograms(prog))
	require.NoError(t, err)
	require.Equal(t, []string{string(schema.SFR_MODE_SSA)}, ruleFilterModes(cfg))
}
