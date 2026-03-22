package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEndpoint_GetReviewType(t *testing.T) {
	t.Run("stores and returns reviewType from CreateEndpointWithEventType", func(t *testing.T) {
		epm := NewEndpointManager()
		ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		assert.Equal(t, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, ep.GetReviewType())
	})

	t.Run("empty reviewType for CreateEndpoint", func(t *testing.T) {
		epm := NewEndpointManager()
		ep := epm.CreateEndpoint()
		assert.Equal(t, schema.EventType(""), ep.GetReviewType())
	})

	t.Run("tool use review type preserved", func(t *testing.T) {
		epm := NewEndpointManager()
		ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		assert.Equal(t, schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, ep.GetReviewType())
	})

	t.Run("task review type preserved", func(t *testing.T) {
		epm := NewEndpointManager()
		ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
		assert.Equal(t, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, ep.GetReviewType())
	})
}

func TestYOLO_DisableDynamicPlanning_AutoContinues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(true),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		assert.Equal(t, "continue", params["suggestion"])
	case <-time.After(3 * time.Second):
		t.Fatal("should not block when DisableDynamicPlanning is true")
	}
}

func mockAICallbackForReview(actionName, suggestion string) AICallbackType {
	return func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.EmitOutputStream(strings.NewReader(
			`{"@action": "` + actionName + `", "suggestion": "` + suggestion + `", "reason": "mock AI decision"}`,
		))
		rsp.Close()
		return rsp, nil
	}
}

func newDynamicPlanningTestConfig(ctx context.Context, aiCallback AICallbackType, opts ...ConfigOption) *Config {
	c := NewTestConfig(ctx, opts...)
	c.OriginalAICallback = aiCallback
	c.QualityPriorityAICallback = aiCallback
	return c
}

func TestYOLO_DynamicPlanning_DefaultCallbacks_WithAI(t *testing.T) {
	t.Run("plan review calls AI and uses suggestion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cb := mockAICallbackForReview("plan_review", "continue")
		c := newDynamicPlanningTestConfig(ctx, cb,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		ep.SetReviewMaterials(aitool.InvokeParams{
			"plans": map[string]any{"tasks": []string{"task1", "task2"}},
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			require.Equal(t, "continue", params["suggestion"])
		case <-time.After(8 * time.Second):
			t.Fatal("plan review AI call should complete within timeout")
		}
	})

	t.Run("task review calls AI and uses suggestion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cb := mockAICallbackForReview("task_review", "continue")
		c := newDynamicPlanningTestConfig(ctx, cb,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		ep.SetReviewMaterials(aitool.InvokeParams{
			"task":          map[string]any{"name": "test task", "goal": "test goal"},
			"short_summary": "completed successfully",
			"long_summary":  "the task was completed with all expected results",
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			require.Equal(t, "continue", params["suggestion"])
		case <-time.After(8 * time.Second):
			t.Fatal("task review AI call should complete within timeout")
		}
	})

	t.Run("plan review AI suggests incomplete", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cb := mockAICallbackForReview("plan_review", "incomplete")
		c := newDynamicPlanningTestConfig(ctx, cb,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		ep.SetReviewMaterials(aitool.InvokeParams{
			"plans": map[string]any{"tasks": []string{"vague task"}},
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			require.Equal(t, "incomplete", params["suggestion"])
		case <-time.After(8 * time.Second):
			t.Fatal("plan review AI should complete within timeout")
		}
	})

	t.Run("task review AI suggests deeply_think", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cb := mockAICallbackForReview("task_review", "deeply_think")
		c := newDynamicPlanningTestConfig(ctx, cb,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		ep.SetReviewMaterials(aitool.InvokeParams{
			"task":          map[string]any{"name": "shallow task"},
			"short_summary": "surface level analysis",
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			require.Equal(t, "deeply_think", params["suggestion"])
		case <-time.After(8 * time.Second):
			t.Fatal("task review AI should complete within timeout")
		}
	})
}

func TestYOLO_DynamicPlanning_TaskReview_EmitsCompactStructuredObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events := make(chan *schema.AiOutputEvent, 32)
	originalReason := "发现登录入口暴露且原计划假设错误，需要补充验证并调整后续任务执行顺序，避免继续沿用无效路径。"

	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.EmitOutputStream(strings.NewReader(`{"@action":"task_review","suggestion":"adjust_plan","reason":"` + originalReason + `","task_deltas":[{"op":"remove","ref_task_index":"1-4"}]}`))
		rsp.Close()
		return rsp, nil
	}

	c := newDynamicPlanningTestConfig(ctx, cb,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			events <- e
		}),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	ep.SetReviewMaterials(aitool.InvokeParams{
		"task":          map[string]any{"name": "login review", "goal": "review task output"},
		"long_summary":  "found login endpoint and invalid assumptions",
		"pending_tasks": "1-4 SMB enumeration",
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		require.Equal(t, "adjust_plan", params["suggestion"])
		reason := params.GetString("reason")
		require.Contains(t, reason, "需要修改后续任务")
		require.Less(t, len([]rune(reason)), len([]rune(originalReason))+10)
	case <-time.After(8 * time.Second):
		t.Fatal("task review AI call should complete within timeout")
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-events:
			if evt.Type != schema.EVENT_TYPE_STRUCTURED || evt.NodeId != "task-review" {
				continue
			}

			var payload map[string]any
			require.NoError(t, json.Unmarshal(evt.Content, &payload))
			require.Equal(t, "需要修改后续任务", payload["verdict"])
			require.Equal(t, "adjust_plan", payload["suggestion"])
			require.Contains(t, payload["reason"], "需要修改后续任务")

			rawDeltas, ok := payload["task_deltas"].([]any)
			require.True(t, ok)
			require.Len(t, rawDeltas, 1)
			return
		case <-deadline:
			t.Fatal("did not receive compact structured task-review event")
		}
	}
}

func TestYOLO_DynamicPlanning_CustomPlanReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
		WithAiPlanReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
			return aitool.InvokeParams{"suggestion": "incomplete", "extra_prompt": "missing security scan step"}, nil
		}),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		assert.Equal(t, "incomplete", params["suggestion"])
		assert.Equal(t, "missing security scan step", params["extra_prompt"])
	case <-time.After(3 * time.Second):
		t.Fatal("custom plan review should complete quickly")
	}
}

func TestYOLO_DynamicPlanning_CustomTaskReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
		WithAiTaskReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
			return aitool.InvokeParams{"suggestion": "deeply_think"}, nil
		}),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		assert.Equal(t, "deeply_think", params["suggestion"])
	case <-time.After(3 * time.Second):
		t.Fatal("custom task review should complete quickly")
	}
}

func TestYOLO_DynamicPlanning_ToolReview_StillAutoContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	planCallbackCalled := false
	taskCallbackCalled := false

	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
		WithAiPlanReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
			planCallbackCalled = true
			return aitool.InvokeParams{"suggestion": "incomplete"}, nil
		}),
		WithAiTaskReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
			taskCallbackCalled = true
			return aitool.InvokeParams{"suggestion": "deeply_think"}, nil
		}),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		assert.Equal(t, "continue", params["suggestion"],
			"tool review should always auto-continue regardless of dynamic planning")
		assert.False(t, planCallbackCalled, "plan callback should not be called for tool review")
		assert.False(t, taskCallbackCalled, "task callback should not be called for tool review")
	case <-time.After(3 * time.Second):
		t.Fatal("tool review should auto-continue immediately")
	}
}

func TestYOLO_DynamicPlanning_CallbackError_FallbackContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("plan review callback error falls back to continue", func(t *testing.T) {
		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
			WithAiPlanReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
				return nil, errors.New("simulated plan review failure")
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			assert.Equal(t, "continue", params["suggestion"],
				"should fallback to continue when callback fails")
		case <-time.After(3 * time.Second):
			t.Fatal("should not block on callback failure")
		}
	})

	t.Run("task review callback error falls back to continue", func(t *testing.T) {
		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
			WithAiTaskReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
				return nil, errors.New("simulated task review failure")
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			assert.Equal(t, "continue", params["suggestion"],
				"should fallback to continue when callback fails")
		case <-time.After(3 * time.Second):
			t.Fatal("should not block on callback failure")
		}
	})
}

func TestYOLO_DynamicPlanning_EmptyReviewType_AutoContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	callbackCalled := false
	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
		WithAiPlanReviewControl(func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
			callbackCalled = true
			return aitool.InvokeParams{"suggestion": "incomplete"}, nil
		}),
	)
	c.StartEventLoop(ctx)

	ep := c.Epm.CreateEndpoint()
	ep.SetDefaultSuggestionContinue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
	}()

	select {
	case <-done:
		params := ep.GetParams()
		assert.Equal(t, "continue", params["suggestion"],
			"empty review type should auto-continue")
		assert.False(t, callbackCalled, "callback should not be called for empty review type")
	case <-time.After(3 * time.Second):
		t.Fatal("empty review type should auto-continue immediately")
	}
}

func TestYOLO_DynamicPlanning_DefaultCallback_AIFailure_FallbackContinue(t *testing.T) {
	failingAICallback := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		return nil, errors.New("AI service unavailable")
	}

	t.Run("plan review AI failure falls back to continue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		c := newDynamicPlanningTestConfig(ctx, failingAICallback,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			assert.Equal(t, "continue", params["suggestion"],
				"when default AI callback fails, should fallback to auto-continue")
		case <-time.After(8 * time.Second):
			t.Fatal("should not block when AI fails")
		}
	})

	t.Run("task review AI failure falls back to continue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		c := newDynamicPlanningTestConfig(ctx, failingAICallback,
			WithAgreePolicy(AgreePolicyYOLO),
			WithDisableDynamicPlanning(false),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyYOLO, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			assert.Equal(t, "continue", params["suggestion"],
				"when default AI callback fails, should fallback to auto-continue")
		case <-time.After(8 * time.Second):
			t.Fatal("should not block when AI fails")
		}
	})
}
