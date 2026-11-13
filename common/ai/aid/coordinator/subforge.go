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

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
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
		tool.ExclusiveBatchGroup = "sub-Forge"
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
	call := newSubForgeCall(ctx, s, task, callID, binding, releaseSlot)
	defer func() { call.finish(retErr) }()
	childCtx := WithForgeParent(ctx, s.invoker, task, call.tree.ChildCoordinatorID)
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
		aicommon.WithWorkdir(filepath.Join(s.GetOrCreateWorkDir(), "artifacts", "sub-forges", call.tree.ChildCoordinatorID)),
		aicommon.WithEventInputChanx(call.input),
		aicommon.WithEventHandler(call.handleEvent),
	)
	dispatched = true
	return call.run(childCtx, binding, params, allowed, opts...)
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
