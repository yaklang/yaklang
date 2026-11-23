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

func TestSecurityAskIgnoresYOLO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var interactiveEvents atomic.Int32
	var cfg *Config
	cfg = NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithAgreePolicy(AgreePolicyYOLO),
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

	_, _, err = caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"command": "echo hello"})
	require.ErrorContains(t, err, "cancelled")
	require.Equal(t, int32(1), interactiveEvents.Load())
	require.False(t, invoked)
}
