package aicommon

import (
	"fmt"
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

func (c *Config) GetEnableSelfReflection() bool {
	return c.EnableSelfReflection
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

func (c *Config) GetExecutionPolicy() string {
	if c == nil {
		return ""
	}
	lines := make([]string, 0, 4)
	if c.GetPreferDispatchSubReactAgents() {
		lines = append(lines,
			"- Multi-agent mode is ENABLED (preference, not hard-enforced): for any task with 2+ mostly-independent workstreams, your DEFAULT first move is dispatch_sub_react_agents, not serial tool calls. Only fall back to serial execution when the task is genuinely sequential or a single sub-goal.",
			"- Dispatch is for parallelizing independent workstreams, NOT for offloading one sequential task or dumping every subtask at once. Only batch subtasks you have confirmed are mutually independent; if B depends on A's result, do A first and dispatch B in a later iteration once A's result is in the timeline.",
			"- When dispatching, write a crisp goal for each sub agent and use result_contract to define the expected output shape whenever possible.",
		)
	}
	if c.GetEnableGoalMode() {
		lines = append(lines,
			fmt.Sprintf("- Goal mode is enabled: do not use finish before iteration %d.", c.GetGoalMinIterations()),
			"- Before the finish gate opens, only emit progress updates via directly_answer when necessary; keep pushing execution forward instead of wrapping up early.",
		)
	}
	if c.GetPreferDispatchSubReactAgents() && c.GetEnableGoalMode() {
		lines = append(lines,
			fmt.Sprintf("- Both modes are active: dispatch sub-agents for parallelizable work, but the top-level loop must still reach iteration %d before finishing; do not idle after dispatch — keep verifying sub-agent results or synthesize outputs until the gate opens.", c.GetGoalMinIterations()),
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
