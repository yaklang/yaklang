package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewTestConfig(ctx context.Context) *Config {
	return NewConfig(ctx,
		WithAICallback(func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}),
	)
}

func TestProcessInputEvent_Interactive_WithSuggestion(t *testing.T) {
	// Setup config with epm stub
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := NewTestConfig(ctx)
	c.StartEventLoop(ctx)

	epm := c.Epm

	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	reqs := map[string]any{
		"id": ep.GetId(),
	}
	ep.SetReviewMaterials(reqs)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        ep.GetId(),
		InteractiveJSONInput: string(`{"suggestion":"doit","other":{"a":"b"}}`),
	})

	c.DoWaitAgree(ctx, ep)
	params := ep.GetParams()

	require.Equal(t, "doit", params["suggestion"])
}

func TestProcessInputEvent_Interactive_DefaultContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := NewTestConfig(ctx)
	c.StartEventLoop(ctx)

	epm := c.Epm

	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	reqs := map[string]any{
		"id": ep.GetId(),
	}
	ep.SetReviewMaterials(reqs)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        ep.GetId(),
		InteractiveJSONInput: string(`{"other":{"a":"b"}}`),
	})

	c.DoWaitAgree(ctx, ep)
	params := ep.GetParams()

	require.Equal(t, "continue", params["suggestion"])
}

func TestProcessInputEvent_MirrorAndFreeInputCallback(t *testing.T) {
	var c Config
	processor := NewAIInputEventProcessor()
	c.InputEventManager = processor

	// mirror should be called
	var mirrorCalled bool
	processor.RegisterMirrorOfAIInputEvent("m1", func(e *ypb.AIInputEvent) {
		mirrorCalled = true
	})

	// free input callback should be called
	var freeCalled bool
	processor.SetFreeInputCallback(func(e *ypb.AIInputEvent) error {
		freeCalled = true
		return nil
	})

	event := &ypb.AIInputEvent{
		IsFreeInput: true,
	}

	if err := c.processInputEvent(event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mirrorCalled {
		t.Fatalf("expected mirror to be called")
	}
	if !freeCalled {
		t.Fatalf("expected free input callback to be called")
	}
}

func TestProcessInputEvent_SyncCallback(t *testing.T) {
	var c Config
	processor := NewAIInputEventProcessor()
	c.InputEventManager = processor

	var syncCalled bool
	processor.RegisterSyncCallback("sync-1", func(e *ypb.AIInputEvent) error {
		syncCalled = true
		return nil
	})

	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      "sync-1",
	}

	if err := c.processInputEvent(event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !syncCalled {
		t.Fatalf("expected sync callback to be called")
	}
}
