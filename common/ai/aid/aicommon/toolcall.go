package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"gopkg.in/yaml.v3"
)

// toolParamAITagStartRegexp nonce 段允许 [a-zA-Z0-9_\-\[\]], 既支持历史
// turn nonce (uuid 风格 a-f0-9-), 又支持新引入的占位符字面量 nonce
// "[current-nonce]" (含方括号). 包含 `[` `]` 是正则字符类内字面量,
// 已用 `[A-Za-z0-9_\[\]\-]+` 写法明确表达.
//
// 关键词: toolParamAITagStartRegexp, nonce 占位符, [current-nonce]
var toolParamAITagStartRegexp = regexp.MustCompile(`<\|TOOL_PARAM_([A-Za-z0-9_]+)_([A-Za-z0-9_\[\]\-]+)\|>`)

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
// `<|FACTS_CURRENT_NONCE|>` / `<|FINAL_ANSWER_CURRENT_NONCE|>` /
// `<|GEN_CODE_CURRENT_NONCE|>` 等.
//
// 设计本意是让 AI 把 `CURRENT_NONCE` 替换为本 turn 实际生效的 nonce. 但实测
// 部分模型会把这个占位符当作字面量直接照抄输出, 导致 AITag 解析器只用 turn
// nonce 注册 callback 时根本匹配不到, 内容丢失, verifier 误判为"AI 没提供
// 内容", 触发 5 次重试黑洞甚至致命中断 (实例: output_facts: facts content
// is required).
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

type toolParamAITagBlock struct {
	ParamName string
	Nonce     string
	Content   string
}

func normalizeAITAGBlockContent(content string) string {
	content = strings.TrimPrefix(content, "\r\n")
	content = strings.TrimPrefix(content, "\n")
	content = strings.TrimSuffix(content, "\r\n")
	content = strings.TrimSuffix(content, "\n")
	return content
}

func extractKnownToolParamAITagBlocks(raw string, knownParams []string) []toolParamAITagBlock {
	if raw == "" {
		return nil
	}

	allowedParams := make(map[string]struct{}, len(knownParams))
	for _, paramName := range knownParams {
		allowedParams[paramName] = struct{}{}
	}

	matches := toolParamAITagStartRegexp.FindAllStringSubmatchIndex(raw, -1)
	blocks := make([]toolParamAITagBlock, 0, len(matches))
	for _, match := range matches {
		if len(match) != 6 {
			continue
		}
		paramName := raw[match[2]:match[3]]
		if len(allowedParams) > 0 {
			if _, ok := allowedParams[paramName]; !ok {
				continue
			}
		}
		nonce := raw[match[4]:match[5]]
		contentStart := match[1]
		endTag := fmt.Sprintf("<|TOOL_PARAM_%s_END_%s|>", paramName, nonce)
		endOffset := strings.Index(raw[contentStart:], endTag)
		if endOffset < 0 {
			continue
		}
		contentEnd := contentStart + endOffset
		blocks = append(blocks, toolParamAITagBlock{
			ParamName: paramName,
			Nonce:     nonce,
			Content:   normalizeAITAGBlockContent(raw[contentStart:contentEnd]),
		})
	}
	return blocks
}

func recoverSingleMismatchedAITagParam(invokeParams aitool.InvokeParams, rawAIResponse string, expectedNonce string, knownParams []string, mergedParams map[string]struct{}) (*toolParamAITagBlock, string) {
	if expectedNonce == "" {
		return nil, "expected nonce is empty"
	}
	if len(mergedParams) > 0 {
		return nil, "exact nonce aitag already merged"
	}

	blocks := extractKnownToolParamAITagBlocks(rawAIResponse, knownParams)
	if len(blocks) == 0 {
		return nil, "no known aitag blocks found"
	}

	var mismatched []toolParamAITagBlock
	for _, block := range blocks {
		if block.Nonce == expectedNonce {
			return nil, "found exact nonce aitag block"
		}
		mismatched = append(mismatched, block)
	}

	if len(mismatched) != 1 {
		return nil, fmt.Sprintf("found %d mismatched aitag blocks", len(mismatched))
	}

	candidate := mismatched[0]
	if candidate.Content == "" {
		return nil, "mismatched aitag block is empty"
	}
	if invokeParams.GetString(candidate.ParamName) != "" {
		return nil, fmt.Sprintf("param %s already has a non-empty value", candidate.ParamName)
	}

	invokeParams.Set(candidate.ParamName, candidate.Content)
	return &candidate, ""
}

type ToolCaller struct {
	runtimeId       string
	task            AITask
	config          AICallerConfigIf
	emitter         *Emitter // specific, backup for config.GetEmitter()
	ai              AICaller
	start           *sync.Once
	done            *sync.Once
	callToolId      string
	startTime       time.Time // Track tool call start time
	reason          string    // human-readable reason for this tool call, emitted with the start card
	reasonFinalized bool      // when true, the thinking stream should not overwrite the reason

	// invokeRuntime is an optional AIInvokeRuntime used to (re)generate a
	// human-readable reason via a lightweight (speed-priority) lite forge when
	// no reason was preset (emitStart) or when review changed the tool/params.
	// nil in paths without a runtime (e.g. AiTask.callTool) → graceful no-op.
	invokeRuntime AIInvokeRuntime

	ctx    context.Context
	cancel context.CancelFunc

	generateToolParamsBuilder         func(tool *aitool.Tool, toolName string) (string, error)
	generateToolParamsBuilderWithMeta func(tool *aitool.Tool, toolName string) (*ToolParamsPromptMeta, error)

	m               *sync.Mutex
	onCallToolStart func(callToolId string)
	onCallToolEnd   func(callToolId string)

	intervalReviewHandler  func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error)
	intervalReviewDuration time.Duration // interval duration for review, default is DefaultToolCallIntervalReviewDuration

	reviewWrongToolHandler  func(ctx context.Context, tool *aitool.Tool, newToolName, keyword string) (*aitool.Tool, bool, error)
	reviewWrongParamHandler func(ctx context.Context, tool *aitool.Tool, oldParam aitool.InvokeParams, suggestion string) (aitool.InvokeParams, error)

	paramAugment func(aitool.InvokeParams) aitool.InvokeParams // optional merge before tool execution

	callExpectations           string
	omitResultParamsInTimeline bool
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

// WithToolCaller_OmitResultParamsInTimeline is used when the final params were
// already recorded by an earlier, dedicated timeline item. It only affects the
// timeline rendering hint on the returned ToolResult; invocation and persisted
// tool-call reports still retain the complete params.
func WithToolCaller_OmitResultParamsInTimeline() ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.omitResultParamsInTimeline = true
	}
}

// WithToolCaller_Reason sets the human-readable reason for this tool call. The
// reason is emitted via EmitToolCallReason alongside the start card so the
// frontend can show why the tool is being invoked. EmitToolCallReason may be
// called again later (e.g. with the AI's thinking during param generation) to
// update the reason on the card.
func WithToolCaller_Reason(reason string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reason = reason
		if strings.TrimSpace(reason) != "" {
			tc.reasonFinalized = true
		}
	}
}

// WithToolCaller_InvokeRuntime sets an optional AIInvokeRuntime on the ToolCaller.
// When present, a speed-priority lite forge is used to (re)generate a
// human-readable reason: as a fallback in emitStart when no reason was preset,
// and after a review override (wrong_tool/wrong_params) so the card's reason
// matches the finally-executed tool/params. When absent, both paths no-op and
// behavior is unchanged.
func WithToolCaller_InvokeRuntime(rt AIInvokeRuntime) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.invokeRuntime = rt
	}
}

// generateReasonByLiteForge uses the speed-priority lite forge to generate one
// concise sentence describing WHY the given tool call is needed. It returns an
// empty string (no-op) when no invokeRuntime is configured or generation fails,
// so callers without a runtime keep their original behavior.
func (t *ToolCaller) generateReasonByLiteForge(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams) string {
	if t.invokeRuntime == nil || utils.IsNil(t.invokeRuntime) || tool == nil {
		return ""
	}
	if utils.IsNil(ctx) {
		ctx = t.ctx
	}
	prompt := buildToolCallReasonPrompt(tool, params, t.task)
	action, err := t.invokeRuntime.InvokeSpeedPriorityLiteForge(
		ctx, "tool-call-reason", prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("reason",
				aitool.WithParam_Description("A terse phrase (under 15 words) stating WHAT this tool call does right now. No transitions or prior-step summaries. Match the language of the user input."),
				aitool.WithParam_MaxLength(30),
				aitool.WithParam_Required(true)),
		},
	)
	if err != nil || utils.IsNil(action) {
		log.Debugf("generate tool-call reason via liteforge failed: %v", err)
		return ""
	}
	return strings.TrimSpace(action.GetString("reason"))
}

// buildToolCallReasonPrompt builds the lite-forge prompt asking for a concise
// reason for a tool call, given the tool, its (possibly empty) params, and the
// owning task's user input / name for intent context. It also includes a brief
// summary of recent tool-call results so the generated reason can reference the
// specific progress that motivates this call.
func buildToolCallReasonPrompt(tool *aitool.Tool, params aitool.InvokeParams, task AITask) string {
	var sb strings.Builder
	sb.WriteString("Generate a terse reason (under 15 words) stating WHAT this tool call does right now. " +
		"Focus on the concrete current action, not on prior steps or transitions. " +
		"Bad: '端口扫描完成，接下来需要执行简单爬虫收集页面' / 'previous scan found open ports, now crawling'. " +
		"Good: '爬取目标站点页面与API端点' / 'crawl site pages and API endpoints'.\n")
	sb.WriteString(fmt.Sprintf("Tool: %s\n", tool.Name))
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		sb.WriteString(fmt.Sprintf("Tool description: %s\n", desc))
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
// recent N tool-call results from the task, oldest first. Each entry is a
// one-line bullet: "- tool_name: success/failed (brief error if any)".
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
		status := "success"
		extra := ""
		if !r.Success {
			status = "failed"
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

func WithToolCaller_ReviewWrongTool(
	handler func(ctx context.Context, tool *aitool.Tool, newToolName, keyword string) (*aitool.Tool, bool, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reviewWrongToolHandler = handler
	}
}

func WithToolCaller_ReviewWrongParam(
	handler func(ctx context.Context, tool *aitool.Tool, oldParam aitool.InvokeParams, suggestion string) (aitool.InvokeParams, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reviewWrongParamHandler = handler
	}
}

// WithToolCaller_ParamAugment sets an optional callback to merge extra params into the final invoke params
// after generation or preset. Used when infra must inject params (e.g. sample code for validation tools).
func WithToolCaller_ParamAugment(augment func(aitool.InvokeParams) aitool.InvokeParams) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.paramAugment = augment
	}
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

func WithToolCaller_AICaller(ai AICaller) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.ai = ai
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

func WithToolCaller_GenerateToolParamsBuilder(
	builder func(tool *aitool.Tool, toolName string) (string, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.generateToolParamsBuilder = builder
	}
}

// ToolParamsPromptMeta contains the generated prompt and metadata for AITAG parsing
type ToolParamsPromptMeta struct {
	Prompt     string
	Nonce      string
	ParamNames []string
	Identifier string // destination identifier extracted from AI response, e.g. "query_large_file", "find_process"
}

// WithToolCaller_GenerateToolParamsBuilderWithMeta sets a builder that returns prompt with metadata for AITAG support
func WithToolCaller_GenerateToolParamsBuilderWithMeta(
	builder func(tool *aitool.Tool, toolName string) (*ToolParamsPromptMeta, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.generateToolParamsBuilderWithMeta = builder
	}
}

func NewToolCaller(ctx context.Context, opts ...ToolCallerOption) (*ToolCaller, error) {
	caller := &ToolCaller{
		callToolId: ksuid.New().String(),
		start:      &sync.Once{},
		done:       &sync.Once{},
		m:          &sync.Mutex{},
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

	if caller.ai == nil || utils.IsNil(caller.ai) {
		return nil, fmt.Errorf("ai caller is nil in ToolCaller")
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

func (t *ToolCaller) GetParamGeneratingPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if t.generateToolParamsBuilder == nil {
		return "", fmt.Errorf("generateToolParamsBuilder is nil")
	}

	return t.generateToolParamsBuilder(tool, toolName)
}

func (t *ToolCaller) CallTool(tool *aitool.Tool) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	return t.CallToolWithExistedParams(tool, false, make(aitool.InvokeParams))
}

// emitStart records the start time, binds the emitter via onCallToolStart, and
// emits EmitToolCallStart (the tool-call card) — and EmitToolCallReason when a
// reason was preset via WithToolCaller_Reason. It is invoked through t.start
// (a sync.Once) so the card is emitted exactly once even when the caller triggers
// start early (e.g. DirectlyCallTool) and CallToolWithExistedParams runs later.
func (t *ToolCaller) emitStart(tool *aitool.Tool) {
	t.m.Lock()
	defer t.m.Unlock()
	t.startTime = time.Now() // Record start time
	if t.onCallToolStart != nil {
		t.onCallToolStart(t.callToolId)
	}
	// should emit after call tool start callback, this call will bind call tool id for emitter
	t.emitter.EmitToolCallStart(t.callToolId, tool, t.startTime)
	if t.reason != "" {
		t.emitter.EmitToolCallReason(t.callToolId, t.reason)
	} else if t.invokeRuntime != nil && !utils.IsNil(t.invokeRuntime) {
		// No preset reason: ask the lightweight model for a one-line reason so
		// the card isn't blank. Run async so emitStart (and the require path's
		// param generation right after) isn't blocked on the model call. The
		// goroutine does not take t.m, so the lock held here is safe.
		toolRef := tool
		go func() {
			defer func() { _ = recover() }()
			if reason := t.generateReasonByLiteForge(t.ctx, toolRef, nil); reason != "" {
				t.emitter.EmitToolCallReason(t.callToolId, reason)
				t.reasonFinalized = true
			}
		}()
	}
}

// DirectlyCallPrepareFunc is the loop-layer callback that prepares the final
// invoke params for a directly_call_tool action AFTER the tool-call card has been
// created. It receives the streaming *Action (whose field getters block until
// each field has streamed in) and the resolved tool. It returns the finalized
// params, whether to fall back to the AI param-generation (require) path (e.g.
// on schema-validation failure), or an error.
type DirectlyCallPrepareFunc func(action *Action, toolName string) (finalParams aitool.InvokeParams, fallbackToRequire bool, tool *aitool.Tool, err error)

// DirectlyCallTool handles the "card already created" flow for a
// directly_call_tool action: it emits the tool-call card (loading) FIRST, then
// reads the reason and params from the streaming action (blocking per-field until
// they arrive), then invokes the tool. Because Action is an async streaming
// structure, the card appears immediately and reason/params stream in afterwards
// — card creation is never blocked on reason/params parsing.
//
// On fallbackToRequire the same card is reused and execution switches to the AI
// param-generation path (CallToolWithExistedParams with skipRequire=false).
func (t *ToolCaller) DirectlyCallTool(nominalTool *aitool.Tool, action *Action, prepare DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	if t.emitter == nil {
		emitter := t.config.GetEmitter()
		if emitter == nil {
			return nil, false, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	// 1. emit start card first (loading). sync.Once guards the later CallToolWithExistedParams.
	t.start.Do(func() { t.emitStart(nominalTool) })

	// 2. read reason from the streaming action (blocks until reason streams in),
	//    fallback to human_readable_thought, emit as a separate event so the card
	//    is never blocked on reason parsing.
	if action != nil {
		reason := action.GetString("directly_call_reason")
		if strings.TrimSpace(reason) == "" {
			reason = action.GetString("human_readable_thought")
		}
		if strings.TrimSpace(reason) != "" {
			t.emitter.EmitToolCallReason(t.callToolId, reason)
			t.reasonFinalized = true
		}
	}

	// 3. prepare params via loop-layer callback (reads action fields, blocks until streamed).
	finalParams := make(aitool.InvokeParams)
	fallback := false
	var tool *aitool.Tool
	if prepare != nil {
		var fp aitool.InvokeParams
		var err error
		var fb bool
		t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_PROCESSING_PARAMS)
		fp, fb, tool, err = prepare(action, nominalTool.Name)
		t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_RUNNING)
		if err != nil {
			// close the card on prepare error (we never reach CallToolWithExistedParams' defers)
			t.done.Do(func() {
				t.m.Lock()
				defer t.m.Unlock()
				endTime := time.Now()
				t.emitter.EmitToolCallError(t.callToolId, err, endTime, t.startTime, 0)
				if t.onCallToolEnd != nil {
					t.onCallToolEnd(t.callToolId)
				}
			})
			return nil, false, err
		}
		finalParams = fp
		fallback = fb
	}

	// 4. run the post-card flow. sync.Once ensures start is not re-emitted.
	if fallback {
		// The direct params were rejected and the require path generated a new set,
		// so the final result remains the only authoritative timeline copy.
		t.omitResultParamsInTimeline = false
		return t.CallToolWithExistedParams(tool, false, nil)
	}
	if finalParams == nil {
		finalParams = make(aitool.InvokeParams)
	}
	return t.CallToolWithExistedParams(tool, true, finalParams)
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

// GenerateParamsResult contains the result of generateParams including params and identifier
type GenerateParamsResult struct {
	Params           aitool.InvokeParams
	Identifier       string        // destination identifier, e.g. "query_large_file", "find_process"
	Duration         time.Duration // time spent generating params via AI
	RawAIResponse    string        // raw AI stream output for param generation
	CallExpectations string        // AI-generated expectations for this tool call (timing, success criteria, etc.)
}

func (t *ToolCaller) generateParams(tool *aitool.Tool, handleError func(i any)) (*GenerateParamsResult, error) {
	emitter := t.emitter

	t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_PROCESSING_PARAMS)
	defer t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_RUNNING)

	// Try to use the new builder with metadata first (for AITAG support)
	var paramsPrompt string
	var promptMeta *ToolParamsPromptMeta
	var err error

	if t.generateToolParamsBuilderWithMeta != nil {
		promptMeta, err = t.generateToolParamsBuilderWithMeta(tool, tool.Name)
		if err != nil {
			emitter.EmitError("error generate tool[%v] params with meta in task: %v, err: %v", tool.Name, t.task.GetName(), err)
			handleError(fmt.Sprintf("error generate tool[%v] params with meta in task: %v", tool.Name, t.task.GetName()))
			return nil, err
		}
		paramsPrompt = promptMeta.Prompt
	} else {
		paramsPrompt, err = t.GetParamGeneratingPrompt(tool, tool.Name)
		if err != nil {
			emitter.EmitError("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
			handleError(fmt.Sprintf("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName()))
			return nil, err
		}
	}

	invokeParams := aitool.InvokeParams{}
	var identifier string
	var callExpectations string
	var paramDuration time.Duration
	var rawAIResponse string
	err = CallAITransaction(t.config, paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
		boundEmitter := rsp.BindEmitter(emitter)
		// Stream the model's thinking as the tool-call reason (updating the card)
		// during param generation — but only when no specific reason has already
		// been finalized (via preset, LiteForge, or directly_call_reason). This
		// prevents raw thinking fragments from overwriting a contextualized reason.
		rsp.SetOnReasonChunk(func(b []byte) {
			if len(b) == 0 || t.reasonFinalized {
				return
			}
			boundEmitter.EmitToolCallReason(t.callToolId, string(b))
		})
		pr, pw := utils.NewPipe()

		stream := rsp.GetOutputStreamReader("call-tools", true, emitter)

		var response bytes.Buffer
		stream = io.TeeReader(stream, &response)

		var paramNames []string

		// Build action maker options for AITAG support
		var actionOpts []ActionMakerOption
		if promptMeta != nil && promptMeta.Nonce != "" && len(promptMeta.ParamNames) > 0 {
			actionOpts = append(actionOpts, WithActionNonce(promptMeta.Nonce))
			// Register AITAG handlers for each parameter
			for _, paramName := range promptMeta.ParamNames {
				paramNames = append(paramNames, paramName)
				tagName := fmt.Sprintf("TOOL_PARAM_%s", paramName)
				// Map the tag to a special key in params that we'll merge later
				tagParamName := fmt.Sprintf("__aitag__%s", paramName)
				paramNames = append(paramNames, tagParamName)
				actionOpts = append(actionOpts, WithActionTagToKey(tagName, tagParamName))
			}
			log.Debugf("registered AITAG handlers for tool[%s] params: %v with nonce: %s", tool.Name, promptMeta.ParamNames, promptMeta.Nonce)
		}

		event, err := boundEmitter.EmitDefaultSystemStreamEvent("generating-tool-call-params", pr, t.task.GetIndex())
		if err != nil {
			boundEmitter.EmitError("error emit default stream event for tool[%s] params: %v", tool.Name, err)
		}
		_ = event

		pw.WriteString("[开始处理参数] → ")

		start := time.Now()
		actionOpts = append(
			actionOpts,
			WithActionOnReaderFinished(func() {
				cost := time.Since(start)
				paramDuration = cost
				rawAIResponse = response.String()
				pw.WriteString(" [done] 耗时(Cost): " + fmt.Sprintf("%.2f", cost.Seconds()) + "s")
				boundEmitter.EmitTextReferenceMaterial(event.GetContentJSONPath(`$.event_writer_id`), rawAIResponse)
				pw.Close()
			}),
			WithActionFieldStreamHandler(paramNames, func(key string, r io.Reader) {
				if !strings.HasPrefix(key, "__aitag__") {
					pw.WriteString(key + ": ")
					io.Copy(pw, r)
				} else {
					actKey := strings.TrimPrefix(key, "__aitag__")
					pw.WriteString(actKey + "(BLOCK)")
					io.Copy(pw, r)
				}
				pw.WriteString(" → ")
			}),
			WithActionFieldStreamHandler([]string{
				"call_expectations",
			}, func(key string, r io.Reader) {
				peekedR := utils.NewPeekableReader(r)
				_, err := peekedR.Peek(1)
				if err != nil {
					return
				}
				pw.WriteString(" [note] -> ")
				io.Copy(pw, peekedR)
			}),
		)

		callToolAction, err := ExtractValidActionFromStream(t.config.GetContext(), stream, "call-tool", actionOpts...)
		if err != nil {
			boundEmitter.EmitError("error extract tool params: %v", err)
			return utils.Errorf("error extracting action params: %v", err)
		}

		// Extract identifier from action (destination identifier for this tool call)
		identifier = sanitizeIdentifier(callToolAction.GetString("identifier"))
		if identifier != "" {
			log.Debugf("extracted identifier[%s] for tool[%s]", identifier, tool.Name)
		}

		callExpectations = callToolAction.GetString("call_expectations")
		if callExpectations != "" {
			log.Debugf("extracted call_expectations for tool[%s]: %s", tool.Name, callExpectations)
		}

		// First, get params from JSON
		for k, v := range callToolAction.GetInvokeParams("params") {
			invokeParams.Set(k, v)
		}

		// Then, merge AITAG params (they take precedence over JSON params)
		mergedAITagParams := make(map[string]struct{})
		if promptMeta != nil && len(promptMeta.ParamNames) > 0 {
			for _, paramName := range MergeActionAITagParams(callToolAction, invokeParams, promptMeta.ParamNames) {
				mergedAITagParams[paramName] = struct{}{}
				log.Debugf("merged AITAG param[%s] for tool[%s]", paramName, tool.Name)
			}

			rawAIResponse = response.String()
			blocks := extractKnownToolParamAITagBlocks(rawAIResponse, promptMeta.ParamNames)
			var mismatched []toolParamAITagBlock
			for _, block := range blocks {
				if block.Nonce != promptMeta.Nonce {
					mismatched = append(mismatched, block)
				}
			}
			if len(mismatched) > 0 {
				parts := make([]string, 0, len(mismatched))
				for _, block := range mismatched {
					parts = append(parts, fmt.Sprintf("%s:%s", block.ParamName, block.Nonce))
				}
				message := fmt.Sprintf("tool[%s] generated mismatched AITAG nonce, expected=%s observed=%s", tool.Name, promptMeta.Nonce, strings.Join(parts, ", "))
				log.Warn(message)
				emitter.EmitWarning(message)
			}

			if recovered, reason := recoverSingleMismatchedAITagParam(invokeParams, rawAIResponse, promptMeta.Nonce, promptMeta.ParamNames, mergedAITagParams); recovered != nil {
				message := fmt.Sprintf("tool[%s] recovered single AITAG param[%s] from mismatched nonce[%s], expected nonce[%s]", tool.Name, recovered.ParamName, recovered.Nonce, promptMeta.Nonce)
				log.Warn(message)
				emitter.EmitWarning(message)
			} else if len(mismatched) > 0 {
				log.Debugf("tool[%s] skipped AITAG nonce fallback: %s", tool.Name, reason)
			}
		}

		return nil
	}, WithAIRequest_CallerLabel("toolcall-params"))
	if err != nil {
		emitter.EmitError("error calling AI for tool[%v] params: %v", tool.Name, err)
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, err
	}
	return &GenerateParamsResult{
		Params:           invokeParams,
		Identifier:       identifier,
		Duration:         paramDuration,
		RawAIResponse:    rawAIResponse,
		CallExpectations: callExpectations,
	}, nil
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

// CallToolWithExistedParams is the normal require/preset tool-call flow (param
// generation/review/invoke/done). DirectlyCallTool delegates here for the actual
// invoke after emitting the card and running the prepare callback.
func (t *ToolCaller) CallToolWithExistedParams(tool *aitool.Tool, presetParams bool, presetInvokeParams aitool.InvokeParams) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	if t.emitter == nil {
		emitter := t.config.GetEmitter()
		if emitter == nil {
			return nil, false, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	callToolId := t.callToolId
	defer func() {
		if useful, err := yakit.UsefulRuntimeId(consts.GetGormProjectDatabase(), callToolId); err == nil && useful {
			t.config.AppendRelatedRuntimeID(callToolId)
		}
	}()

	toolResult := &aitool.ToolResult{}
	defer t.emitter.EmitToolCallSummary(t.callToolId, SummaryRank(t.task, toolResult))

	t.start.Do(func() { t.emitStart(tool) })

	t.emitter.EmitInfo("start to generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
	// pluginInvokeStartTime: 纯插件执行起点（从真正进入 t.invoke 前开始计时）。
	// 该时间不会包含 AI 生成参数、人工 review 等前置阶段。
	var pluginInvokeStartTime time.Time
	// pluginInvokeDuration: 纯插件执行耗时缓存。
	// 由 t.invoke 返回后写入，done/cancel/error 的 once 回调仅读取；访问时统一受 t.m 保护。
	var pluginInvokeDuration time.Duration
	getPurePluginDuration := func(endTime time.Time) time.Duration {
		if pluginInvokeStartTime.IsZero() {
			return 0
		}
		if pluginInvokeDuration > 0 {
			return pluginInvokeDuration
		}
		if endTime.IsZero() {
			endTime = time.Now()
		}
		if endTime.Before(pluginInvokeStartTime) {
			return 0
		}
		return endTime.Sub(pluginInvokeStartTime)
	}

	handleDone := func() {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now() // Record end time
			t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_DONE)
			t.emitter.EmitToolCallDone(callToolId, endTime, t.startTime, getPurePluginDuration(endTime))
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleUserCancel := func(reason any) {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now() // Record end time
			t.emitter.EmitToolCallStatus(t.callToolId, fmt.Sprintf("%s by reason: %v", schema.TOOL_CALL_STATUS_CANCELLED, reason))
			t.emitter.EmitToolCallUserCancel(callToolId, endTime, t.startTime, getPurePluginDuration(endTime))

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleError := func(err any) {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now() // Record end time
			t.emitter.EmitToolCallError(callToolId, err, endTime, t.startTime, getPurePluginDuration(endTime))

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	var (
		_ = handleDone
		_ = handleUserCancel
		_ = handleError
	)

	defer handleDone()

	// generate params
	var invokeParams = make(aitool.InvokeParams)
	var destinationIdentifier string // identifier describing the purpose of this tool call
	var paramGenDuration time.Duration
	var rawAIParamResponse string
	if presetParams {
		invokeParams = presetInvokeParams
		if id, ok := invokeParams[ReservedKeyIdentifier]; ok {
			destinationIdentifier = sanitizeIdentifier(utils.InterfaceToString(id))
			delete(invokeParams, ReservedKeyIdentifier)
		}
		if destinationIdentifier == "" {
			if id, ok := invokeParams["identifier"]; ok {
				destinationIdentifier = sanitizeIdentifier(utils.InterfaceToString(id))
				delete(invokeParams, "identifier")
			}
		}
		if ce, ok := invokeParams[ReservedKeyCallExpectations]; ok {
			t.callExpectations = utils.InterfaceToString(ce)
			delete(invokeParams, ReservedKeyCallExpectations)
		}
		if t.callExpectations == "" {
			if ce, ok := invokeParams["call_expectations"]; ok {
				t.callExpectations = utils.InterfaceToString(ce)
				delete(invokeParams, "call_expectations")
			}
		}
		if destinationIdentifier != "" {
			t.emitter.EmitInfo("tool[%v] destination identifier: %v", tool.Name, destinationIdentifier)
		}
		t.emitter.EmitInfo("use preset params for tool[%v]: %v", tool.Name, invokeParams)
	} else {
		generateResult, err := t.generateParams(tool, handleError)
		if err != nil {
			return nil, false, utils.Errorf("error generating params for tool[%v]: %v", tool.Name, err)
		}
		invokeParams = generateResult.Params
		destinationIdentifier = generateResult.Identifier
		paramGenDuration = generateResult.Duration
		rawAIParamResponse = generateResult.RawAIResponse
		t.callExpectations = generateResult.CallExpectations
		if destinationIdentifier != "" {
			t.emitter.EmitInfo("tool[%v] destination identifier: %v", tool.Name, destinationIdentifier)
		}
	}
	if utils.IsNil(invokeParams) {
		invokeParams = make(aitool.InvokeParams)
	}
	if t.paramAugment != nil {
		invokeParams = t.paramAugment(invokeParams)
	}

	t.emitter.EmitInfo("start to invoke callback function for tool:%v", tool.Name)
	epm := t.config.GetEndpointManager()
	config := t.config
	// DANGER: NoNeedUserReview
	if tool.NoNeedUserReview {
		t.emitter.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", tool.Name)
	} else {
		t.emitter.EmitInfo("start to require review for tool use: %v", tool.Name)
		ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		reqs := map[string]any{
			"id":               ep.GetId(),
			"selectors":        ToolUseReviewSuggestions,
			"tool":             tool.Name,
			"tool_description": tool.Description,
			"params":           invokeParams,
			"reason":           t.reason,
		}
		ep.SetReviewMaterials(reqs)
		err := t.config.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request review to db for tool use failed: %v", err)
		}
		t.emitter.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)

		// 审批前快照原始提议参数 (original_value), 供价值评估比对是否被改动.
		originalReviewParams := make(aitool.InvokeParams, len(invokeParams))
		for k, v := range invokeParams {
			originalReviewParams[k] = v
		}
		reviewQuestion := fmt.Sprintf("determite tool[%v]'s params is proper? what should I do?", tool.Name)

		// wait for agree
		config.DoWaitAgree(t.ctx, ep)
		reviewDecidedAt := time.Now()
		params := ep.GetParams()
		t.emitter.EmitInteractiveRelease(ep.GetId(), params)
		config.CallAfterInteractiveEventReleased(ep.GetId(), params)
		if !isFastNoopContinueReview(ep, params, reviewDecidedAt) {
			config.CallAfterReview(
				ep.GetSeq(),
				reviewQuestion,
				params,
			)
		}
		if params == nil {
			// 价值评估: 用户取消工具审批 (空响应释放) 是高价值的人工否决信号, 不能漏采.
			if cfg, ok := config.(*Config); ok {
				cfg.SubmitToolReviewValueFeedback(ep, reviewQuestion, originalReviewParams, nil)
			}
			t.emitter.EmitError("tool use [%v] review params is nil, user may cancel the review", tool.Name)
			handleError(fmt.Sprintf("tool use [%v] review params is nil, user may cancel the review", tool.Name))
			return nil, false, fmt.Errorf("tool use [%v] review params is nil", tool.Name)
		}
		var overrideResult *aitool.ToolResult
		var next HandleToolUseNext
		tool, invokeParams, overrideResult, next, err = t.review(
			tool, invokeParams, params, handleUserCancel,
		)
		if err != nil {
			t.emitter.EmitError("error handling tool use review: %v", err)
			handleError(fmt.Sprintf("error handling tool use review: %v", err))
			return nil, false, err
		}

		// 价值评估 (review_decision): 记录审批事实 (original/final 参数 + 运行时来源),
		// invokeParams 此时已是 review 应用后的最终参数. 非阻塞, 绝不影响主流程.
		if cfg, ok := config.(*Config); ok {
			cfg.SubmitToolReviewValueFeedback(ep, reviewQuestion, originalReviewParams, invokeParams)
		}

		switch next {
		case HandleToolUseNext_Override:
			return overrideResult, false, nil
		case HandleToolUseNext_DirectlyAnswer:
			return nil, true, nil
		case HandleToolUseNext_Default:
		default:
			return nil, false, utils.Errorf("unknown handle tool use next action: %v", next)
		}
	}

	stdoutReader, stdoutWriter := utils.NewPipe()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter := utils.NewPipe()
	defer stderrWriter.Close()

	// Create buffers to capture stdout and stderr for interval review and file saving.
	stdoutBuffer := &toolOutputBuffer{}
	stderrBuffer := &toolOutputBuffer{}

	artifactBundle := t.newToolCallArtifactBundle(tool, callToolId, destinationIdentifier)

	// Use MultiWriter to write to both the pipe (for streaming) and the buffers (for file saving)
	stdoutUIWriter := newBoundedToolUIWriter(stdoutWriter)
	stderrUIWriter := newBoundedToolUIWriter(stderrWriter)
	stdoutMultiWriter := io.MultiWriter(artifactBundle.Writer(artifactStdout), stdoutUIWriter, stdoutBuffer)
	stderrMultiWriter := io.MultiWriter(artifactBundle.Writer(artifactStderr), stderrUIWriter, stderrBuffer)

	waitToolStdFlush := t.emitter.EmitToolCallStd(tool.Name, stdoutReader, stderrReader, t.task.GetIndex())

	// Refresh MCP tools from the manager immediately before invoke. Parameter generation
	// may take several seconds; background loadMCPServers can replace stubs with live
	// tools while the AI is still drafting params.
	if buildinaitools.IsMCPToolName(tool.Name) {
		if mgr := t.config.GetAiToolManager(); mgr != nil {
			waitCtx := t.ctx
			if waitCtx == nil {
				waitCtx = t.config.GetContext()
			}
			liveTool, waitErr := buildinaitools.WaitForMCPLiveTool(
				waitCtx, mgr, tool.Name,
				buildinaitools.MCPToolInitWaitTimeout,
				buildinaitools.MCPToolInitPollInterval,
				func(elapsed time.Duration) {
					t.emitter.EmitInfo(
						"MCP tool %q still connecting (elapsed %v), waiting for remote server before invoke...",
						tool.Name, elapsed.Round(time.Second),
					)
				},
			)
			if waitErr != nil {
				return nil, false, waitErr
			}
			if liveTool != nil {
				tool = liveTool
			}
		}
	}

	t.emitter.EmitInfo("start to invoke tool: %v", tool.Name)
	t.emitter.EmitToolCallStatus(callToolId, schema.TOOL_CALL_STATUS_RUNNING)
	t.m.Lock()
	// Measure pure plugin execution time from real invoke start to invoke return.
	pluginInvokeStartTime = time.Now()
	pluginInvokeDuration = 0
	t.m.Unlock()

	t.emitter.EmitToolCallParam(callToolId, invokeParams)
	toolResult, err = t.invoke(
		tool, invokeParams, handleUserCancel, handleError,
		stdoutMultiWriter, stderrMultiWriter,
		stdoutBuffer, stderrBuffer,
		func(result *aitool.ToolResult) error {
			return artifactBundle.finalize(t, tool, callToolId, destinationIdentifier, invokeParams, result, paramGenDuration, rawAIParamResponse)
		},
	)
	t.m.Lock()
	if !pluginInvokeStartTime.IsZero() && pluginInvokeDuration <= 0 {
		pluginInvokeDuration = time.Since(pluginInvokeStartTime)
	}
	t.m.Unlock()
	if err != nil {
		if toolResult == nil {
			toolResult = &aitool.ToolResult{
				Param:       invokeParams,
				Name:        tool.Name,
				Description: tool.Description,
				ToolCallID:  callToolId,
			}
		}
		if toolResult.Error == "" {
			toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", tool.Name, err)
		}
		toolResult.Success = false
		enforceCanonicalToolResultLimit(toolResult)
	}
	if toolResult != nil {
		toolResult.OmitParamsInTimeline = t.omitResultParamsInTimeline
	}

	// Close pipe writers to signal end of tool output. This triggers ThrottledCopy's
	// final flush of any remaining buffered data. The deferred Close calls are still
	// safe (bufpipe Close is idempotent).
	if finishErr := stdoutUIWriter.Finish(); finishErr != nil {
		log.Warnf("failed to flush bounded stdout tail: %v", finishErr)
	}
	if finishErr := stderrUIWriter.Finish(); finishErr != nil {
		log.Warnf("failed to flush bounded stderr tail: %v", finishErr)
	}
	stdoutWriter.Close()
	stderrWriter.Close()

	// Wait for ThrottledCopy goroutines to finish flushing all remaining buffered data.
	// This ensures all tool stdout/stderr stream events have been sent via gRPC before
	// we emit the tool result event, preserving correct event ordering.
	waitToolStdFlush()

	t.emitter.EmitInfo("start to generate and feedback tool[%v] result in task: %#v", tool.Name, t.task.GetName())
	if toolResult != nil && toolResult.Data != nil {
		t.emitter.EmitToolCallResult(callToolId, toolResult.Data)
	}

	NotifySessionSnapshotToolCall(t.config, toolResult)

	return toolResult, false, nil
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
