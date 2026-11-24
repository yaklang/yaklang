package aicommon

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func newSecurityAskTestCaller(t *testing.T, ctx context.Context, cfg *Config) *ToolCaller {
	t.Helper()
	caller, err := NewToolCaller(ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_RuleGate(func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
			return &ToolCallRuleDecision{
				Action:   schema.SFR_ACTION_ASK,
				Severity: schema.SFR_SEVERITY_WARNING,
				RuleName: "test-security-ask",
				Reason:   "test security review",
			}
		}),
	)
	require.NoError(t, err)
	return caller
}

func TestSecurityAskDowngradesUnderYOLO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var interactiveEvents atomic.Int32
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithAgreePolicy(AgreePolicyYOLO),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				interactiveEvents.Add(1)
			}
		}))

	invoked := false
	tool, err := aitool.New("bash", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		invoked = true
		return "ok", nil
	}))
	require.NoError(t, err)

	caller := newSecurityAskTestCaller(t, ctx, cfg)
	_, _, err = caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"command": "echo hello"})
	require.NoError(t, err)
	require.Equal(t, int32(0), interactiveEvents.Load())
	require.True(t, invoked)
}

func TestSecurityAskManualStillCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var interactiveEvents atomic.Int32
	var cfg *Config
	cfg = NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithAgreePolicy(AgreePolicyManual),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type != schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				return
			}
			interactiveEvents.Add(1)
			id := event.GetInteractiveId()
			if id == "" {
				t.Errorf("interactive event is missing its id")
				return
			}
			go func() {
				time.Sleep(50 * time.Millisecond)
				cfg.Feed(id, aitool.InvokeParams{"suggestion": "取消"})
			}()
		}))

	invoked := false
	tool, err := aitool.New("bash", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		invoked = true
		return "unexpected", nil
	}))
	require.NoError(t, err)

	caller := newSecurityAskTestCaller(t, ctx, cfg)
	_, _, err = caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"command": "echo hello"})
	require.ErrorContains(t, err, "cancelled")
	require.Equal(t, int32(1), interactiveEvents.Load())
	require.False(t, invoked)
}

func TestResolveSecurityReviewAction(t *testing.T) {
	tests := []struct {
		name      string
		policy    AgreePolicyType
		action    schema.SyntaxFlowAction
		effective schema.SyntaxFlowAction
		review    securityReviewMode
	}{
		{name: "deny-manual", policy: AgreePolicyManual, action: schema.SFR_ACTION_DENY, effective: schema.SFR_ACTION_DENY},
		{name: "deny-yolo", policy: AgreePolicyYOLO, action: schema.SFR_ACTION_DENY, effective: schema.SFR_ACTION_DENY},
		{name: "ask-manual", policy: AgreePolicyManual, action: schema.SFR_ACTION_ASK, effective: schema.SFR_ACTION_ASK, review: securityReviewHuman},
		{name: "ask-ai", policy: AgreePolicyAI, action: schema.SFR_ACTION_ASK, effective: schema.SFR_ACTION_ASK, review: securityReviewAI},
		{name: "ask-ai-auto", policy: AgreePolicyAIAuto, action: schema.SFR_ACTION_ASK, effective: schema.SFR_ACTION_ASK, review: securityReviewAI},
		{name: "ask-yolo", policy: AgreePolicyYOLO, action: schema.SFR_ACTION_ASK, effective: schema.SFR_ACTION_WARN},
		{name: "ask-auto", policy: AgreePolicyAuto, action: schema.SFR_ACTION_ASK, effective: schema.SFR_ACTION_WARN},
		{name: "warn-yolo", policy: AgreePolicyYOLO, action: schema.SFR_ACTION_WARN, effective: schema.SFR_ACTION_WARN},
	}
	for _, test := range tests {
		effective, review := resolveSecurityReviewAction(test.policy, test.action)
		require.Equal(t, test.effective, effective, test.name)
		require.Equal(t, test.review, review, test.name)
	}
}
