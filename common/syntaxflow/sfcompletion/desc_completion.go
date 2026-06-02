package sfcompletion

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak" // ensure ExecuteForge callback registered
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CompleteRule 使用 sf_rule_completion forge 补全 desc/alert，并在内存合并后返回规则正文。
func CompleteRule(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	if fileName == "" {
		return "", utils.Errorf("complete rule failed: fileName is required")
	}
	if ruleContent == "" {
		return "", utils.Errorf("complete rule failed: ruleContent is required")
	}
	result, err := aicommon.ExecuteForgeFromDB(
		"sf_rule_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "file_path", Value: fileName},
			{Key: "file_content", Value: ruleContent},
		},
		aicommon.WithAgreeYOLO(),
	)
	if err != nil {
		return "", utils.Errorf("complete rule failed: %v", err)
	}
	return mergeRuleFromForgeResult(result, ruleContent)
}

// CompleteRuleDesc 委托 CompleteRule（同时补全 desc 与 alert）。
func CompleteRuleDesc(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	return CompleteRule(fileName, ruleContent, aiConfig...)
}

func mergeRuleFromForgeResult(result any, ruleContent string) (string, error) {
	if ruleContent == "" {
		return "", utils.Errorf("merge failed: rule content is empty")
	}
	action := extractCompletionAction(result)
	if action == nil {
		return "", utils.Errorf("merge failed: forge result has no sf_rule_completion_result action")
	}
	descParams := aitool.InvokeParams{
		"title":     action.GetString("title"),
		"title_zh":  action.GetString("title_zh"),
		"desc":      action.GetString("desc"),
		"solution":  action.GetString("solution"),
		"reference": action.GetString("reference"),
		"cwe":       action.GetInt("cwe"),
	}
	alertItems := action.GetInvokeParamsArray("alert")
	if descParams.GetString("title") == "" && descParams.GetString("title_zh") == "" &&
		descParams.GetString("desc") == "" && len(alertItems) == 0 {
		return "", utils.Errorf("merge failed: completion action has empty desc/alert fields")
	}
	merged, err := syntaxflow.MergeBeautificationResults(descParams, aitool.InvokeParams{"alert": alertItems}, ruleContent)
	if err != nil {
		return "", err
	}
	if merged == "" {
		return "", utils.Errorf("merge failed: empty merged rule")
	}
	return merged, nil
}

func extractCompletionAction(result any) *aicommon.Action {
	if result == nil {
		return nil
	}
	switch v := result.(type) {
	case *aiforge.ForgeResult:
		if v != nil && v.Action != nil {
			return v.Action
		}
	case *aicommon.ForgeResult:
		if v != nil && v.Action != nil {
			return v.Action
		}
	case *aicommon.Action:
		return v
	}
	return nil
}
