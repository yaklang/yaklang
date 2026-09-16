package syntaxflowruletests

// SyntaxFlow rule-gen protocol tests (FreeInput + FocusModeLoop + AttachedResourceInfo + syntaxflow_rule_change).
//
// Wire protocol (backend → frontend「规则编写」):
//   - Type/NodeId: syntaxflow_rule_change
//   - op=create|replace: code.content is the full .sf rule text (shape mirrors yaklang_code_change).
//
// Run:
//   go test -v -run TestSyntaxFlowRuleProtocol_ ./common/ai/aid/aireact/reactloops/loop_syntaxflow_rule/tests/...

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type syntaxflowRuleChangeResponse struct {
	Op           string `json:"op"`
	SourceAction string `json:"source_action"`
	Reason       string `json:"reason,omitempty"`
	Code         struct {
		Content string `json:"content"`
		Path    string `json:"path,omitempty"`
		Summary string `json:"summary,omitempty"`
		Version int    `json:"version"`
	} `json:"code"`
}

type syntaxFlowProtocolResult struct {
	timeline         string
	taskFailed       bool
	ruleChangeEvents []*ypb.AIOutputEvent
}

func runSyntaxFlowProtocolScenario(
	t *testing.T,
	userQuery string,
	attached []*ypb.AttachedResourceInfo,
) syntaxFlowProtocolResult {
	t.Helper()

	in := make(chan *ypb.AIInputEvent, 4)
	out := make(chan *ypb.AIOutputEvent, 128)

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedSyntaxFlowWriting(t, i, r)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:          true,
			FreeInput:            userQuery,
			FocusModeLoop:        schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW,
			AttachedResourceInfo: attached,
		}
	}()

	timeout := 8 * time.Second
	if utils.InGithubActions() {
		timeout = 12 * time.Second
	}
	deadline := time.After(timeout)

	var result syntaxFlowProtocolResult

taskLoop:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE) {
				result.ruleChangeEvents = append(result.ruleChangeEvents, e)
			}
			if e.GetNodeId() == "react_task_status_changed" {
				content := string(e.GetContent())
				if strings.Contains(content, "Aborted") || strings.Contains(content, "Failed") {
					result.taskFailed = true
					break taskLoop
				}
				if strings.Contains(content, `"react_task_now_status":"completed"`) ||
					strings.Contains(content, `"react_task_now_status": "completed"`) {
					break taskLoop
				}
			}
			if len(result.ruleChangeEvents) > 0 {
				break taskLoop
			}
		case <-deadline:
			break taskLoop
		}
	}
	close(in)
	ins.Wait()

	result.timeline = ins.DumpTimeline()
	return result
}

func parseSyntaxFlowRuleChangeResponse(t *testing.T, e *ypb.AIOutputEvent) syntaxflowRuleChangeResponse {
	t.Helper()
	require.Equal(t, string(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE), e.Type)
	require.Equal(t, "syntaxflow_rule_change", e.NodeId)
	require.True(t, e.IsJson)

	var payload syntaxflowRuleChangeResponse
	require.NoError(t, json.Unmarshal(e.Content, &payload))
	return payload
}

func TestSyntaxFlowRuleProtocol_EmitsSyntaxFlowRuleChange(t *testing.T) {
	result := runSyntaxFlowProtocolScenario(t, "write a SyntaxFlow rule for testing", nil)
	t.Log("timeline:\n", result.timeline)

	require.False(t, result.taskFailed, "task should not fail")
	require.NotEmpty(t, result.ruleChangeEvents, "should emit syntaxflow_rule_change for SyntaxFlow rule delivery")

	payload := parseSyntaxFlowRuleChangeResponse(t, result.ruleChangeEvents[0])
	assert.Equal(t, "create", payload.Op)
	assert.Contains(t, payload.Code.Content, `rule("test-rule")`)
	assert.Contains(t, payload.Code.Content, "desc(")
	assert.Greater(t, payload.Code.Version, 0)
	assert.True(t, strings.HasSuffix(strings.ToLower(payload.Code.Path), ".sf") || payload.Code.Path == "",
		"path should be .sf or empty (deferred delivery)")
}

func TestSyntaxFlowRuleProtocol_AttachedRuleDraftInTimeline(t *testing.T) {
	draft := `rule("existing-draft")
desc(
	title: "Existing Draft"
	type: audit
)`
	attached := []*ypb.AttachedResourceInfo{
		{
			Type:  aicommon.AttachedResourceTypeSelected,
			Key:   aicommon.AttachedResourceKeySyntaxFlowRule,
			Value: draft,
		},
	}

	result := runSyntaxFlowProtocolScenario(t, "improve this SyntaxFlow rule", attached)
	t.Log("timeline:\n", result.timeline)

	require.Contains(t, result.timeline, "existing-draft",
		"timeline should include attached syntaxflow_rule draft")
	require.Contains(t, result.timeline, "规则编写草稿",
		"timeline should mark the rule draft attachment")

	// Attached draft already fills the editor buffer; mock still tries write_rule which may fail.
	// Timeline draft presence is the primary assertion for this scenario.
}