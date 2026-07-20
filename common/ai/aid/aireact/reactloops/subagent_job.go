package reactloops

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// SubAgentJob 是子 Agent 运行的统一描述符。它涵盖了原先 DispatchJob /
// ForkJob / NestedJob 三种类型所携带的全部字段；不同的行为差异（fork 与
// in-place、AI 润色 goal 与原始 goal、loop-factory 与 loop-name）由调用方 /
// runner 选择，而非由类型区分。
type SubAgentJob struct {
	// Order 是 1 起的序号，用于结果排序 / 展示。
	Order int `json:"order"`
	// Identifier 是子 Agent 的稳定标签（如 "scan_host_a"）。
	Identifier string `json:"identifier"`
	// Goal 是简要意图；当 UserInput 为空时作为子任务的输入，或在 dispatch
	// 路径中传给 goal 润色步骤。
	Goal string `json:"goal"`
	// TaskName 是展示在 timeline / UI 中的人类可读任务名。
	TaskName string `json:"task_name"`
	// UserInput 非空时覆盖 Goal，作为子任务的用户输入。
	UserInput string `json:"user_input,omitempty"`
	// ResultContract 是可选的验收标准 / 输出格式提示，在 fork / dispatch 路径
	// 中追加到用户输入后面。
	ResultContract string `json:"result_contract,omitempty"`
	// LoopName 是已注册的 ReAct loop factory 名称。为空时在 dispatch / nested
	// 路径中默认取 schema.AI_REACT_LOOP_NAME_DEFAULT。
	LoopName string `json:"loop_name"`
	// ForkTimeline 控制 nested 路径的 timeline 隔离方式：
	//   true  → fork 父 timeline（分支 diff 可用，类似 RunForkedJob）
	//   false → 在父 timeline 上原地运行（运行结束后回滚条目，类似
	//           RunNestedLoop）
	// dispatch 路径始终 fork，忽略此标记。
	ForkTimeline bool `json:"fork_timeline,omitempty"`
}

// SubAgentResult 是子 Agent 运行的统一结果。它内嵌 SubAgentJob，使身份字段
//（Order / Identifier / ...）自动提升，且非泛型的 runJobsConcurrently 可以
// 把同一个切片既当作 job 载体又当作结果。结果字段的并集（SubLoop / SubTask /
// Record / Feedback / ...）覆盖了原先所有结果类型；未使用的字段保持零值即可。
type SubAgentResult struct {
	SubAgentJob

	// SubTaskID 是子 Agent 任务 ID（fork 路径设置）。
	SubTaskID string
	// SubTask 是子 Agent 的 stateful task（fork / nested-in-place 路径）。
	SubTask aicommon.AIStatefulTask
	// SubLoop 是执行完成的 ReActLoop；成功时必定设置，创建失败时可能为 nil。
	SubLoop *ReActLoop
	// Fork 是 timeline fork 句柄（仅 fork 路径）。
	Fork *aicommon.TimelineFork
	// ExecErr 是执行错误，成功时为 nil。
	ExecErr error
	// Duration 是运行时长。
	Duration time.Duration
	// Record 是结构化 timeline 记录（dispatch 路径）。
	Record TimelineRecord
	// Feedback 是简短的人类可读摘要（dispatch 路径）。
	Feedback string
}

// ForkLoopFactory 构建在 fork 子 Agent 中执行的 ReAct loop。
type ForkLoopFactory func(childInvoker aicommon.AIInvokeRuntime, job SubAgentJob) (*ReActLoop, error)

// ForkInvokerCallback 在 fork 出的子 invoker 上执行任意逻辑，但不启动 ReAct
// loop。timeline 噪音留在分支上；父 timeline 不会被截断。
type ForkInvokerCallback func(childInvoker aicommon.AIInvokeRuntime, childTask aicommon.AIStatefulTask) error
