package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const subForgeTimeout = 10 * time.Minute

type subForgeContextKey struct{}

func IsSubForgeContext(ctx context.Context) bool {
	return ctx != nil && ctx.Value(subForgeContextKey{}) == true
}

// SubForgeTool binds an already-resolved blueprint to the coordinator without
// importing the Forge builder or adding another global action or registry.
type SubForgeTool struct {
	Schema *mcp.Tool
	Tools  []*aitool.Tool
	Run    func(context.Context, aitool.InvokeParams, []*aitool.Tool, ...aicommon.ConfigOption) (any, error)
}

type subForgeToolsOption []SubForgeTool

func WithSubForgeTools(tools []SubForgeTool) aicommon.ConfigOption {
	return aicommon.WithAppendOtherOption(subForgeToolsOption(tools))
}

func (s *Session) subForgeWorkerManager(task aicommon.AIStatefulTask) (*buildinaitools.AiToolManager, []string, error) {
	manager := s.GetAiToolManager().ForkForSession()
	if IsSubForgeContext(s.GetContext()) {
		return manager, nil, nil
	}
	var descriptions []string
	for _, binding := range s.subForgeTools {
		tool, err := aitool.NewFromMCPTool(binding.Schema, aitool.WithCallback(
			func(ctx context.Context, params aitool.InvokeParams, invocation *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
				callID := ""
				if invocation != nil {
					callID = invocation.RuntimeID
				}
				arguments := maps.Clone(params)
				delete(arguments, "runtime_id") // ToolCaller supplies transport metadata.
				return s.invokeSubForge(ctx, task, callID, binding, arguments)
			},
		))
		if err != nil {
			return nil, nil, err
		}
		tool.RequireForeground = true
		tool.RejectInterruptedReplay = true
		admitted, err := manager.AppendScopedTool(tool)
		if err != nil {
			return nil, nil, err
		}
		if admitted {
			schema, err := json.Marshal(tool.InputSchema.ToMap())
			if err != nil {
				return nil, nil, err
			}
			descriptions = append(descriptions, fmt.Sprintf("- %s: %s. Arguments: %s", tool.Name, tool.Description, schema))
		}
	}
	return manager, descriptions, nil
}

func (s *Session) invokeSubForge(ctx context.Context, task aicommon.AIStatefulTask, callID string, binding SubForgeTool, params aitool.InvokeParams) (_ any, retErr error) {
	if IsSubForgeContext(ctx) || IsSubForgeContext(s.GetContext()) {
		return nil, fmt.Errorf("sub-Forge nesting is limited to one level")
	}
	if s.IsToolUseDisabled() {
		return nil, fmt.Errorf("tool execution is disabled")
	}
	if !s.subForgeActive.CompareAndSwap(false, true) {
		return nil, aicommon.ErrSubForgeBusy
	}
	// Release admission immediately if setup fails. Once started, only the
	// child goroutine can release it, even if its caller has timed out.
	dispatched := false
	releaseSlot := sync.OnceFunc(func() { s.subForgeActive.Store(false) })
	defer func() {
		if !dispatched {
			releaseSlot()
		}
	}()

	ctx, cancel := context.WithTimeout(context.WithValue(ctx, subForgeContextKey{}, true), subForgeTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, aitool.ErrInvocationNotStarted)
	}
	allowed, err := s.subForgeAllowedTools(binding.Tools)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	tree := subForgeTaskTree{
		ParentCoordinatorID: s.GetRuntimeId(),
		ParentTaskID:        task.GetId(),
		ParentTaskUUID:      task.GetUUID(),
		CallToolID:          callID,
		ChildCoordinatorID:  id,
		Revision:            1,
		Status:              "running",
	}
	if binding.Schema != nil {
		tree.ToolName = binding.Schema.Name
	}
	input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	var endpoints sync.Map
	unregister := s.InputEventManager.RouteInteractiveInput(id, func(event *ypb.AIInputEvent) bool {
		endpoint := event.GetInteractiveId()
		if endpoints.CompareAndSwap(endpoint, true, false) {
			if ctx.Err() == nil {
				input.SafeFeed(event)
			}
			return true
		}
		_, known := endpoints.Load(endpoint)
		return known
	})
	forward := func(event *schema.AiOutputEvent) {
		forwarded := *event
		forwarded.CoordinatorId = id
		forwarded.TaskId = task.GetId()
		forwarded.TaskUUID = task.GetUUID()
		forwarded.CallToolID = callID
		if s.EventHandler != nil {
			s.EventHandler(&forwarded)
		}
	}
	var eventMu, deliveryMu sync.Mutex
	pending := make(map[string]struct{})
	closed := false
	childStarted := false
	interrupted := func(err error) error {
		eventMu.Lock()
		started := childStarted
		eventMu.Unlock()
		if !started {
			// The child checks this same context under eventMu before starting.
			releaseSlot()
			return errors.Join(err, aitool.ErrInvocationNotStarted)
		}
		return err
	}
	defer func() {
		eventMu.Lock()
		closed = true
		tree.Revision++
		tree.Status = subForgeTreeStatus(retErr)
		terminal := tree
		eventMu.Unlock()
		var retiredIDs []string
		endpoints.Range(func(key, _ any) bool {
			retiredIDs = append(retiredIDs, key.(string))
			return true
		})
		unregister(retiredIDs...)
		if len(pending) > 0 {
			// Keep legacy review release delivery serialized with child events.
			go func() {
				deliveryMu.Lock()
				defer deliveryMu.Unlock()
				defer func() {
					if failure := recover(); failure != nil {
						log.Errorf("sub-Forge review release handler panicked: %v", failure)
					}
				}()
				for endpoint := range pending {
					content, _ := json.Marshal(map[string]any{"id": endpoint, "params": nil})
					forward(&schema.AiOutputEvent{
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
			s.emitSubForgeTaskTree(terminal)
		}()
		select {
		case <-delivered:
		case <-ctx.Done():
		}
	}()

	childCtx := WithForgeParent(ctx, s.invoker, task, id)
	manager := buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(allowed...))
	opts := []aicommon.ConfigOption{
		aicommon.WithPersistentSessionId(""),
		aicommon.WithSessionPromptState(aicommon.NewSessionPromptState()),
		aicommon.WithTimeline(nil),
	}
	opts = append(opts, aicommon.SubForgeExecutionPolicy(s.Config)...)
	opts = append(opts,
		aicommon.WithInheritedAiToolManager(manager),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithClientUI(""), // The Yak entry routes ordinary review endpoints.
		aicommon.WithEnableToolManagerAISearch(false),
		aicommon.WithEnableSubagentsInPlan(false),
		aicommon.WithDisableCreateDBRuntime(s.DisableCreateDBRuntime),
		aicommon.WithWorkdir(filepath.Join(s.GetOrCreateWorkDir(), "artifacts", "sub-forges", id)),
		aicommon.WithEventInputChanx(input),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			deliveryMu.Lock()
			defer deliveryMu.Unlock()
			eventMu.Lock()
			if closed {
				eventMu.Unlock()
				return
			}
			endpoint := event.GetInteractiveId()
			if event.Type == schema.EVENT_TYPE_REVIEW_RELEASE {
				if _, exists := pending[endpoint]; !exists {
					eventMu.Unlock()
					return
				}
				delete(pending, endpoint)
				endpoints.Store(endpoint, false)
			} else {
				if ctx.Err() != nil {
					eventMu.Unlock()
					return
				}
				if endpoint != "" {
					if _, known := endpoints.LoadOrStore(endpoint, true); !known {
						pending[endpoint] = struct{}{}
					}
				}
			}
			changed := tree.observePlan(event)
			update := tree
			eventMu.Unlock()
			if changed {
				s.emitSubForgeTaskTree(update)
			}
			forward(event)
		}),
	)

	type childResult struct {
		value any
		err   error
	}
	done := make(chan childResult, 1)
	dispatched = true
	initial := tree
	go func() {
		var result childResult
		defer func() {
			if failure := recover(); failure != nil {
				result.err = fmt.Errorf("sub-Forge execution panicked: %v", failure)
			}
			releaseSlot()
			done <- result
		}()
		s.emitSubForgeTaskTree(initial)
		eventMu.Lock()
		result.err = childCtx.Err()
		childStarted = result.err == nil
		eventMu.Unlock()
		if result.err != nil {
			return
		}
		result.value, result.err = binding.Run(childCtx, params, allowed, opts...)
	}()
	select {
	case <-ctx.Done():
		return nil, interrupted(ctx.Err())
	case result := <-done:
		if err := ctx.Err(); err != nil {
			return nil, interrupted(err)
		}
		return result.value, result.err
	}
}

func (s *Session) subForgeAllowedTools(declared []*aitool.Tool) ([]*aitool.Tool, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	active, err := s.GetAiToolManager().GetEnableTools()
	if err != nil {
		return nil, err
	}
	authorized := make(map[*aitool.Tool]bool, len(active))
	for _, tool := range active {
		authorized[tool] = true
	}
	var allowed []*aitool.Tool
	for _, tool := range declared {
		if tool != nil && authorized[tool] {
			allowed = append(allowed, tool)
		}
	}
	return allowed, nil
}
