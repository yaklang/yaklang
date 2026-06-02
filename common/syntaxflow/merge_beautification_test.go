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

