package aicommon

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"

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
	if c.EnableDispatchSubReactAgents && !c.GetPreferDispatchSubReactAgents() {
		lines = append(lines,
			"- 默认模式：维护总目标和主线；复杂长链路、跨源验证和有依据的探索分支，尽早通过 dispatch_sub_react_agents 委派。一个子任务可完成含顺序步骤的阶段，无须凑并行任务。简单问答和微小单步任务直接处理，遵守用户的委派限制。",
			"- 派发时写清输入、约束、范围、交付和验收要求（result_contract）；探索另给假设和停止条件。同批任务须独立且无冲突；有依赖则先验收前序，再派发后序。",
			"- 主智能体负责协调、验收和汇总，也可推进必要主线工作。受理不等于完成；无可推进工作时用 wait_sub_react_agents 等待。阶段结束后核对总目标和剩余待办，再决定下一阶段或 finish。",
		)
	}
	if c.GetPreferDispatchSubReactAgents() {
		lines = append(lines,
			"- 【强制】多智能体模式：主智能体以拆解、协调、验收和汇总为主。复杂多步任务在必要摸底后，首先通过 dispatch_sub_react_agents 委派完整阶段或探索分支，资料处理、深入调查和产出制作交给子智能体。单个顺序阶段也可委派；简单问答、微小单步任务及用户禁止委派时除外。",
			"- 子任务说明须包含输入、约束、范围、交付、证据、未确认项和阻塞条件（result_contract）；探索另给假设和停止条件。主智能体维护主线待办，根据真实发现选择后续工作，避免重复执行已委派的细节。",
			"- 【强制】同批任务须独立且无冲突。若后序依赖前序，先派发并验收前序，再携带真实结果派发后序；子任务内部允许顺序执行。",
			"- context_mode 默认 fork，继承派发时的上下文快照；独立检索或审查可用 task_only，但须在 goal 中写全事实、约束和文件路径，因为它不继承父对话、证据和输入附件。两者均不改变宿主权限和取消范围。",
			"- 受理不等于完成。子任务执行时继续协调和验收；无可推进工作时用 wait_sub_react_agents 有界等待，默认 30 秒，超时不取消子任务。全部子任务结束、结果已接收且总目标无遗漏后才可 finish；放弃任务须显式 cancel_sub_react_agents。",
		)
	}
	if c.GetEnableGoalMode() {
		lines = append(lines,
			"- 目标模式：finish 由宿主完成门禁控制；根据真实任务状态持续推进，用具体证据支撑进展。",
			"- 门禁开放前，仅在必要时用 directly_answer 汇报进度；不要反复请求 finish，也不要清空未完成待办来过关。",
		)
		if c.GetGoalDurationSeconds() != 0 {
			if c.GetGoalDurationSeconds() == -1 {
				lines = append(lines,
					"- 目标时间窗：无限。时间门禁不会自动放行 finish；持续推进，直至满足已设置的验收标准或用户停止会话。",
				)
			} else {
				lines = append(lines,
					fmt.Sprintf("- 目标时间窗：%d 秒。到期前不能 finish；继续深入探索、验证结果和补齐边界情况。", c.GetGoalDurationSeconds()),
				)
			}
		}
		if criteria := c.GetGoalAcceptanceCriteria(); criteria != "" {
			lines = append(lines,
				fmt.Sprintf("- 目标验收标准：%s", criteria),
				"- 达到时间要求后，仍须通过验收审查才能 finish。",
			)
		}
	}
	if c.GetPreferDispatchSubReactAgents() && c.GetEnableGoalMode() {
		lines = append(lines,
			"- 多智能体与目标模式同时启用：持续委派、协调和验收，直至完成门禁开放；派发后不能停工，也不能靠清空待办收口。",
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
