package loopinfra

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
)

func TestToolCompose_ActionMetadata_DescriptionRestrictsUsage(t *testing.T) {
	desc := loopAction_toolCompose.Description

	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE, loopAction_toolCompose.ActionType,
		"action type should be TOOL_COMPOSE")

	assert.Contains(t, desc, "KNOWN",
		"description must emphasize that tools should already be known")
	assert.Contains(t, desc, "explicit dependencies or parallelism",
		"description must require explicit dependency or parallel structure")
	assert.Contains(t, desc, "request_plan_and_execution",
		"description must mention plan as the alternative for uncertain tasks")
	assert.Contains(t, desc, "require_ai_blueprint",
		"description must mention require_ai_blueprint for AI Blueprints")
	assert.Contains(t, desc, "at least 2 tool nodes",
		"description must state the minimum node requirement")

	assert.NotContains(t, strings.ToLower(desc), "complex multi-step operations",
		"description should NOT use overly broad language that overlaps with PLAN scope")
}

func TestToolCompose_OutputExamples_ContainsGateGuidance(t *testing.T) {
	examples := loopAction_toolCompose.OutputExamples

	assert.Contains(t, examples, "使用前提",
		"output examples must include usage prerequisites section")
	assert.Contains(t, examples, "至少 2 个",
		"output examples must state minimum 2 tools requirement")
	assert.Contains(t, examples, "request_plan_and_execution",
		"output examples must mention plan as alternative")
	assert.Contains(t, examples, "require_ai_blueprint",
		"output examples must mention require_ai_blueprint for blueprints")
	assert.Contains(t, examples, "require_tool",
		"output examples must mention require_tool for single-tool scenarios")
}

func TestToolCompose_StrategyGateConditions_SingleNode(t *testing.T) {
	payload := `[{"call_id":"only_step","tool_name":"search","call_intent":"search something"}]`

	var nodes []workflowdag.ToolCallNode
	err := json.Unmarshal([]byte(payload), &nodes)
	assert.NoError(t, err)
	assert.Len(t, nodes, 1, "single-node DAG should parse to 1 node")

	isSingleNode := len(nodes) == 1
	assert.True(t, isSingleNode,
		"strategy gate should detect single-node pattern (should recommend require_tool)")
}

func TestToolCompose_StrategyGateConditions_MultiNodeNoDeps(t *testing.T) {
	payload := `[
		{"call_id":"step1","tool_name":"tool_a","call_intent":"do A"},
		{"call_id":"step2","tool_name":"tool_b","call_intent":"do B"},
		{"call_id":"step3","tool_name":"tool_c","call_intent":"do C"}
	]`

	var nodes []workflowdag.ToolCallNode
	err := json.Unmarshal([]byte(payload), &nodes)
	assert.NoError(t, err)
	assert.Len(t, nodes, 3)

	hasDep := false
	for _, n := range nodes {
		if len(n.RawDependsOn) > 0 {
			hasDep = true
			break
		}
	}
	assert.False(t, hasDep,
		"strategy gate should detect no-dependency pattern — "+
			"nodes without depends_on lack clear DAG structure")
}

func TestToolCompose_StrategyGateConditions_ProperDAG(t *testing.T) {
	payload := `[
		{"call_id":"fetch","tool_name":"http_get","call_intent":"fetch data"},
		{"call_id":"parse","tool_name":"json_parse","call_intent":"parse response","depends_on":["fetch"]},
		{"call_id":"store","tool_name":"db_save","call_intent":"save result","depends_on":["parse"]}
	]`

	var nodes []workflowdag.ToolCallNode
	err := json.Unmarshal([]byte(payload), &nodes)
	assert.NoError(t, err)
	assert.Len(t, nodes, 3)

	hasDep := false
	for _, n := range nodes {
		if len(n.RawDependsOn) > 0 {
			hasDep = true
			break
		}
	}
	assert.True(t, hasDep,
		"proper DAG with dependencies should pass strategy gate without warnings")
	assert.True(t, len(nodes) >= 2,
		"proper DAG should have at least 2 nodes")
}

func TestToolCompose_StrategyGateConditions_DiamondDAG(t *testing.T) {
	payload := `[
		{"call_id":"init","tool_name":"setup"},
		{"call_id":"branch_a","tool_name":"fetch_a","depends_on":["init"]},
		{"call_id":"branch_b","tool_name":"fetch_b","depends_on":["init"]},
		{"call_id":"merge","tool_name":"combine","depends_on":["branch_a","branch_b"]}
	]`

	var nodes []workflowdag.ToolCallNode
	err := json.Unmarshal([]byte(payload), &nodes)
	assert.NoError(t, err)
	assert.Len(t, nodes, 4)

	depCount := 0
	for _, n := range nodes {
		if len(n.RawDependsOn) > 0 {
			depCount++
		}
	}
	assert.Equal(t, 3, depCount,
		"diamond DAG should have 3 nodes with dependencies — this is a valid tool_compose pattern")
}
