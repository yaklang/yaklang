package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// subForgeCall owns one admitted child's execution and interactive event scope.
// eventMu protects lifecycle state. deliveryMu
// serializes child callbacks; never hold eventMu while invoking a subscriber.
type subForgeCall struct {
	session     *Session
	task        aicommon.AIStatefulTask
	ctx         context.Context
	input       *chanx.UnlimitedChan[*ypb.AIInputEvent]
	endpoints   sync.Map
	planReviews sync.Map
	unregister  func(...string)
	releaseSlot func()

	eventMu      sync.Mutex
	deliveryMu   sync.Mutex
	childID      string
	tree         subForgeTaskTree
	callID       string
	pending      map[string]struct{}
	queued       []*schema.AiOutputEvent
	active       string
	closed       bool
	childStarted bool
}

func newSubForgeCall(ctx context.Context, s *Session, task aicommon.AIStatefulTask, callID string, releaseSlot func()) *subForgeCall {
	c := &subForgeCall{
		session: s, task: task, ctx: ctx, releaseSlot: releaseSlot,
		input:   chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		pending: make(map[string]struct{}),
		childID: uuid.NewString(),
		callID:  callID,
	}
	c.tree = subForgeTaskTree{ParentCoordinatorID: s.GetRuntimeId(), ParentTaskID: task.GetId(), ParentTaskUUID: task.GetUUID(), CallToolID: callID, ChildCoordinatorID: c.childID, Revision: 1, Status: "running"}
	c.unregister = s.InputEventManager.RouteInteractiveInput(c.childID, c.routeInput)
	return c
}

func (c *subForgeCall) routeInput(event *ypb.AIInputEvent) bool {
	endpoint := event.GetInteractiveId()
	if review, ok := c.planReviews.Load(endpoint); ok {
		var valid bool
		event, valid = subForgePlanReviewInput(event, review.(*schema.AiOutputEvent))
		if !valid {
			return true
		} // Keep the visible endpoint open for a corrected reply.
	}
	if c.endpoints.CompareAndSwap(endpoint, true, false) {
		if c.ctx.Err() == nil {
			c.input.SafeFeed(event)
		}
		return true
	}
	_, known := c.endpoints.Load(endpoint)
	return known
}

func (c *subForgeCall) forward(event *schema.AiOutputEvent) {
	forwarded := *event
	forwarded.CoordinatorId = c.childID
	forwarded.TaskId = c.task.GetId()
	forwarded.TaskUUID = c.task.GetUUID()
	forwarded.CallToolID = c.callID
	if c.session.EventHandler != nil {
		c.session.EventHandler(&forwarded)
	}
}

func (c *subForgeCall) interrupted(err error) error {
	c.eventMu.Lock()
	started := c.childStarted
	c.eventMu.Unlock()
	if !started {
		// The child checks this same context under c.eventMu before starting.
		c.releaseSlot()
		return errors.Join(err, aitool.ErrInvocationNotStarted)
	}
	return err
}

func (c *subForgeCall) finish(err error) {
	c.eventMu.Lock()
	c.closed = true
	c.tree.Revision++
	c.tree.Status = subForgeTreeStatus(err)
	terminal := c.tree
	active := c.active
	c.eventMu.Unlock()
	var retiredIDs []string
	c.endpoints.Range(func(key, _ any) bool {
		retiredIDs = append(retiredIDs, key.(string))
		return true
	})
	c.unregister(retiredIDs...)
	if active != "" {
		// Keep legacy review release delivery serialized with child events.
		go func() {
			c.deliveryMu.Lock()
			defer c.deliveryMu.Unlock()
			defer func() {
				if failure := recover(); failure != nil {
					log.Errorf("sub-Forge review release handler panicked: %v", failure)
				}
			}()
			content, _ := json.Marshal(map[string]any{"id": active, "params": nil})
			c.forward(&schema.AiOutputEvent{
				Type: schema.EVENT_TYPE_REVIEW_RELEASE, NodeId: "review-release", IsJson: true, Content: content,
			})
		}()
	}
	delivered := make(chan struct{})
	go func() { defer close(delivered); c.session.emitSubForgeTaskTree(terminal) }()
	select {
	case <-delivered:
	case <-c.ctx.Done():
	}

}

func (c *subForgeCall) handleEvent(event *schema.AiOutputEvent) {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	c.eventMu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.eventMu.Unlock()
		return
	}
	changed := c.tree.observePlan(event)
	snapshot := c.tree
	c.eventMu.Unlock()
	if changed {
		c.session.emitSubForgeTaskTree(snapshot)
	}
	original := event
	// Only the explicit snapshot can update the client's child tree. Keep raw
	// child lifecycle and tool results isolated from the parent execution view.
	event = subForgeInteractiveEvent(event)
	if event == nil {
		return
	}
	c.eventMu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.eventMu.Unlock()
		return
	}
	endpoint := event.GetInteractiveId()
	if endpoint == "" {
		c.eventMu.Unlock()
		return
	}
	var next *schema.AiOutputEvent
	if event.Type == schema.EVENT_TYPE_REVIEW_RELEASE {
		if _, exists := c.pending[endpoint]; !exists {
			c.eventMu.Unlock()
			return
		}
		delete(c.pending, endpoint)
		c.endpoints.Store(endpoint, false)
		if c.active != endpoint {
			c.eventMu.Unlock()
			return
		}
		c.active = ""
		for len(c.queued) > 0 {
			candidate := c.queued[0]
			c.queued = c.queued[1:]
			if _, exists := c.pending[candidate.GetInteractiveId()]; exists {
				next = candidate
				c.active = candidate.GetInteractiveId()
				break
			}
		}
	} else {
		if _, known := c.endpoints.LoadOrStore(endpoint, true); known {
			c.eventMu.Unlock()
			return
		}
		if original.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			c.planReviews.Store(endpoint, original)
		}
		c.pending[endpoint] = struct{}{}
		if c.active != "" {
			c.queued = append(c.queued, event)
			c.eventMu.Unlock()
			return
		}
		c.active = endpoint
	}
	c.eventMu.Unlock()
	c.forward(event)
	if next != nil {
		c.forward(next)
	}
}

func (c *subForgeCall) run(ctx context.Context, binding SubForgeTool, params aitool.InvokeParams, allowed []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
	type childResult struct {
		value any
		err   error
	}
	done := make(chan childResult, 1)
	initial := c.tree
	go func() {
		var result childResult
		defer func() {
			if failure := recover(); failure != nil {
				result.err = fmt.Errorf("sub-Forge execution panicked: %v", failure)
			}
			c.releaseSlot()
			done <- result
		}()
		c.session.emitSubForgeTaskTree(initial)
		c.eventMu.Lock()
		result.err = ctx.Err()
		c.childStarted = result.err == nil
		c.eventMu.Unlock()
		if result.err != nil {
			return
		}
		result.value, result.err = binding.Run(ctx, params, allowed, opts...)
	}()
	select {
	case <-ctx.Done():
		return nil, c.interrupted(ctx.Err())
	case result := <-done:
		if err := ctx.Err(); err != nil {
			return nil, c.interrupted(err)
		}
		return result.value, result.err
	}
}
