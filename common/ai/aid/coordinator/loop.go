package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var planInstruction = promptloader.MustLoad("ai/aid/coordinator/planning.txt")
var instruction = promptloader.MustLoad("ai/aid/coordinator/instruction.txt")
var reportInstruction = promptloader.MustLoad("ai/aid/coordinator/reporting.txt")
var planningOnlyInstruction = promptloader.MustLoad("ai/aid/coordinator/planning_only.txt")

func WithController(c *Controller) reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) { loop.Set("coordinator_controller", c) }
}

// WithPlanningOnly retains the old plan-only entrypoint without adding a
// separate planner loop. Its finish gate requires approval, not execution.
func WithPlanningOnly() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) { loop.Set("coordinator_planning_only", true) }
}

func planningOnly(loop *reactloops.ReActLoop) bool {
	value, _ := loop.GetVariable("coordinator_planning_only").(bool)
	return value
}

func controller(loop *reactloops.ReActLoop) *Controller {
	c, _ := loop.GetVariable("coordinator_controller").(*Controller)
	return c
}

var ActionNames = []string{"create_plan", "modify_plan", "submit_plan", "wait_messages", "inspect_task", "review_task", "retry_task", "cancel_tasks", "create_report", "modify_report", "submit_report"}

// NewLoop uses the main loop's configured action protocol and Timeline assembly.
// The tool guard is per-loop, so execution workers retain their own tool policy.
func NewLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	// These are planning preferences, scoped to this coordinator's role
	// instructions (semi-dynamic 2), never worker instructions or high static.
	var owningController *Controller
	var planningOnlyForOwner bool
	var preferences strings.Builder
	if global := yakit.GetCachedAIGlobalConfig(); global != nil && strings.TrimSpace(global.GetAIPlanPrompt()) != "" {
		fmt.Fprintf(&preferences, "\n<planning_preferences>\n%s\n</planning_preferences>\n", global.GetAIPlanPrompt())
	}
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok && strings.TrimSpace(cfg.PlanPrompt) != "" {
		fmt.Fprintf(&preferences, "\n<user_planning_preferences>\n%s\n</user_planning_preferences>\n", cfg.PlanPrompt)
	}
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		_ = aicommon.WithLiteForgeExecutor(executeNativeHelper)(cfg)
		_ = aicommon.WithAiAgreeRiskControl(NativeRiskReview)(cfg)
		_ = aicommon.WithDisableDynamicPlanning(true)(cfg)
	}
	explorationConfig := false
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		explorationConfig = cfg.EnableSubagentsInPlan
		cfg.EnableDispatchSubReactAgents = explorationConfig
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithFunctionCallActionVariants(), reactloops.WithAllowToolCall(true), reactloops.WithAllowUserInteract(true), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentContextProvider(func(l *reactloops.ReActLoop, _ string) (string, error) {
			template := instruction
			if controller(l).Snapshot().Phase == PhasePlan {
				template = planInstruction
			} else if !planningOnly(l) && controller(l).ReportReady() {
				template = reportInstruction
			}
			text, err := utils.RenderTemplate(template, map[string]any{"FunctionCallMode": l.FunctionCallModeEnabled(), "ExplorationEnabled": explorationEnabled(l)})
			if err != nil {
				return "", err
			}
			if controller(l).Snapshot().Phase == PhasePlan {
				text += preferences.String()
			}
			if planningOnly(l) {
				return text + "\n" + planningOnlyInstruction, nil
			}
			return text, nil
		}),
		reactloops.WithReactiveDataBuilder(func(_ *reactloops.ReActLoop, b *bytes.Buffer, _ string) (string, error) { return b.String(), nil }),
		reactloops.WithToolInvokeGuard(func(name string, params aitool.InvokeParams) (bool, string) {
			workdir := ""
			if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
				workdir = cfg.GetOrCreateWorkDir()
			}
			if AllowedTool(name, params, workdir) {
				return true, ""
			}
			return false, "Coordinator cannot execute this tool. Put business execution in a plan task."
		}),
		reactloops.WithActionFilter(func(a *reactloops.LoopAction) bool {
			for _, name := range ActionNames {
				if a.ActionType == name {
					return owningController.actionAllowed(name)
				}
			}
			if a.ActionType == "dispatch_sub_react_agents" || reactloops.IsSubAgentControlAction(a.ActionType) {
				return owningController.Snapshot().Phase == PhasePlan && explorationConfig
			}
			switch a.ActionType {
			case "finish":
				return owningController.Snapshot().Phase == PhasePlan || planningOnlyForOwner
			case "directly_answer", "save_evidence", "adjust_todolist", "require_tool", "directly_call_tool", "ask_for_clarification", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities":
				return true
			}
			return false
		}),
	}
	preset = append(preset, actionOptions()...)
	preset = append(preset, opts...)
	preset = append(preset, reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true))
	loop, err := reactloops.NewReActLoop(Name, r, preset...)
	if err != nil {
		return nil, err
	}
	if controller(loop) == nil {
		return nil, fmt.Errorf("coordinator requires an owning Session or WithController")
	}
	if err := controller(loop).attachResultTimeline(loop.GetConfig()); err != nil {
		return nil, err
	}

	owningController = controller(loop)
	planningOnlyForOwner = planningOnly(loop)
	if err := configureExploration(loop); err != nil {
		return nil, err
	}
	// Preserve the existing TODO/goal/subagent completion gates as well.
	reactloops.WithPlanStatusProvider(func() string {
		c := controller(loop)
		through, err := c.deliverMessages()
		if err != nil {
			loop.Set("coordinator_wait_error", err)
		}
		loop.Set("coordinator_message_cursor", through)
		s, revision := c.contextSnapshot()
		status := s.PromptStatus()
		if m := loop.GetSubAgentManager(); m != nil {
			cursor, _ := m.ChangeCursor()
			loop.Set("coordinator_exploration_cursor", cursor)
			jobs, _ := m.Inspect(nil)
			for _, job := range jobs {
				status += fmt.Sprintf("探索 %s [%s]: %s; cleanup_pending=%t\n", job.Identifier, job.ID, job.State, job.CleanupPending)
			}
		}
		loop.Set("coordinator_observed_user_revision", s.UserRevision)
		loop.Set("coordinator_event_revision", revision)
		return status
	})(loop)
	if err := configureCoordinatorFinish(loop); err != nil {
		return nil, err
	}
	if err := configureCoordinatorAnswer(loop); err != nil {
		return nil, err
	}
	configureDecisionBoundary(loop)
	configureAutomaticWait(loop)
	return loop, nil
}

func planData(a *aicommon.Action) string {
	data, _ := json.Marshal(a.GetInvokeParams("plan"))
	return string(data)
}

func init() {
	_ = reactloops.RegisterLoopFactory(Name, NewLoop,
		reactloops.WithLoopDescription("计划协调：PLAN 调查并提交审核；EXEC 自动 DAG 调度、消息驱动审核与调整，最终提交 artifacts 报告。"),
		reactloops.WithVerboseName("Coordinator"), reactloops.WithVerboseNameZh("任务协调"),
		reactloops.WithLoopDescriptionZh("计划协调运行体：探索、审批、调度、等待、验收、重试和报告，兼容已有 PLAN 与 Yakit 通道。"))
}
