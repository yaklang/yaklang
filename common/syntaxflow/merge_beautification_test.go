package syntaxflow

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const sampleRuleWithRuleID = `desc(
	title: "Old Title"
	title_zh: "旧标题"
	desc: "short"
	solution: "none"
	reference: "none"
	rule_id: "existing-rule-id-1234"
)

alert $sink for {
	name: "sink"
	title: "Sink"
}
`

const sampleRuleWithoutRuleID = `desc(
	title: "Old Title"
	title_zh: "旧标题"
)

alert $sink for {
	name: "sink"
}
`

func TestMergeBeautificationResults_PreservesExistingRuleID(t *testing.T) {
	descParams := aitool.InvokeParams{
		"title":    "New Title",
		"title_zh": "新标题",
		"desc":     "补全后的描述内容，长度足够用于测试合并逻辑是否正常工作。",
		"solution": "none",
	}
	alertParams := aitool.InvokeParams{
		"alert": []any{
			map[string]any{
				"name":     "sink",
				"title":    "Sink Alert",
				"title_zh": "告警",
			},
		},
	}

	merged, err := MergeBeautificationResults(descParams, alertParams, sampleRuleWithRuleID)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !strings.Contains(merged, `rule_id: "existing-rule-id-1234"`) {
		t.Fatalf("expected existing rule_id preserved, got:\n%s", merged)
	}
	if !strings.Contains(merged, `title: "New Title"`) {
		t.Fatalf("expected title updated, got:\n%s", merged)
	}
}

func TestMergeBeautificationResults_GeneratesRuleIDWhenMissing(t *testing.T) {
	descParams := aitool.InvokeParams{
		"title":    "New Title",
		"title_zh": "新标题",
		"desc":     "补全后的描述内容，长度足够用于测试合并逻辑是否正常工作。",
		"solution": "none",
	}
	alertParams := aitool.InvokeParams{
		"alert": []any{
			map[string]any{
				"name":     "sink",
				"title":    "Sink Alert",
				"title_zh": "告警",
			},
		},
	}

	merged, err := MergeBeautificationResults(descParams, alertParams, sampleRuleWithoutRuleID)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !strings.Contains(merged, `rule_id: "`) {
		t.Fatalf("expected generated rule_id, got:\n%s", merged)
	}
	if strings.Contains(merged, `rule_id: ""`) {
		t.Fatalf("expected non-empty rule_id, got:\n%s", merged)
	}
}

func TestMergeBeautificationResults_PreservesLevel(t *testing.T) {
	const rule = `desc(
	title: "T"
	level: high
)

alert $sink for {
	name: "sink"
	level: "critical"
	risk: "旧风险"
}
`
	descParams := aitool.InvokeParams{
		"title":    "New",
		"title_zh": "新",
		"desc":     "补全后的描述内容，长度足够用于测试合并逻辑是否正常工作。",
		"level":    "low",
		"solution": "none",
	}
	alertParams := aitool.InvokeParams{
		"alert": []any{
			map[string]any{
				"name":     "sink",
				"title_zh": "告警",
				"level":    "info",
				"risk":     "SQL注入",
			},
		},
	}
	merged, err := MergeBeautificationResults(descParams, alertParams, rule)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !strings.Contains(merged, `level: high`) {
		t.Fatalf("expected main desc level preserved, got:\n%s", merged)
	}
	if strings.Contains(merged, `level: low`) {
		t.Fatalf("expected main desc level not overwritten, got:\n%s", merged)
	}
	if !strings.Contains(merged, `level: "critical"`) && !strings.Contains(merged, `level: critical`) {
		t.Fatalf("expected alert level preserved, got:\n%s", merged)
	}
}

func TestMergeBeautificationResults_RuleIDFromDescParams(t *testing.T) {
	const rule = sampleRuleWithoutRuleID
	descParams := aitool.InvokeParams{
		"title":    "T",
		"title_zh": "标题",
		"desc":     "补全后的描述内容，长度足够用于测试合并逻辑是否正常工作。",
		"solution": "none",
		"rule_id":  "tool-generated-uuid-0001",
	}
	merged, err := MergeBeautificationResults(descParams, nil, rule)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !strings.Contains(merged, `rule_id: "tool-generated-uuid-0001"`) {
		t.Fatalf("expected rule_id from descParams, got:\n%s", merged)
	}
}

func TestMergeBeautificationResultsForYak(t *testing.T) {
	merged, err := MergeBeautificationResultsForYak(map[string]any{
		"title": "Beautified", "title_zh": "美化标题",
		"desc": "美化后的描述内容，用于验证 Yak 导出合并路径是否正常工作。",
		"solution": "none",
	}, map[string]any{
		"alert": []any{map[string]any{"name": "sink", "title_zh": "告警"}},
	}, sampleRuleWithoutRuleID)
	if err != nil {
		t.Fatalf("merge via yak export failed: %v", err)
	}
	if !strings.Contains(merged, `title: "Beautified"`) {
		t.Fatalf("expected beautified title, got:\n%s", merged)
	}
}

// 规则体含语法错误时，合并仍应更新 desc/alert 文本，不因 CompileRule/FormatRule 校验失败。
func TestMergeBeautificationResults_IgnoresRuleBodySyntaxErrors(t *testing.T) {
	ruleWithBrokenBody := `desc(
	title: "Old Title"
	title_zh: "旧标题"
)

alert $sink for {
	name: "sink"
	title: "Sink"
}

((( invalid syntaxflow body
`
	descParams := aitool.InvokeParams{
		"title":    "New Title",
		"title_zh": "新标题",
		"desc":     "补全后的描述内容，长度足够用于测试合并逻辑是否正常工作。",
		"solution": "none",
	}
	alertParams := aitool.InvokeParams{
		"alert": []any{
			map[string]any{
				"name":     "sink",
				"title":    "Sink Alert",
				"title_zh": "告警",
			},
		},
	}

	merged, err := MergeBeautificationResults(descParams, alertParams, ruleWithBrokenBody)
	if err != nil {
		t.Fatalf("merge should not fail on rule body syntax errors: %v", err)
	}
	if !strings.Contains(merged, `title: "New Title"`) {
		t.Fatalf("expected title updated despite broken body, got:\n%s", merged)
	}
}


// AI 值中的换行必须安全落盘：简单 key 双引号内做转义，复杂 key heredoc 前归一化字面 \n。
func TestMergeBeautificationResults_EscapesNewlineAndQuotes(t *testing.T) {
	const rule = `desc(
	title: "Old Title"
	rule_id: "escape-test-0001"
)

alert $sink for {
	name: "sink"
	title: "Sink"
}
`
	descParams := aitool.InvokeParams{
		"title":    "标题A\n标题B",     // 简单key：真实换行，双引号内应转义为 \n
		"solution": "步骤1\n步骤2",    // 复杂key：字面 \n，heredoc 前应归一化为真实换行
	}
	alertParams := aitool.InvokeParams{
		"alert": []any{
			map[string]any{
				"name":    "sink",
				"message": "消息A\n含\"引号\"", // 简单key：字面 \n 归一化后转义，引号转义
			},
		},
	}

	merged, err := MergeBeautificationResults(descParams, alertParams, rule)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// 简单key：换行以 \n 转义序列落盘，不产生裸换行破坏字面量
	if !strings.Contains(merged, `title: "标题A\n标题B"`) {
		t.Fatalf("expected title newline escaped in quoted literal, got:\n%q", merged)
	}
	// 复杂key：heredoc 内是真实换行，不再残留字面 \n
	if !strings.Contains(merged, "步骤1\n步骤2") || strings.Contains(merged, `步骤1\n步骤2`) {
		t.Fatalf("expected solution literal \n normalized to real newline in heredoc, got:\n%q", merged)
	}
	// alert 简单key：字面 \n 归一化后转义 + 引号转义
	if !strings.Contains(merged, `message: "消息A\n含\"引号\""`) {
		t.Fatalf("expected alert message escaped, got:\n%q", merged)
	}
}
