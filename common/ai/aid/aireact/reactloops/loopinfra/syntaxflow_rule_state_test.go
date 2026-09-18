package loopinfra

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestApplyLoopYaklangCodeChange_SyntaxFlowContentType_EmitsSyntaxFlowRuleChange(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := NewSingleFileModificationSuiteFactory(runtime,
		WithLoopVarsPrefix("sf"),
		WithActionSuffix("rule"),
		WithFileExtension(".sf"),
		WithAITagConfig("GEN_RULE", "sf_rule", "syntaxflow-rule", "text/syntaxflow"),
		WithEditorChange(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE),
	)
	loop, capture, _ := newLoopWithCapturedEvents(t, runtime, factory)

	rule := `rule("test")
desc(title: "t")`
	result, err := factory.applyLoopCodeChange(loop, &loopCodeChange{
		Content:      rule,
		Path:         "/tmp/demo.sf",
		SourceAction: "write_rule",
		EmitEvent:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, rule, loop.Get("full_sf_code"))
	assert.Empty(t, capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE), "SF must not emit yaklang_code_change")
	events := capture.byType(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE)
	require.Len(t, events, 1)
	require.Equal(t, "syntaxflow_rule_change", events[0].NodeId)
	require.Equal(t, schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE, factory.editorChange)

	var payload CodeChangeEvent
	require.NoError(t, json.Unmarshal(events[0].Content, &payload))
	assert.Equal(t, "create", payload.Op)
	assert.Equal(t, rule, payload.Code.Content)
	assert.Equal(t, "write_rule:1", payload.Code.ChangeID)
}
