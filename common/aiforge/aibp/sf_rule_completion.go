package aibp

import (
	"context"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	err := aiforge.RegisterForgeExecutor("sf_rule_completion", sfRuleCompletionExecutor)
	if err != nil {
		log.Errorf("register sf_rule_completion failed: %v", err)
		return
	}
}

func sfRuleCompletionExecutor(ctx context.Context, items []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
	ruleName := aiforge.GetCliValueByKey("rule_name", items)
	if ruleName == "" {
		ruleName = aiforge.GetCliValueByKey("query", items)
	}
	if ruleName == "" {
		return nil, utils.Errorf("rule_name or query is required")
	}

	var rule *schema.SyntaxFlowRule
	for r := range syntaxflow.QuerySyntaxFlowRulesByKeyword(ruleName) {
		rule = r
		break
	}
	if rule == nil {
		return nil, utils.Errorf("no rule found for keyword: %s", ruleName)
	}
	fileName := rule.RuleName
	fileContent := rule.Content
	if fileContent == "" {
		return nil, utils.Errorf("rule %s has empty content", fileName)
	}

	descResult, err := aiforge.ExecuteForge(
		"sf_desc_completion",
		ctx,
		[]*ypb.ExecParamItem{
			{Key: "file_name", Value: fileName},
			{Key: "file_content", Value: fileContent},
		},
		opts...,
	)
	if err != nil {
		return nil, utils.Wrapf(err, "sf_desc_completion failed")
	}
	alertResult, err := aiforge.ExecuteForge(
		"sf_alert_completion",
		ctx,
		[]*ypb.ExecParamItem{
			{Key: "file_name", Value: fileName},
			{Key: "file_content", Value: fileContent},
		},
		opts...,
	)
	if err != nil {
		return nil, utils.Wrapf(err, "sf_alert_completion failed")
	}

	descParams := descResult.GetInvokeParams("params")
	alertParams := alertResult.GetInvokeParams("params")
	if descParams == nil {
		return nil, utils.Errorf("sf_desc_completion returned no params")
	}

	descHandler := func(key, value string) string {
		if typ := sfvm.ValidDescItemKeyType(key); typ == sfvm.SFDescKeyType_Unknown {
			return value
		}
		if got := descParams.GetString(key); got != "" {
			return got
		}
		if got := descParams.GetInt(key); got != 0 {
			return strconv.FormatInt(got, 10)
		}
		return value
	}
	normalizeAlertName := func(s string) string {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "$") {
			return s[1:]
		}
		return s
	}
	alertHandler := func(name, key, value string) string {
		array := alertParams.GetObjectArray("alert")
		if len(array) == 0 {
			return value
		}
		nameNorm := normalizeAlertName(name)
		for _, invokeParams := range array {
			aiName := normalizeAlertName(invokeParams.GetString("name"))
			if nameNorm != aiName {
				continue
			}
			aiVal := invokeParams.GetString(key)
			if aiVal == "" {
				return value
			}
			return aiVal
		}
		return value
	}

	merged, err := sfvm.FormatRule(fileContent,
		sfvm.RuleFormatWithRequireInfoDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(descHandler),
		sfvm.RuleFormatWithAlertHandler(alertHandler),
		sfvm.RuleFormatWithRequireAlertDescKeyType(sfvm.GetAlertDescKeyType()...),
	)
	if err != nil {
		return nil, utils.Wrapf(err, "format rule failed")
	}

	writeBack := strings.EqualFold(aiforge.GetCliValueByKey("write_back", items), "true")
	if writeBack {
		if rule.IsBuildInRule {
			log.Warnf("skip write_back for built-in rule: %s", rule.RuleName)
		} else if err := sfdb.UpdateRuleContent(rule.RuleName, merged); err != nil {
			return nil, utils.Wrapf(err, "write rule content back to database failed")
		}
	}

	action := aicommon.NewSimpleAction("sf_rule_completion", aitool.InvokeParams{
		"params": aitool.InvokeParams{
			"rule_content": merged,
		},
	})
	return &aiforge.ForgeResult{Action: action}, nil
}
