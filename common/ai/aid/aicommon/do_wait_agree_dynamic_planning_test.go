package aicommon

import (
	"context"
	"errors"
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

func TestYOLO_DynamicPlanning_DefaultCallbacks_AutoContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyYOLO),
		WithDisableDynamicPlanning(false),
	)
	c.StartEventLoop(ctx)

	t.Run("plan review with default callback returns continue", func(t *testing.T) {
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
			require.Equal(t, "continue", params["suggestion"])
		case <-time.After(3 * time.Second):
			t.Fatal("default plan review should auto-continue")
		}
	})

	t.Run("task review with default callback returns continue", func(t *testing.T) {
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
			require.Equal(t, "continue", params["suggestion"])
		case <-time.After(3 * time.Second):
			t.Fatal("default task review should auto-continue")
		}
	})
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
