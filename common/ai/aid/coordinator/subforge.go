package coordinator

import (
	"context"
	"encoding/json"
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

func (s *Session) invokeSubForge(ctx context.Context, task aicommon.AIStatefulTask, callID string, binding SubForgeTool, params aitool.InvokeParams) (any, error) {
	if IsSubForgeContext(ctx) || IsSubForgeContext(s.GetContext()) {
		return nil, fmt.Errorf("sub-Forge nesting is limited to one level")
	}
	if s.IsToolUseDisabled() {
		return nil, fmt.Errorf("tool execution is disabled")
	}
	if !s.subForgeActive.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("another sub-Forge is already running")
	}
	defer s.subForgeActive.Store(false)

	ctx, cancel := context.WithTimeout(context.WithValue(ctx, subForgeContextKey{}, true), subForgeTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	allowed, err := s.subForgeAllowedTools(binding.Tools)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
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
	defer func() {
		var retiredIDs []string
		endpoints.Range(func(key, _ any) bool {
			retiredIDs = append(retiredIDs, key.(string))
			return true
		})
		unregister(retiredIDs...)
	}()

	childCtx := WithForgeParent(ctx, s.invoker, task, id)
	manager := buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(allowed...))
	opts := append(aicommon.SubForgeExecutionPolicy(s.Config),
		aicommon.WithInheritedAiToolManager(manager),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithClientUI(""), // The Yak entry routes ordinary review endpoints.
		aicommon.WithEnableToolManagerAISearch(false),
		aicommon.WithEnableSubagentsInPlan(false),
		aicommon.WithWorkdir(filepath.Join(s.GetOrCreateWorkDir(), "artifacts", "sub-forges", id)),
		aicommon.WithEventInputChanx(input),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if endpoint := event.GetInteractiveId(); endpoint != "" {
				if event.Type == schema.EVENT_TYPE_REVIEW_RELEASE {
					endpoints.Store(endpoint, false)
				} else {
					endpoints.LoadOrStore(endpoint, true)
				}
			}
			forwarded := *event
			forwarded.CoordinatorId = id
			forwarded.TaskId = task.GetId()
			forwarded.TaskUUID = task.GetUUID()
			forwarded.CallToolID = callID
			if s.EventHandler != nil {
				s.EventHandler(&forwarded)
			}
		}),
	)
	result, err := binding.Run(childCtx, params, allowed, opts...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, err
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
