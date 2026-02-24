package aibp

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 最小化 SF 规则模板，仅含必要骨架，关键字段留空供 AI 补全
const minimalRuleTemplate = `desc(
	title: ""
	title_zh: ""
	type: audit
	level: low
	lang: php
	desc: <<<DESC

DESC
	solution: <<<SOLUTION

SOLUTION
	reference: <<<REFERENCE

REFERENCE
)
/$_GET/ as $sink
alert $sink for {
	title: ""
	title_zh: ""
	desc: ""
}
`

func TestSFRuleCompletion(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	testRuleName := "php-sf-" + uuid.NewString() + ".sf"
	consts.InitializeYakitDatabase("", "", "")
	cb, err := aiconfig.GetLightweightAIModelCallback()
	require.NoError(t, err)

	_, err = sfdb.ImportRuleWithoutValid(testRuleName, minimalRuleTemplate, false)
	require.NoError(t, err)
	defer sfdb.DeleteRuleByRuleName(testRuleName)

	results, err := aiforge.ExecuteForge(
		"sf_rule_completion",
		context.Background(),
		[]*ypb.ExecParamItem{{Key: "rule_name", Value: testRuleName}},
		aicommon.WithAgreeYOLO(true),
		aicommon.WithAICallback(cb),
	)
	require.NoError(t, err)
	require.NotNil(t, results)

	params := results.GetInvokeParams("params")
	require.NotNil(t, params)
	ruleContent := params.GetString("rule_content")
	require.NotEmpty(t, ruleContent)

	log.Infof(ruleContent)

	// 验证 desc 块关键信息是否被补全
	require.Regexp(t, regexp.MustCompile(`desc\(\s*\n[\s\S]*?title:\s*"[^"]{2,}"`), ruleContent, "desc title 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc\(\s*\n[\s\S]*?title_zh:\s*"[^"]{2,}"`), ruleContent, "desc title_zh 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc:\s*<<<DESC\s*\n[\s\S]+?\nDESC`), ruleContent, "desc 内容应被补全")
	require.Regexp(t, regexp.MustCompile(`solution:\s*<<<SOLUTION\s*\n[\s\S]+?\nSOLUTION`), ruleContent, "solution 应被补全")

	// 验证 alert 块内容被补全（原模板中 title、title_zh、desc 为空，应由 AI 填充）
	require.Regexp(t, regexp.MustCompile(`alert\s+\$sink\s+for\s+\{`), ruleContent, "应有 alert $sink for {")
	alertBlock := regexp.MustCompile(`alert\s+\$sink\s+for\s+\{([\s\S]*?)\n\}`).FindStringSubmatch(ruleContent)
	require.NotEmpty(t, alertBlock, "应能解析出 alert 块")
	alertContent := alertBlock[1]
	require.Regexp(t, regexp.MustCompile(`title:\s*"[^"]{2,}"`), alertContent, "alert title 应被补全")
	require.Regexp(t, regexp.MustCompile(`title_zh:\s*"[^"]{2,}"`), alertContent, "alert title_zh 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc:\s*<<<DESC\s*\n[\s\S]+?\nDESC`), alertContent, "alert desc 应被补全")
}

func TestSFRuleCompletion_NotFound(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	_, err := aiforge.ExecuteForge(
		"sf_rule_completion",
		context.Background(),
		[]*ypb.ExecParamItem{{Key: "rule_name", Value: "__nonexistent_rule_xyz__"}},
		aicommon.WithAgreeYOLO(true),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no rule found")
}

func TestSFRuleCompletion_RequireRuleName(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	_, err := aiforge.ExecuteForge(
		"sf_rule_completion",
		context.Background(),
		[]*ypb.ExecParamItem{},
		aicommon.WithAgreeYOLO(true),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rule_name or query is required")
}

func TestSFRuleCompletion_WriteBack(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	testRuleName := "php-sf-" + uuid.NewString() + ".sf"
	consts.InitializeYakitDatabase("", "", "")
	cb, err := aiconfig.GetLightweightAIModelCallback()
	require.NoError(t, err)

	_, err = sfdb.ImportRuleWithoutValid(testRuleName, minimalRuleTemplate, false)
	require.NoError(t, err)
	defer sfdb.DeleteRuleByRuleName(testRuleName)

	var ruleBefore *schema.SyntaxFlowRule
	for r := range syntaxflow.QuerySyntaxFlowRulesByKeyword(testRuleName) {
		ruleBefore = r
		break
	}
	require.NotNil(t, ruleBefore)
	contentBefore := ruleBefore.Content

	results, err := aiforge.ExecuteForge(
		"sf_rule_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "rule_name", Value: testRuleName},
			{Key: "write_back", Value: "true"},
		},
		aicommon.WithAgreeYOLO(true),
		aicommon.WithAICallback(cb),
	)
	require.NoError(t, err)
	require.NotNil(t, results)

	var ruleAfter *schema.SyntaxFlowRule
	for r := range syntaxflow.QuerySyntaxFlowRulesByKeyword(testRuleName) {
		ruleAfter = r
		break
	}
	require.NotNil(t, ruleAfter)
	require.NotEqual(t, contentBefore, ruleAfter.Content, "数据库中的 rule Content 应已被写回更新")

	content := ruleAfter.Content
	log.Infof(content)

	require.Regexp(t, regexp.MustCompile(`desc\(\s*\n[\s\S]*?title:\s*"[^"]{2,}"`), content, "写回后 desc title 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc\(\s*\n[\s\S]*?title_zh:\s*"[^"]{2,}"`), content, "写回后 desc title_zh 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc:\s*<<<DESC\s*\n[\s\S]+?\nDESC`), content, "写回后 desc 内容应被补全")
	require.Regexp(t, regexp.MustCompile(`solution:\s*<<<SOLUTION\s*\n[\s\S]+?\nSOLUTION`), content, "写回后 solution 应被补全")
	alertBlock := regexp.MustCompile(`alert\s+\$sink\s+for\s+\{([\s\S]*?)\n\}`).FindStringSubmatch(content)
	require.NotEmpty(t, alertBlock, "写回后应能解析出 alert 块")
	alertContent := alertBlock[1]
	require.Regexp(t, regexp.MustCompile(`title:\s*"[^"]{2,}"`), alertContent, "写回后 alert title 应被补全")
	require.Regexp(t, regexp.MustCompile(`title_zh:\s*"[^"]{2,}"`), alertContent, "写回后 alert title_zh 应被补全")
	require.Regexp(t, regexp.MustCompile(`desc:\s*<<<DESC\s*\n[\s\S]+?\nDESC`), alertContent, "写回后 alert desc 应被补全")
}
