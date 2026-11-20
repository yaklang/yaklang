package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDoWaitAgreeWithPolicy_ForceHumanOverridesYOLO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := NewTestConfig(ctx, WithAgreePolicy(AgreePolicyYOLO))
	endpoint := config.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	endpoint.SetDefaultSuggestionContinue()
	endpoint.SetForceHumanReview("bashguard: dangerous command")
	require.True(t, endpoint.IsForceHumanReview())

	done := make(chan struct{})
	go func() {
		defer close(done)
		config.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, endpoint)
	}()

	select {
	case <-done:
		t.Fatal("force-human review must not be auto-approved by yolo")
	case <-time.After(200 * time.Millisecond):
	}

	endpoint.ActiveWithParams(ctx, aitool.InvokeParams{"suggestion": "continue"})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("force-human review did not unblock after a release")
	}

	meta := endpoint.GetApprovalMeta()
	require.NotNil(t, meta)
	require.Equal(t, ApprovalSourceRule, meta.Source)
	require.True(t, meta.Required)
	require.Equal(t, "bashguard: dangerous command", meta.Reason)
}
