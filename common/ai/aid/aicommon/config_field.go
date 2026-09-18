package aicommon

import (
	"fmt"
	"time"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"

	"sync/atomic"
)

func (c *Config) GetTimeline() *Timeline {
	return c.Timeline
}

func (c *Config) GetAIForgeManager() AIForgeFactory {
	return c.AiForgeManager
}

// LookupAIForgeForInvoke returns the forge definition used when invoking a blueprint by name.
// Resolution order matches aireact.ReAct.getForgeByName: ExtendedForge first, then AiForgeManager.
func (c *Config) LookupAIForgeForInvoke(forgeName string) (*schema.AIForge, error) {
	if c == nil {
		return nil, utils.Error("config is nil")
	}
	if forgeName == "" {
		return nil, utils.Error("forge name is empty")
	}
	for _, forge := range c.ExtendedForge {
		if forge != nil && forge.ForgeName == forgeName {
			return forge, nil
		}
	}
	if c.AiForgeManager == nil {
		return nil, utils.Error("AiForgeManager is not configured")
	}
	return c.AiForgeManager.GetAIForge(forgeName)
}

func (c *Config) GetForgeName() string {
	return c.ForgeName
}

func (c *Config) GetInputConsumption() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	input, _ := state.GetConsumptionPointers()
	if input == nil {
		return 0
	}
	return atomic.LoadInt64(input)
}

func (c *Config) GetOutputConsumption() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	_, output := state.GetConsumptionPointers()
	if output == nil {
		return 0
	}
	return atomic.LoadInt64(output)
}

func (c *Config) GetCacheHitToken() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	cacheHit := state.GetCacheHitTokenPointer()
	if cacheHit == nil {
		return 0
	}
	return atomic.LoadInt64(cacheHit)
}

func (c *Config) GetSequenceStart() int64 {
	return c.Seq
}

func (c *Config) GetLanguage() string {
	return c.Language
}

func (c *Config) GetEnablePlanAndExec() bool {
	return c.EnablePlanAndExec
}

func (c *Config) GetEnableDetachedPlan() bool {
	return c.EnableDetachedPlan
}

func (c *Config) GetEnableUserInteract() bool {
	return c.AllowRequireForUserInteract
}

func (c *Config) GetEnhanceKnowledgeManager() *EnhanceKnowledgeManager {
	return c.EnhanceKnowledgeManager
}

func (c *Config) GetDisableEnhanceDirectlyAnswer() bool {
	return c.DisableEnhanceDirectlyAnswer
}

func (c *Config) GetDisableIntentRecognition() bool {
	return c.DisableIntentRecognition
}

func (c *Config) GetSyncPerceptionTrigger() bool {
	return c.SyncPerceptionTrigger
}

func (c *Config) GetAiToolManager() *buildinaitools.AiToolManager {
	return c.AiToolManager
}

func (c *Config) GetTopToolsCount() int {
	return c.TopToolsCount
}

func (c *Config) GetShowForgeListInPrompt() bool {
	return c.ShowForgeListInPrompt
}

func (c *Config) GetMaxIterations() int64 {
	return c.GetMaxIterationCount()
}

func (c *Config) GetToolCallIntervalReviewExtraPrompt() string {
	return c.ToolCallIntervalReviewExtraPrompt
}

func (c *Config) GetPreferDispatchSubReactAgents() bool {
	if c == nil {
		return false
	}
	return c.PreferDispatchSubReactAgents
}

func (c *Config) GetMaxSubAgents() int64 {
	if c == nil {
		return DefaultMaxSubAgentConcurrency
	}
	n := c.MaxSubAgents
	if n <= 0 {
		return DefaultMaxSubAgentConcurrency
	}
	if n > AbsoluteMaxSubAgentConcurrency {
		return AbsoluteMaxSubAgentConcurrency
	}
	return n
}

func (c *Config) GetEnableGoalMode() bool {
	if c == nil {
		return false
	}
	return c.EnableGoalMode
}

func (c *Config) GetGoalMinIterations() int64 {
	if c == nil {
		return DefaultGoalMinIterations
	}
	return NormalizeGoalMinIterations(c.GoalMinIterations)
}

func NormalizeGoalMinIterations(n int64) int64 {
	if n <= 0 {
		return DefaultGoalMinIterations
	}
	return n
}

func (c *Config) GetGoalDurationSeconds() int64 {
	if c == nil {
		return 0
	}
	c.m.Lock()
	defer c.m.Unlock()
	return c.GoalDurationSeconds
}

func (c *Config) GetGoalAcceptanceCriteria() string {
	if c == nil {
		return ""
	}
	c.m.Lock()
	defer c.m.Unlock()
	return c.GoalAcceptanceCriteria
}

// GetGoalDeadline returns the computed deadline for the goal time window.
// Returns the zero time if the time window has not been started or is disabled.
func (c *Config) GetGoalDeadline() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	return c.GoalDeadline
}

// StartGoalDeadline computes and stores the goal-mode deadline from the
// current time + GoalDurationSeconds. If GoalDurationSeconds is 0 the
// deadline is not set (gate disabled). If -1 the deadline is set to a
// far-future sentinel (never-ending). This is called lazily on the first
// finish attempt, not at config creation, so the window measures actual
// execution time rather than session start time.
func (c *Config) StartGoalDeadline() {
	if c == nil {
		return
	}
	c.m.Lock()
	defer c.m.Unlock()
	if c.GoalDurationSeconds == 0 {
		return
	}
	if !c.GoalDeadline.IsZero() {
		return // already started
	}
	if c.GoalDurationSeconds == -1 {
		c.GoalDeadline = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
		return
	}
	c.GoalDeadline = time.Now().Add(time.Duration(c.GoalDurationSeconds) * time.Second)
}

// IsGoalDeadlinePassed reports whether the goal time window has elapsed.
// Returns false if the deadline has not been started or is disabled.
func (c *Config) IsGoalDeadlinePassed() bool {
	if c == nil {
		return false
	}
	c.m.Lock()
	deadline := c.GoalDeadline
	c.m.Unlock()
	if deadline.IsZero() {
		return false
	}
	return time.Now().After(deadline)
}

func (c *Config) GetExecutionPolicy() string {
	if c == nil {
		return ""
	}
	lines := make([]string, 0, 4)
	if c.GetPreferDispatchSubReactAgents() {
		lines = append(lines,
			"- 【MUST】Multi-agent mode is ENABLED. For ANY task containing 2+ mostly-independent workstreams, you MUST make dispatch_sub_react_agents your FIRST move — do not start with serial tool calls. This is a mandatory strategy, not a suggestion; only fall back to serial execution when you can justify that the task is genuinely sequential or a single sub-goal.",
			"- 【MUST】Dispatch is strictly for parallelizing INDEPENDENT workstreams. You MUST NOT use it to offload a single sequential task you should do yourself, and you MUST NOT dump every imaginable subtask into one call to avoid thinking. Before batching, you MUST verify each subtask is mutually independent.",
			"- 【MUST】If subtask B depends on subtask A's result, you MUST NOT batch them together. Do A first (dispatch it or do it yourself), wait for its result to land in the timeline, then dispatch B in a LATER iteration. Only zero-dependency subtasks may share one dispatch.",
			"- When dispatching, write a crisp, self-contained goal for each sub agent and use result_contract to define the expected output shape whenever possible.",
			"- Choose context_mode per child: fork (default) for work needing prior investigation; task_only for self-contained searches or independent reviews. task_only omits parent history, evidence and input attachments, so include required facts, constraints and file paths in goal. Neither mode changes host authority or cancellation scope.",
			"- Dispatch returns accepted job IDs immediately, not completed results. Continue useful independent work while children run. When no useful local work remains, use wait_sub_react_agents (default 30 seconds), inspect the returned progress, and decide again. A wait timeout never cancels child execution. Results arrive in a subsequent model input; finish requires all jobs settled and their results received. Use cancel_sub_react_agents to abandon unwanted work explicitly.",
		)
	}
	if c.GetEnableGoalMode() {
		lines = append(lines,
			"- Goal mode is enabled: finish is controlled by a host-side completion gate. Keep producing evidence-backed progress while finish is unavailable; decide from the task state rather than the gate.",
			"- Before the finish gate opens, only emit progress updates via directly_answer when necessary; keep pushing execution forward instead of wrapping up early or administratively clearing TODOs.",
		)
		if c.GetGoalDurationSeconds() != 0 {
			if c.GetGoalDurationSeconds() == -1 {
				lines = append(lines,
					"- Goal time window: NEVER-ENDING. finish will never be auto-accepted by the time gate; keep working until the acceptance criteria are met (if set) or the user stops the session.",
				)
			} else {
				lines = append(lines,
					fmt.Sprintf("- Goal time window: %d seconds. finish is blocked until the time window elapses; use the time to explore deeper, verify results, and address edge cases.", c.GetGoalDurationSeconds()),
				)
			}
		}
		if criteria := c.GetGoalAcceptanceCriteria(); criteria != "" {
			lines = append(lines,
				fmt.Sprintf("- Goal acceptance criteria: %s", criteria),
				"- After the time window (if set) passes, finish still requires the acceptance criteria to be satisfied. The system will review your work against the criteria before allowing finish.",
			)
		}
	}
	if c.GetPreferDispatchSubReactAgents() && c.GetEnableGoalMode() {
		lines = append(lines,
			"- Both modes are active: dispatch sub-agents for parallelizable work, then keep verifying their results or synthesizing outputs until the completion gate opens; do not idle after dispatch or attempt to satisfy the gate by closing unfinished TODOs.",
		)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func EnsureGoalModeMaxIterations(maxIterations, goalMinIterations int64) int64 {
	goalMinIterations = NormalizeGoalMinIterations(goalMinIterations)
	if maxIterations <= 0 {
		return maxIterations
	}
	minRequired := goalMinIterations + GoalModeIterationBuffer
	if maxIterations < minRequired {
		return minRequired
	}
	return maxIterations
}
