package syntaxflow_scan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRulePreparationSharedAcrossProductStages(t *testing.T) {
	var options []ssaconfig.Option
	for _, mode := range []string{"source", "struct", "ssa"} {
		options = append(options, ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "prepared-" + mode, Language: "general",
			Content: fmt.Sprintf("desc(mode: %q, title: %q) * as $hit", mode, mode),
		}))
	}
	cfg, err := NewConfig(options...)
	require.NoError(t, err)
	rules := cfg.customRules()
	require.Len(t, rules, 3)
	for _, stage := range []string{"source", "struct", "ssa"} {
		t.Run(stage, func(t *testing.T) {
			child, err := NewConfig(sharedScanCallbackOptions(cfg)...)
			require.NoError(t, err)
			prepared, _, err := child.preparedRuleInputs()
			require.NoError(t, err)
			require.Len(t, prepared, 3)
			for i := range rules {
				require.Same(t, rules[i], prepared[i], "do not reparse or discard compiled opcodes at stage boundaries")
				require.NotEmpty(t, prepared[i].OpCodes)
			}
			require.Same(t, cfg.rulePreparationState(), child.rulePreparationState())
		})
	}
	independent, err := NewConfig(options...)
	require.NoError(t, err)
	require.NotSame(t, rules[0], independent.customRules()[0], "different scans must not share mutable rule metadata")
}

func TestRulePreparationInvalidInputDoesNotPoisonOthers(t *testing.T) {
	cfg, err := NewConfig(
		ssaconfig.WithRuleInputRaw(""),
		ssaconfig.WithRuleInputRaw(`desc(mode: "source", title: "valid") ${*}.regexp(/key/) as $hit`),
	)
	require.NoError(t, err)
	rules := cfg.customRules()
	require.Len(t, rules, 1)
	require.Equal(t, "valid", rules[0].Title)
}
