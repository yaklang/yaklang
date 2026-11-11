package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gopkg.in/yaml.v3"
)

const toolParamAITagActionKeyPrefix = "__aitag__"

// RecentToolCacheStableNonce 是 CACHE_TOOL_CALL 块及其内部所有 AITAG (TOOL_xxx /
// TOOL_PARAM_xxx) 渲染时使用的稳定 nonce 字面量. 跨 react turn 不变, 让承载
// 该块的 prompt 段保持字节级稳定, 进入 prefix cache.
//
// 字面量选 "[current-nonce]" 带方括号占位符语义, 用意:
//   - 让 LLM 一眼看出"这是个占位符, 应该替换为 prompt 上下文里的 current nonce
//     (USER_QUERY 等其他 AITAG 用的 turn nonce)"
//   - 即使 LLM 不替换、直接照抄字面量输出, ActionMaker 端通过 ExtraNonces
//     双注册也能命中 (turn nonce + [current-nonce] 同时注册 callback)
//
// 必须与渲染侧 (Timeline promoted recent-tool-cache state) 与解析侧
// (reactloops.syncRecentToolParamAITagFields 注册的 LoopAITagField.ExtraNonces)
// 保持一致, 否则字面量被改变后任一侧落后都会导致解析丢失.
//
// 关键词: RecentToolCacheStableNonce, [current-nonce], 占位符语义,
//
//	prefix cache 字节稳定, 双注册兜底
const RecentToolCacheStableNonce = "[current-nonce]"

// LiteralCurrentNoncePlaceholder 是各 react loop 在 persistent_instruction /
// output_example 等示例 prompt 里使用的 nonce 占位符字面量, 例如
// `<|CONTENT_CURRENT_NONCE|>` / `<|FINAL_ANSWER_CURRENT_NONCE|>` /
// `<|GEN_CODE_CURRENT_NONCE|>` 等.
//
// 设计本意是让 AI 把 `CURRENT_NONCE` 替换为本 turn 实际生效的 nonce. 但实测
// 部分模型会把这个占位符当作字面量直接照抄输出, 导致 AITag 解析器只用 turn
// nonce 注册 callback 时根本匹配不到, 内容丢失, verifier 误判为"AI 没提供
// 内容", 触发 verifier 重试甚至任务失败。
//
// 为了兼容这种照抄行为, ReActLoop.buildActionTagOption 会默认把这个字面量
// 作为 ExtraNonces 候选注册, 与 turn nonce 并列双注册, AI 用任一格式输出
// AITag 块都能被正确捕获到 action 字段.
//
// 关键词: LiteralCurrentNoncePlaceholder, CURRENT_NONCE 字面量兼容,
//
//	AI 占位符照抄, AITag 双注册兜底
const LiteralCurrentNoncePlaceholder = "CURRENT_NONCE"

func GetToolParamAITagActionKey(paramName string) string {
	return toolParamAITagActionKeyPrefix + paramName
}

func IsSupportedToolParamAITagName(paramName string) bool {
	if paramName == "" {
		return false
	}
	for _, ch := range paramName {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func FilterSupportedToolParamAITagNames(paramNames []string) []string {
	if len(paramNames) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(paramNames))
	seen := make(map[string]struct{}, len(paramNames))
	for _, paramName := range paramNames {
		if !IsSupportedToolParamAITagName(paramName) {
			continue
		}
		if _, ok := seen[paramName]; ok {
			continue
		}
		seen[paramName] = struct{}{}
		filtered = append(filtered, paramName)
	}
	return filtered
}

func MergeActionAITagParams(action *Action, invokeParams aitool.InvokeParams, paramNames []string) []string {
	if action == nil || len(paramNames) == 0 {
		return nil
	}
	if invokeParams == nil {
		invokeParams = make(aitool.InvokeParams)
	}

	merged := make([]string, 0, len(paramNames))
	for _, paramName := range paramNames {
		if paramName == "" {
			continue
		}
		if aitagValue := action.GetString(GetToolParamAITagActionKey(paramName)); aitagValue != "" {
			invokeParams.Set(paramName, aitagValue)
			merged = append(merged, paramName)
		}
	}
	return merged
}

type ToolCaller struct {
	runtimeId  string
	task       AITask
	config     AICallerConfigIf
	emitter    *Emitter // specific, backup for config.GetEmitter()
	start      *sync.Once
	done       *sync.Once
	callToolId string
	startTime  time.Time // Track tool call start time
	reason     string    // human-readable reason for this tool call (preset / action / liteforge-generated)

	reasonGen bool // liteforge reason generation has run for this tool call (at most once; reset by review)

	// invokeRuntime supplies current-task user input to script tools when the
	// caller has no stateful task.
	invokeRuntime AIInvokeRuntime

	ctx    context.Context
	cancel context.CancelFunc

	detachHandler func() (string, error)
	onDetach      func()
	detachOnce    sync.Once
	detached      atomic.Bool

	m               *sync.Mutex
	onCallToolStart func(callToolId string)
	onCallToolEnd   func(callToolId string)

	intervalReviewHandler  func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error)
	intervalReviewDuration time.Duration // interval duration for review, default is DefaultToolCallIntervalReviewDuration

	reviewReconsiderHandler func(context.Context, *aitool.Tool, aitool.InvokeParams, aitool.InvokeParams) string

	paramAugment        func(aitool.InvokeParams) aitool.InvokeParams               // legacy scalar callback
	paramAugmentForTool func(*aitool.Tool, aitool.InvokeParams) aitool.InvokeParams // current-proposal-aware callback

	callExpectations           string
	omitResultParamsInTimeline bool

	// Batch-only scheduling hooks. Scalar callers leave these nil and retain
	// their historical behaviour. The hooks deliberately live on ToolCaller so
	// interactive review and the actual plugin invoke can
	// be scheduled independently without duplicating the tool-call pipeline.
	reviewGate   ToolCallerGate
	beforeInvoke ToolCallerBeforeInvoke

	// Runtime-owned deterministic metadata. These values are allocated before a
	// batch starts, so goroutine completion order cannot change checkpoint,
	// result or artifact ordering.
	checkpointSeq         int64
	reviewCheckpointSeq   int64
	watcherCheckpointSeq  int64
	resultID              int64
	artifactOrdinal       int
	destinationIdentifier string
	batchID               string
	batchIndex            int
	statsSource           string
	checkpointReplayed    bool

	// Inspector reads these original invocation buffers and the frozen final
	// result. No event subscription or second output stream is involved.
	inspectionMu            sync.RWMutex
	inspectionStdout        *toolOutputBuffer
	inspectionStderr        *toolOutputBuffer
	inspectionResult        json.RawMessage
	inspectionError         error
	waitForCallbackOnCancel bool
	asyncResourceBound      bool // guarded by m; one resource owns this caller
	executionStarted        bool // guarded by m; invocation has started

	terminalOnce sync.Once
}

// ToolCallReviewResult is the approved, fully prepared proposal. Execution
// accepts only this value and never performs another review.
type ToolCallReviewResult struct {
	Tool                  *aitool.Tool
	Params                aitool.InvokeParams
	DestinationIdentifier string
	DirectlyAnswer        bool

	caller *ToolCaller
}

func (r *ToolCallReviewResult) ToolCaller() *ToolCaller {
	if r == nil {
		return nil
	}
	return r.caller
}

func (r *ToolCallReviewResult) CallToolID() string {
	if r == nil || r.caller == nil {
		return ""
	}
	return r.caller.CallToolID()
}

func (r *ToolCallReviewResult) ResultID() int64 {
	if r == nil || r.caller == nil {
		return 0
	}
	return r.caller.ResultID()
}

func (r *ToolCallReviewResult) Execute() (*aitool.ToolResult, bool, error) {
	if r == nil || r.caller == nil {
		return nil, false, fmt.Errorf("an approved tool review is required")
	}
	return r.caller.ExecuteReviewedToolCall(r)
}

func (r *ToolCallReviewResult) Abort(err error) {
	if r == nil || r.caller == nil {
		return
	}
	r.caller.finishToolCallError(err)
	r.caller.Cancel()
}

func (r *ToolCallReviewResult) Cancel() {
	if r == nil || r.caller == nil {
		return
	}
	r.caller.Cancel()
}

func (r *ToolCallReviewResult) BindAsyncExecutionID(executeID string) {
	if r == nil || r.caller == nil || executeID == "" {
		return
	}
	WithToolCaller_AsyncExecutionID(executeID)(r.caller)
}

func (r *ToolCallReviewResult) NewAsyncResource(onResult ...func(*aitool.ToolResult, bool, error)) (asyncexecmanager.Resource, error) {
	if r == nil || r.caller == nil {
		return nil, fmt.Errorf("an approved tool review is required")
	}
	return r.caller.NewReviewedAsyncResource(r, onResult...)
}

func (t *ToolCaller) finalizeToolCall(result *aitool.ToolResult) {
	if t == nil {
		return
	}
	t.terminalOnce.Do(func() {
		if t.emitter != nil {
			summaryResult := result
			if summaryResult == nil {
				summaryResult = &aitool.ToolResult{}
			}
			t.emitter.EmitToolCallSummary(t.callToolId, SummaryRank(t.task, summaryResult))
		}
		if t.callToolId != "" {
			if useful, err := yakit.UsefulRuntimeId(consts.GetGormProjectDatabase(), t.callToolId); err == nil && useful {
				t.config.AppendRelatedRuntimeID(t.callToolId)
			}
		}
		t.publishInspectionResult(result)
	})
}

// ToolCallerGate bounds one phase of a tool call. The returned release
// function must be idempotent; ToolCaller invokes it on every exit path.
type ToolCallerGate func(context.Context) (release func(), err error)

// ToolCallerBeforeInvoke runs after parameter validation and user review but
// immediately before plugin execution. Batch runtimes use it as an all-calls
// barrier and as the bounded invoke semaphore. cleanup is called after invoke.
type ToolCallerBeforeInvoke func(
	context.Context,
	*aitool.Tool,
	aitool.InvokeParams,
) (cleanup func(), err error)

func idempotentToolCallerRelease(release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(release)
	}
}

const ReservedKeyCallExpectations = "__call_expectations__"
const ReservedKeyIdentifier = "__identifier__"

// DefaultToolCallIntervalReviewDuration limits how often a long-running tool
// wakes the speed-priority model for a progress review. Keep every fallback on
// this shared value so Config, ReAct, and ToolCaller cannot drift apart.
const DefaultToolCallIntervalReviewDuration = 60 * time.Second

type toolCallIntervalReviewMetadataContextKey struct{}

// ToolCallIntervalReviewMetadata carries scheduler-owned timing into the
// review handler without changing the long-standing handler signature.
type ToolCallIntervalReviewMetadata struct {
	ToolExecutionStartedAt time.Time
	ReviewCount            int
}

func withToolCallIntervalReviewMetadata(ctx context.Context, metadata ToolCallIntervalReviewMetadata) context.Context {
	return context.WithValue(ctx, toolCallIntervalReviewMetadataContextKey{}, metadata)
}

// ToolCallIntervalReviewMetadataFromContext returns the actual tool execution
// start and scheduler review count for the current progress check.
func ToolCallIntervalReviewMetadataFromContext(ctx context.Context) (ToolCallIntervalReviewMetadata, bool) {
	if ctx == nil {
		return ToolCallIntervalReviewMetadata{}, false
	}
	metadata, ok := ctx.Value(toolCallIntervalReviewMetadataContextKey{}).(ToolCallIntervalReviewMetadata)
	return metadata, ok
}

type ToolCallerOption func(tc *ToolCaller)

func WithToolCaller_CallExpectations(expectations string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.callExpectations = expectations
	}
}

// WithToolCaller_ReviewGate serializes the interactive approval endpoint. The
// gate is released before applying a review decision, which keeps legacy
// wrong-tool/wrong-params recursion re-entrant while ensuring only one pending
// approval card exists at a time.
func WithToolCaller_ReviewGate(gate ToolCallerGate) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reviewGate = gate
	}
}

// WithToolCaller_BeforeInvoke installs the final batch barrier/invoke gate.
func WithToolCaller_BeforeInvoke(hook ToolCallerBeforeInvoke) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.beforeInvoke = hook
	}
}

func WithToolCaller_CheckpointSeq(seq int64) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.checkpointSeq = seq
	}
}

func WithToolCaller_ReviewCheckpointSeq(seq int64) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reviewCheckpointSeq = seq
	}
}

func WithToolCaller_WatcherCheckpointSeq(seq int64) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.watcherCheckpointSeq = seq
	}
}

func WithToolCaller_ResultID(id int64) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.resultID = id
	}
}

func WithToolCaller_ArtifactOrdinal(ordinal int) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.artifactOrdinal = ordinal
	}
}

func WithToolCaller_DestinationIdentifier(identifier string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.destinationIdentifier = sanitizeIdentifier(identifier)
	}
}

func WithToolCaller_BatchMetadata(batchID string, batchIndex int) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.batchID = batchID
		tc.batchIndex = batchIndex
	}
}

// WithToolCaller_StatsSource records how this logical tool call was selected.
// The source is explicit call metadata rather than being inferred from the tool
// name or action text, so scalar calls, batch children and review replacements
// all retain the source of the original model decision.
func WithToolCaller_StatsSource(source string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.statsSource = normalizeToolCallStatsSource(source)
	}
}

// WithToolCaller_OmitResultParamsInTimeline is used when the final params were
// already recorded by an earlier, dedicated timeline item. It only affects the
// timeline rendering hint on the returned ToolResult; invocation and persisted
// tool-call reports still retain the complete params.
func WithToolCaller_OmitResultParamsInTimeline() ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.omitResultParamsInTimeline = true
	}
}

// WithToolCaller_Reason presets the human-readable reason emitted by the explicit call flow.
func WithToolCaller_Reason(reason string) ToolCallerOption {
	return func(tc *ToolCaller) { tc.reason = reason }
}

// WithToolCaller_InvokeRuntime supplies task context for script-tool execution.
func WithToolCaller_InvokeRuntime(rt AIInvokeRuntime) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.invokeRuntime = rt
	}
}

var toolCallReasonOutputs = []aitool.ToolOption{
	aitool.WithStringParam("reason",
		aitool.WithParam_Description("A terse phrase (at most 30 characters, including spaces) stating WHAT this tool call does right now. No transitions or prior-step summaries. Match the language of the user input."),
		aitool.WithParam_MaxLength(30),
		aitool.WithParam_Required(true)),
}

// buildToolCallReasonPrompt builds the lite-forge prompt asking for a concise
// reason for a tool call, given the tool, its (possibly empty) params, and the
// owning task's user input / name for intent context. It also includes a brief
// summary of recent tool-call results so the generated reason can reference the
// specific progress that motivates this call. The optional intent identifier
// distinguishes siblings that use the same tool and have no explicit reason.
func buildToolCallReasonPrompt(tool *aitool.Tool, params aitool.InvokeParams, task AITask, intentIdentifier string) string {
	var sb strings.Builder
	sb.WriteString("Generate a terse reason (at most 30 characters, including spaces) stating WHAT this tool call does right now. " +
		"Focus on the concrete current action, not on prior steps or transitions. " +
		"Bad: '端口扫描完成，接下来需要执行简单爬虫收集页面' / 'previous scan found open ports, now crawling'. " +
		"Good: '爬取目标站点页面与API端点' / 'crawl site pages and APIs'.\n")
	sb.WriteString(fmt.Sprintf("Tool: %s\n", tool.Name))
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		sb.WriteString(fmt.Sprintf("Tool description: %s\n", desc))
	}
	if intentIdentifier != "" {
		sb.WriteString(fmt.Sprintf("This call's intent identifier: %s\n", intentIdentifier))
		sb.WriteString("Use this intent to distinguish this call from other calls of the same tool; express it naturally rather than copying snake_case.\n")
	}
	if task != nil {
		if name := strings.TrimSpace(task.GetName()); name != "" {
			sb.WriteString(fmt.Sprintf("Task: %s\n", name))
		}
		// GetUserInput lives on AIStatefulTask (the concrete task type used by the
		// ReAct runtime path); AITask itself does not expose it. Type-assert so the
		// intent context is included when available, and silently skip otherwise.
		if stateful, ok := task.(AIStatefulTask); ok {
			if userInput := strings.TrimSpace(stateful.GetUserInput()); userInput != "" {
				sb.WriteString(fmt.Sprintf("User input: %s\n", userInput))
			}
		}

		if recentSteps := buildRecentToolCallSummary(task, 5); recentSteps != "" {
			sb.WriteString("Recent steps (use these to contextualize your reason):\n")
			sb.WriteString(recentSteps)
		}
	}
	if len(params) > 0 {
		sb.WriteString("Params:\n")
		sb.WriteString(renderParamsAsYAML(params))
	}
	sb.WriteString("\nMatch the language of the user input (Chinese if the user writes in Chinese, English otherwise).\nOutput only the reason in the `reason` field.")
	return sb.String()
}

// buildRecentToolCallSummary returns a short markdown summary of the most
// recent N tool-call results from the task, oldest first. Status describes the
// invocation protocol only; execution semantics remain in the tool result.
// Returns "" when no prior results exist.
func buildRecentToolCallSummary(task AITask, maxItems int) string {
	if task == nil {
		return ""
	}
	results := task.GetAllToolCallResults()
	if len(results) == 0 {
		return ""
	}

	start := 0
	if len(results) > maxItems {
		start = len(results) - maxItems
	}
	var sb strings.Builder
	for _, r := range results[start:] {
		status := "completed"
		extra := ""
		if !r.Success {
			status = "protocol-error"
			if r.Error != "" {
				errMsg := r.Error
				if len(errMsg) > 80 {
					errMsg = errMsg[:80] + "..."
				}
				extra = fmt.Sprintf(" (%s)", errMsg)
			}
		}
		sb.WriteString(fmt.Sprintf("- %s: %s%s\n", r.Name, status, extra))
	}
	return sb.String()
}

// WithToolCaller_ReviewReconsider routes rejected proposals back to their owner.
// The handler may load schemas and record feedback, but must not invoke tools.
func WithToolCaller_ReviewReconsider(handler func(context.Context, *aitool.Tool, aitool.InvokeParams, aitool.InvokeParams) string) ToolCallerOption {
	return func(tc *ToolCaller) { tc.reviewReconsiderHandler = handler }
}

// WithToolCaller_ParamAugment sets an optional callback to merge extra params into the final invoke params
// after generation or preset. Used when infra must inject params (e.g. sample code for validation tools).
func WithToolCaller_ParamAugment(augment func(aitool.InvokeParams) aitool.InvokeParams) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.paramAugment = augment
	}
}

// WithToolCaller_ParamAugmentForTool installs a proposal-aware parameter
// mutator. Review may replace the originally requested tool, so batch callers
// need the actual current tool for each regenerated proposal. The legacy
// WithToolCaller_ParamAugment remains unchanged for scalar callers.
func WithToolCaller_ParamAugmentForTool(
	augment func(*aitool.Tool, aitool.InvokeParams) aitool.InvokeParams,
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.paramAugmentForTool = augment
	}
}

func toolCallerContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// WithToolCaller_IntervalReviewHandler sets the interval review handler for tool execution.
// The handler is called periodically during tool execution to review the progress.
// If the handler returns false, the tool execution will be cancelled.
func WithToolCaller_IntervalReviewHandler(
	handler func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.intervalReviewHandler = handler
	}
}

// WithToolCaller_IntervalReviewDuration sets the interval duration for the review handler.
// Default value is DefaultToolCallIntervalReviewDuration if not set.
func WithToolCaller_IntervalReviewDuration(duration time.Duration) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.intervalReviewDuration = duration
	}
}

func WithToolCaller_CallToolID(callToolId string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.callToolId = callToolId
	}
}

func WithToolCaller_Task(task AITask) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.task = task
	}
}

func WithToolCaller_RuntimeId(runtimeId string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.runtimeId = runtimeId
	}
}

func WithToolCaller_Emitter(e *Emitter) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.emitter = e
	}
}

func WithToolCaller_AICallerConfig(config AICallerConfigIf) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.config = config
	}
}

func WithToolCaller_OnStart(i func(callToolId string)) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.onCallToolStart = i
	}
}

func WithToolCaller_OnEnd(i func(callToolId string)) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.onCallToolEnd = i
	}
}

func NewToolCaller(ctx context.Context, opts ...ToolCallerOption) (*ToolCaller, error) {
	caller := &ToolCaller{
		callToolId:  ksuid.New().String(),
		start:       &sync.Once{},
		done:        &sync.Once{},
		m:           &sync.Mutex{},
		statsSource: StatsSourceToolDirect,
	}
	for _, opt := range opts {
		opt(caller)
	}
	if caller.runtimeId == "" {
		caller.runtimeId = caller.callToolId
	}

	if caller.config == nil || utils.IsNil(caller.config) {
		return nil, fmt.Errorf("config is nil in ToolCaller")
	}

	if utils.IsNil(ctx) {
		caller.ctx, caller.cancel = context.WithCancel(caller.config.GetContext())
	} else {
		caller.ctx, caller.cancel = context.WithCancel(ctx)
	}

	return caller, nil
}

func (t *ToolCaller) SetEmitter(e *Emitter) {
	t.emitter = e
}

// rebindExecutionContext transfers an unused prepared caller from its short
// preparation context to the manager-owned execution context. Task completion
// must not cancel a background tool after it has been admitted.
func (t *ToolCaller) rebindExecutionContext(ctx context.Context) {
	if t == nil || ctx == nil {
		return
	}
	t.m.Lock()
	previousCancel := t.cancel
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.m.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
}

func (t *ToolCaller) cancelContext() {
	if t == nil || t.m == nil {
		return
	}
	t.m.Lock()
	cancel := t.cancel
	t.m.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (t *ToolCaller) GetEmitter() *Emitter {
	return t.emitter
}

// GetIntervalReviewDuration returns the interval review duration.
// If not set, returns DefaultToolCallIntervalReviewDuration.
func (t *ToolCaller) GetIntervalReviewDuration() time.Duration {
	if t.intervalReviewDuration <= 0 {
		return DefaultToolCallIntervalReviewDuration
	}
	return t.intervalReviewDuration
}

// emitStart emits the tool-call START event (the loading card) exactly once,
// guarded by t.start (sync.Once). It only emits the start event; ReviewToolCall
// owns reason emission and liteforge generation for every explicit-call flow.
func (t *ToolCaller) emitStart(tool *aitool.Tool) {
	t.m.Lock()
	defer t.m.Unlock()
	t.startTime = time.Now() // Record start time
	if t.onCallToolStart != nil {
		t.onCallToolStart(t.callToolId)
	}
	// should emit after call tool start callback, this call will bind call tool id for emitter
	t.emitter.EmitToolCallStart(t.callToolId, tool, t.startTime)
}

// emitReason emits a concrete reason string on the tool-call card.
func (t *ToolCaller) emitReason(reason string) {
	if strings.TrimSpace(reason) == "" {
		return
	}
	t.m.Lock()

	t.m.Unlock()
	if t.emitter != nil {
		t.emitter.EmitToolCallReason(t.callToolId, reason)
	}
}

// emitIdentifierReasonFallback keeps batch cards distinguishable when the model
// omitted a child reason and the auxiliary reason generator is skipped or fails.
// The tool's localized verbose name is already shown as the card title.
func (t *ToolCaller) emitIdentifierReasonFallback() {
	reason := strings.Join(strings.Fields(strings.ReplaceAll(t.destinationIdentifier, "_", " ")), " ")
	if reason == "" {
		return
	}
	t.m.Lock()
	if strings.TrimSpace(t.reason) != "" {
		t.m.Unlock()
		return
	}
	t.reason = reason

	t.m.Unlock()
	if t.emitter != nil {
		t.emitter.EmitToolCallReason(t.callToolId, reason)
	}
}

// resetReasonForReview allows an explicit manual edit to generate a fresh reason once.
func (t *ToolCaller) resetReasonForReview() {
	t.m.Lock()
	defer t.m.Unlock()
	t.reason = ""
	t.reasonGen = false
}

// generateReasonIfNeeded generates a reason via the speed-priority liteforge AT
// MOST ONCE per tool call (guarded by t.reasonGen). It is a no-op when a reason
// is already present (preset / action-stashed) or no Config is configured.
func (t *ToolCaller) generateReasonIfNeeded(tool *aitool.Tool, params aitool.InvokeParams) {
	if func() bool {
		t.m.Lock()
		defer t.m.Unlock()
		// already have a concrete reason, or liteforge already ran once
		if strings.TrimSpace(t.reason) != "" || t.reasonGen {
			return true
		}
		t.reasonGen = true
		return false
	}() {
		return
	}
	if t.config == nil || utils.IsNil(t.config) || tool == nil {
		t.emitIdentifierReasonFallback()
		return
	}
	if !t.config.ResolveAuxiliaryTask(CallerLabelToolCallReason).ShouldRun() {
		t.emitIdentifierReasonFallback()
		return
	}
	// Show the per-call intent immediately. A later auxiliary result may
	// replace it, but must never be overwritten by a delayed fallback emit.
	t.emitIdentifierReasonFallback()
	t.config.ScheduleAuxiliaryTask(t.ctx,
		CallerLabelToolCallReason,
		func() string {
			return buildToolCallReasonPrompt(tool, params, t.task, t.destinationIdentifier)
		},
		func(action *Action) {
			reason := strings.TrimSpace(action.GetString("reason"))
			if reason == "" {
				return
			}
			t.m.Lock()
			t.reason = reason
			t.m.Unlock()
			if t.emitter != nil {
				t.emitter.EmitToolCallReason(t.callToolId, reason)
			}
		},
		WithAuxiliaryOutputs(toolCallReasonOutputs...),
	)
}

// DirectlyCallPrepareFunc is the loop-layer callback that prepares the final
// invoke params for a directly_call_tool action AFTER the tool-call card has been
// created. It receives the streaming *Action (whose field getters block until
// each field has streamed in) and the resolved tool. It returns the finalized
// params, whether the proposal requires correction, the resolved tool, and an error.
// A rejected proposal returns reason/retry to the owning loop; it never generates arguments.
type DirectlyCallPrepareFunc func(action *Action, toolName string) (finalParams aitool.InvokeParams, retryRequired bool, tool *aitool.Tool, err error)

// ReviewDirectToolCall handles the directly_call_tool approval phase. It emits
// the loading card first, reads reason and explicit parameters from the
// streaming action, and blocks until the review decision is final. It never
// invokes the tool.
//
// A rejected prepare proposal returns a retry error without generating arguments or executing.
func (t *ToolCaller) ReviewDirectToolCall(nominalTool *aitool.Tool, action *Action, prepare DirectlyCallPrepareFunc) (*ToolCallReviewResult, error) {
	if t.emitter == nil {
		emitter := t.config.GetEmitter()
		if emitter == nil {
			return nil, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	// 1. emit start card first (loading). sync.Once guards the later ReviewToolCall.
	//    Only the start event is emitted here; reason is handled in the single
	//    reason-handling point inside ReviewToolCall, shared by both
	//    explicit tool-call entries.
	t.start.Do(func() { t.emitStart(nominalTool) })

	// 2. stash the action's reason (if any) into t.reason so the unified
	//    reason handler in ReviewToolCall can emit it. We do NOT emit
	//    here — that path is the single source of truth for reason events.
	if action != nil && strings.TrimSpace(t.reason) == "" {
		reason := action.GetString("directly_call_reason")
		if strings.TrimSpace(reason) == "" {
			reason = action.GetString("human_readable_thought")
		}
		if strings.TrimSpace(reason) != "" {
			t.reason = reason
		}
	}

	// 3. prepare params via loop-layer callback (reads action fields, blocks until streamed).
	finalParams := make(aitool.InvokeParams)
	retryRequired := false
	var tool *aitool.Tool
	if prepare != nil {
		var fp aitool.InvokeParams
		var err error
		var fb bool
		t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_PROCESSING_PARAMS)
		fp, fb, tool, err = prepare(action, nominalTool.Name)
		t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_RUNNING)
		if err != nil {
			// close the card on prepare error (ReviewToolCall is never entered)
			t.done.Do(func() {
				t.m.Lock()
				defer t.m.Unlock()
				endTime := time.Now()
				t.emitter.EmitToolCallError(t.callToolId, err, endTime, t.startTime, 0)
				if t.onCallToolEnd != nil {
					t.onCallToolEnd(t.callToolId)
				}
			})
			t.finalizeToolCall(nil)
			return nil, err
		}
		finalParams = fp
		retryRequired = fb
	}

	// 4. run the post-card flow. sync.Once ensures start is not re-emitted.
	if retryRequired {
		err := &ToolCallRetryError{ToolName: nominalTool.Name, Reason: "direct-call parameters were rejected; automatic parameter generation is disabled"}
		t.done.Do(func() {
			t.emitter.EmitToolCallError(t.callToolId, err, time.Now(), t.startTime, 0)
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(t.callToolId)
			}
		})
		t.finalizeToolCall(nil)
		return nil, err
	}
	if finalParams == nil {
		finalParams = make(aitool.InvokeParams)
	}
	if tool == nil {
		tool = nominalTool
	}
	return t.ReviewToolCall(tool, finalParams)
}

// DirectlyCallTool preserves the legacy combined helper.
func (t *ToolCaller) DirectlyCallTool(nominalTool *aitool.Tool, action *Action, prepare DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	review, err := t.ReviewDirectToolCall(nominalTool, action, prepare)
	if err != nil {
		return nil, false, err
	}
	return t.ExecuteReviewedToolCall(review)
}

// EmitReason emits (or updates) the human-readable reason for this tool call on
// the current tool-call card. It may be called multiple times — e.g. with the
// AI's thinking streamed during param generation — so the frontend can update
// the reason shown on the card.
func (t *ToolCaller) EmitReason(reason string) {
	if t.emitter == nil {
		return
	}
	t.emitter.EmitToolCallReason(t.callToolId, reason)
}

// sanitizeIdentifier sanitizes the identifier to be safe for use in file paths
// It converts to lowercase, replaces spaces with underscores, and removes invalid characters
func sanitizeIdentifier(identifier string) string {
	if identifier == "" {
		return ""
	}
	// Convert to lowercase
	result := ""
	for _, r := range identifier {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result += string(r)
		} else if r >= 'A' && r <= 'Z' {
			result += string(r - 'A' + 'a') // Convert to lowercase
		} else if r == ' ' || r == '-' {
			result += "_"
		}
		// Skip other characters
	}
	// Limit length to 30 characters
	if len(result) > 30 {
		result = result[:30]
	}
	if result == "" {
		return ""
	}
	return result
}

// SanitizeTaskName sanitizes a task name for use in directory names.
// Unlike sanitizeIdentifier, this function preserves Unicode characters (including CJK)
// to keep task names human-readable across different languages.
func SanitizeTaskName(name string) string {
	if name == "" {
		return ""
	}
	var result []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r >= 'A' && r <= 'Z' {
				r = r - 'A' + 'a' // Convert ASCII uppercase to lowercase
			}
			result = append(result, r)
		} else if r == ' ' || r == '-' || r == '\t' || r == '_' {
			result = append(result, '_')
		}
		// Skip filesystem-unsafe and other special characters (/, \, :, *, ?, ", <, >, | etc.)
	}
	// Collapse consecutive underscores
	collapsed := make([]rune, 0, len(result))
	for i, r := range result {
		if r == '_' && i > 0 && result[i-1] == '_' {
			continue
		}
		collapsed = append(collapsed, r)
	}
	str := strings.Trim(string(collapsed), "_")
	// Limit rune count (not byte count) to 40 characters for filesystem friendliness
	runes := []rune(str)
	if len(runes) > 40 {
		str = string(runes[:40])
		str = strings.TrimRight(str, "_") // Don't end with underscore after truncation
	}
	return str
}

// CallToolWithExistedParams is kept for callers that still use the combined
// explicit-parameter entry point. New code should call ReviewToolCall and then
// ExecuteReviewedToolCall so approval always completes before execution starts.
func (t *ToolCaller) CallToolWithExistedParams(tool *aitool.Tool, invokeParams aitool.InvokeParams) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	review, err := t.ReviewToolCall(tool, invokeParams)
	if err != nil {
		return nil, false, err
	}
	if review.DirectlyAnswer {
		return nil, true, nil
	}
	return t.ExecuteReviewedToolCall(review)
}

const fastNoopContinueReviewThreshold = 500 * time.Millisecond

// isFastNoopContinueReview recognizes an automatic/default pass-through: the
// endpoint was released within 0.5s and the response contains only the default
// "continue" suggestion. Such a response carries no user-authored information,
// so recording it as user/review only adds noise to the timeline.
func isFastNoopContinueReview(ep *Endpoint, params aitool.InvokeParams, decidedAt time.Time) bool {
	if ep == nil || len(params) != 1 || !strings.EqualFold(strings.TrimSpace(params.GetString("suggestion")), "continue") {
		return false
	}
	createdAtMs := ep.GetCreatedAtMs()
	if createdAtMs <= 0 {
		return false
	}
	duration := decidedAt.Sub(time.UnixMilli(createdAtMs))
	return duration >= 0 && duration <= fastNoopContinueReviewThreshold
}

// markdownCodeFence is 10 backticks used as code fence in markdown to safely nest any content
const markdownCodeFence = "``````````"

// renderParamsAsYAML renders InvokeParams as YAML for human-readable output.
func renderParamsAsYAML(params aitool.InvokeParams) string {
	if len(params) == 0 {
		return "(no parameters)\n"
	}
	yamlBytes, err := yaml.Marshal(map[string]any(params))
	if err != nil {
		log.Warnf("failed to marshal params as YAML: %v, falling back to JSON", err)
		return string(utils.Jsonify(params)) + "\n"
	}
	return string(yamlBytes)
}

func sanitizeFilename(name string) string {
	// Replace invalid filename characters with underscores
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}
	if result == "" {
		return "unknown"
	}
	return result
}

// BuildTaskDirName builds a task directory name with optional semantic identifier.
// Format: task_{index}_{sanitized_name} or task_{index} when name is empty.
// This function uses sanitizeTaskName which preserves Unicode characters (including CJK)
// for human-readable directory names across different languages.
// Example: task_1-1_detect_os_type, task_1-2_扫描目标端口
func BuildTaskDirName(index, name string) string {
	if index == "" {
		index = "0"
	}
	if name == "" {
		return fmt.Sprintf("task_%s", index)
	}
	sanitized := SanitizeTaskName(name)
	if sanitized == "" {
		return fmt.Sprintf("task_%s", index)
	}
	return fmt.Sprintf("task_%s_%s", index, sanitized)
}

// BuildTaskTimelineDiffFilename builds the timeline diff filename with task index and
// optional semantic identifier. The semantic identifier follows the same sanitization
// rules as task directory naming so artifact filenames remain semantically aligned.
// Format: task_{safeIndex}_{sanitized_name}_timeline_diff.txt or
// task_{safeIndex}_timeline_diff.txt when the semantic identifier is empty.
func BuildTaskTimelineDiffFilename(index, semanticIdentifier string) string {
	if index == "" {
		index = "0"
	}
	safeIndex := strings.ReplaceAll(index, "-", "_")
	sanitized := SanitizeTaskName(semanticIdentifier)
	if sanitized == "" {
		return fmt.Sprintf("task_%s_timeline_diff.txt", safeIndex)
	}
	return fmt.Sprintf("task_%s_%s_timeline_diff.txt", safeIndex, sanitized)
}

// BuildTaskResultSummaryFilename builds the result summary filename with task index and
// optional semantic identifier. The semantic identifier follows the same sanitization
// rules as task directory naming so downstream AI can infer task meaning from filenames.
// Format: task_{safeIndex}_{sanitized_name}_result_summary.txt or
// task_{safeIndex}_result_summary.txt when the semantic identifier is empty.
func BuildTaskResultSummaryFilename(index, semanticIdentifier string) string {
	if index == "" {
		index = "0"
	}
	safeIndex := strings.ReplaceAll(index, "-", "_")
	sanitized := SanitizeTaskName(semanticIdentifier)
	if sanitized == "" {
		return fmt.Sprintf("task_%s_result_summary.txt", safeIndex)
	}
	return fmt.Sprintf("task_%s_%s_result_summary.txt", safeIndex, sanitized)
}

func SummaryRank(task AITask, callResult *aitool.ToolResult) string {
	if callResult.ShrinkResult != "" {
		return callResult.ShrinkResult
	}
	if callResult.ShrinkSimilarResult != "" {
		return callResult.ShrinkSimilarResult
	}
	if task.GetSummary() != "" {
		return task.GetSummary()
	}
	return string(utils.Jsonify(callResult.Data))
}
