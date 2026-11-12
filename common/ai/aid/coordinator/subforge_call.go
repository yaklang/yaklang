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
// eventMu protects lifecycle state and the task-tree projection. deliveryMu
// serializes child callbacks; never hold eventMu while invoking a subscriber.
type subForgeCall struct {
	session     *Session
	task        aicommon.AIStatefulTask
	ctx         context.Context
	input       *chanx.UnlimitedChan[*ypb.AIInputEvent]
	endpoints   sync.Map
	unregister  func(...string)
	releaseSlot func()

	eventMu      sync.Mutex
	deliveryMu   sync.Mutex
	tree         subForgeTaskTree
	pending      map[string]struct{}
	closed       bool
	childStarted bool
}

func newSubForgeCall(ctx context.Context, s *Session, task aicommon.AIStatefulTask, callID string, binding SubForgeTool, releaseSlot func()) *subForgeCall {
	c := &subForgeCall{
		session: s, task: task, ctx: ctx, releaseSlot: releaseSlot,
		input:   chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		pending: make(map[string]struct{}),
		tree: subForgeTaskTree{
			ParentCoordinatorID: s.GetRuntimeId(),
			ParentTaskID:        task.GetId(),
			ParentTaskUUID:      task.GetUUID(),
			CallToolID:          callID,
			ChildCoordinatorID:  uuid.NewString(),
			Revision:            1,
			Status:              "running",
		},
	}
	if binding.Schema != nil {
		c.tree.ToolName = binding.Schema.Name
	}
	c.unregister = s.InputEventManager.RouteInteractiveInput(c.tree.ChildCoordinatorID, c.routeInput)
	return c
}

func (c *subForgeCall) routeInput(event *ypb.AIInputEvent) bool {
	endpoint := event.GetInteractiveId()
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
	forwarded.CoordinatorId = c.tree.ChildCoordinatorID
	forwarded.TaskId = c.task.GetId()
	forwarded.TaskUUID = c.task.GetUUID()
	forwarded.CallToolID = c.tree.CallToolID
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
	c.eventMu.Unlock()
	var retiredIDs []string
	c.endpoints.Range(func(key, _ any) bool {
		retiredIDs = append(retiredIDs, key.(string))
		return true
	})
	c.unregister(retiredIDs...)
	if len(c.pending) > 0 {
		// Keep legacy review release delivery serialized with child events.
		go func() {
			c.deliveryMu.Lock()
			defer c.deliveryMu.Unlock()
			defer func() {
				if failure := recover(); failure != nil {
					log.Errorf("sub-Forge review release handler panicked: %v", failure)
				}
			}()
			for endpoint := range c.pending {
				content, _ := json.Marshal(map[string]any{"id": endpoint, "params": nil})
				c.forward(&schema.AiOutputEvent{
					Type: schema.EVENT_TYPE_REVIEW_RELEASE, NodeId: "review-release", IsJson: true, Content: content,
				})
			}
		}()
	}
	// Persist the terminal snapshot even if an earlier progress callback
	// is blocked. Its revision orders it against any in-flight progress.
	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		c.session.emitSubForgeTaskTree(terminal)
	}()
	select {
	case <-delivered:
	case <-c.ctx.Done():
	}
}

func (c *subForgeCall) handleEvent(event *schema.AiOutputEvent) {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	c.eventMu.Lock()
	if c.closed {
		c.eventMu.Unlock()
		return
	}
	endpoint := event.GetInteractiveId()
	if event.Type == schema.EVENT_TYPE_REVIEW_RELEASE {
		if _, exists := c.pending[endpoint]; !exists {
			c.eventMu.Unlock()
			return
		}
		delete(c.pending, endpoint)
		c.endpoints.Store(endpoint, false)
	} else {
		if c.ctx.Err() != nil {
			c.eventMu.Unlock()
			return
		}
		if endpoint != "" {
			if _, known := c.endpoints.LoadOrStore(endpoint, true); !known {
				c.pending[endpoint] = struct{}{}
			}
		}
	}
	changed := c.tree.observePlan(event)
	update := c.tree
	c.eventMu.Unlock()
	if changed {
		c.session.emitSubForgeTaskTree(update)
	}
	c.forward(event)
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
