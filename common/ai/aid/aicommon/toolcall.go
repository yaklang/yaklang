package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gopkg.in/yaml.v3"
)

type ToolCaller struct {
	runtimeId  string
	task       AITask
	config     AICallerConfigIf
	emitter    *Emitter // specific, backup for config.GetEmitter()
	ai         AICaller
	start      *sync.Once
	done       *sync.Once
	callToolId string
	startTime  time.Time // Track tool call start time

	ctx    context.Context
	cancel context.CancelFunc

	generateToolParamsBuilder         func(tool *aitool.Tool, toolName string) (string, error)
	generateToolParamsBuilderWithMeta func(tool *aitool.Tool, toolName string) (*ToolParamsPromptMeta, error)

	m               *sync.Mutex
	onCallToolStart func(callToolId string)
	onCallToolEnd   func(callToolId string)

	intervalReviewHandler  func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error)
	intervalReviewDuration time.Duration // interval duration for review, default is 20 seconds

	reviewWrongToolHandler  func(ctx context.Context, tool *aitool.Tool, newToolName, keyword string) (*aitool.Tool, bool, error)
	reviewWrongParamHandler func(ctx context.Context, tool *aitool.Tool, oldParam aitool.InvokeParams, suggestion string) (aitool.InvokeParams, error)
}

type ToolCallerOption func(tc *ToolCaller)

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

// WithToolCaller_IntervalReviewHandler sets the interval review handler for tool execution.
// The handler is called periodically during tool execution to review the progress.
// If the handler returns false, the tool execution will be cancelled.
func WithToolCaller_IntervalReviewHandler(
	handler func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.intervalReviewHandler = handler
	}
}

// WithToolCaller_IntervalReviewDuration sets the interval duration for the review handler.
// Default value is 20 seconds if not set.
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
// If not set, returns the default value of 20 seconds.
func (t *ToolCaller) GetIntervalReviewDuration() time.Duration {
	if t.intervalReviewDuration <= 0 {
		return time.Second * 20
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

// GenerateParamsResult contains the result of generateParams including params and identifier
type GenerateParamsResult struct {
	Params        aitool.InvokeParams
	Identifier    string        // destination identifier, e.g. "query_large_file", "find_process"
	Duration      time.Duration // time spent generating params via AI
	RawAIResponse string        // raw AI stream output for param generation
}

func (t *ToolCaller) generateParams(tool *aitool.Tool, handleError func(i any)) (*GenerateParamsResult, error) {
	emitter := t.emitter

	// Try to use the new builder with metadata first (for AITAG support)
	var paramsPrompt string
	var promptMeta *ToolParamsPromptMeta
	var err error

	if t.generateToolParamsBuilderWithMeta != nil {
		promptMeta, err = t.generateToolParamsBuilderWithMeta(tool, tool.Name)
		if err != nil {
			emitter.EmitError("error generate tool[%v] params with meta in task: %v", tool.Name, t.task.GetName())
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
	var paramDuration time.Duration
	var rawAIResponse string
	err = CallAITransaction(t.config, paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
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

		event, err := emitter.EmitDefaultStreamEvent("generating-tool-call-params", pr, t.task.GetIndex())
		if err != nil {
			emitter.EmitError("error emit default stream event for tool[%s] params: %v", tool.Name, err)
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
				emitter.EmitTextReferenceMaterial(event.GetContentJSONPath(`$.event_writer_id`), rawAIResponse)
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
		)

		callToolAction, err := ExtractValidActionFromStream(t.config.GetContext(), stream, "call-tool", actionOpts...)
		if err != nil {
			emitter.EmitError("error extract tool params: %v", err)
			return utils.Errorf("error extracting action params: %v", err)
		}

		// Extract identifier from action (destination identifier for this tool call)
		identifier = sanitizeIdentifier(callToolAction.GetString("identifier"))
		if identifier != "" {
			log.Debugf("extracted identifier[%s] for tool[%s]", identifier, tool.Name)
		}

		// First, get params from JSON
		for k, v := range callToolAction.GetInvokeParams("params") {
			invokeParams.Set(k, v)
		}

		// Then, merge AITAG params (they take precedence over JSON params)
		if promptMeta != nil && len(promptMeta.ParamNames) > 0 {
			for _, paramName := range promptMeta.ParamNames {
				aitagKey := fmt.Sprintf("__aitag__%s", paramName)
				if aitagValue := callToolAction.GetString(aitagKey); aitagValue != "" {
					invokeParams.Set(paramName, aitagValue)
					log.Debugf("merged AITAG param[%s] for tool[%s]", paramName, tool.Name)
				}
			}
		}

		return nil
	})
	if err != nil {
		emitter.EmitError("error calling AI for tool[%v] params: %v", tool.Name, err)
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, err
	}
	return &GenerateParamsResult{
		Params:        invokeParams,
		Identifier:    identifier,
		Duration:      paramDuration,
		RawAIResponse: rawAIResponse,
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

func (t *ToolCaller) CallToolWithExistedParams(tool *aitool.Tool, presetParams bool, presetInvokeParams aitool.InvokeParams) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	if t.emitter == nil {
		emitter := t.config.GetEmitter()
		if emitter == nil {
			return nil, false, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	callToolId := t.callToolId

	toolResult := &aitool.ToolResult{}
	defer t.emitter.EmitToolCallSummary(t.callToolId, SummaryRank(t.task, toolResult))

	t.start.Do(func() {
		t.m.Lock()
		defer t.m.Unlock()
		t.startTime = time.Now() // Record start time
		if t.onCallToolStart != nil {
			t.onCallToolStart(callToolId)
		}
		t.emitter.EmitToolCallStart(callToolId, tool, t.startTime) // should emit after call tool start callback , this call will bind call tool id for emitter
	})

	t.emitter.EmitInfo("start to generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
	handleDone := func() {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now() // Record end time
			t.emitter.EmitToolCallStatus(t.callToolId, "done")
			t.emitter.EmitToolCallDone(callToolId, endTime, t.startTime)
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
			t.emitter.EmitToolCallStatus(t.callToolId, fmt.Sprintf("cancelled by reason: %v", reason))
			t.emitter.EmitToolCallUserCancel(callToolId, endTime, t.startTime)

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
			t.emitter.EmitToolCallError(callToolId, err, endTime, t.startTime)

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
		if destinationIdentifier != "" {
			t.emitter.EmitInfo("tool[%v] destination identifier: %v", tool.Name, destinationIdentifier)
		}
	}
	if utils.IsNil(invokeParams) {
		invokeParams = make(aitool.InvokeParams)
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
		}
		ep.SetReviewMaterials(reqs)
		err := t.config.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request review to db for tool use failed: %v", err)
		}
		t.emitter.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)

		// wait for agree
		config.DoWaitAgree(t.ctx, ep)
		params := ep.GetParams()
		t.emitter.EmitInteractiveRelease(ep.GetId(), params)
		config.CallAfterInteractiveEventReleased(ep.GetId(), params)
		config.CallAfterReview(
			ep.GetSeq(),
			fmt.Sprintf("determite tool[%v]'s params is proper? what should I do?", tool.Name),
			params,
		)
		if params == nil {
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

	// Create buffers to capture stdout and stderr for file saving
	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}

	// Use MultiWriter to write to both the pipe (for streaming) and the buffer (for file saving)
	stdoutMultiWriter := io.MultiWriter(stdoutWriter, stdoutBuffer)
	stderrMultiWriter := io.MultiWriter(stderrWriter, stderrBuffer)

	waitToolStdFlush := t.emitter.EmitToolCallStd(tool.Name, stdoutReader, stderrReader, t.task.GetIndex())
	t.emitter.EmitInfo("start to invoke tool: %v", tool.Name)

	toolResult, err = t.invoke(
		tool, invokeParams, handleUserCancel, handleError,
		stdoutMultiWriter, stderrMultiWriter,
		stdoutBuffer, stderrBuffer,
	)
	if err != nil {
		if toolResult == nil {
			toolResult = &aitool.ToolResult{
				Param:       invokeParams,
				Name:        tool.Name,
				Description: tool.Description,
				ToolCallID:  callToolId,
			}
		}
		toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", tool.Name, err)
		toolResult.Success = false
	}

	// Close pipe writers to signal end of tool output. This triggers ThrottledCopy's
	// final flush of any remaining buffered data. The deferred Close calls are still
	// safe (bufpipe Close is idempotent).
	stdoutWriter.Close()
	stderrWriter.Close()

	// Wait for ThrottledCopy goroutines to finish flushing all remaining buffered data.
	// This ensures all tool stdout/stderr stream events have been sent via gRPC before
	// we emit the tool result event, preserving correct event ordering.
	waitToolStdFlush()

	// Save tool call report markdown (params, stdout, stderr, result merged into one file)
	t.saveToolCallFiles(tool, callToolId, destinationIdentifier, invokeParams, stdoutBuffer, stderrBuffer, toolResult, paramGenDuration, rawAIParamResponse)

	t.emitter.EmitInfo("start to generate and feedback tool[%v] result in task: %#v", tool.Name, t.task.GetName())
	if toolResult.Data != nil {
		toolExecutionResult, ok := toolResult.Data.(*aitool.ToolExecutionResult)
		if ok {
			t.emitter.EmitToolCallResult(callToolId, toolExecutionResult.Result)

		}
	}

	return toolResult, false, nil
}

// markdownCodeFence is 10 backticks used as code fence in markdown to safely nest any content
const markdownCodeFence = "``````````"

func (t *ToolCaller) saveToolCallFiles(
	tool *aitool.Tool,
	callToolId string,
	destinationIdentifier string,
	params aitool.InvokeParams,
	stdoutBuffer *bytes.Buffer,
	stderrBuffer *bytes.Buffer,
	toolResult *aitool.ToolResult,
	paramGenDuration time.Duration,
	rawAIParamResponse string,
) {
	// Get workdir - try to get from config if it's a Config type
	workdir := ""
	if cfg, ok := t.config.(*Config); ok {
		workdir = cfg.Workdir
	}
	if workdir == "" {
		workdir = t.config.GetOrCreateWorkDir()
	}
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}

	// Get task index and name for file naming
	taskIndex := ""
	taskName := ""
	if t.task != nil {
		taskIndex = t.task.GetIndex()
		taskName = t.task.GetSemanticIdentifier()
	}
	if taskIndex == "" {
		taskIndex = "0"
	}

	// Get tool call count for this task (number of previous tool calls + 1)
	toolCallNumber := 1
	if t.task != nil {
		existingResults := t.task.GetAllToolCallResults()
		toolCallNumber = len(existingResults) + 1
	}

	// Generate tool name (sanitized for filename)
	toolName := sanitizeFilename(tool.Name)
	if toolName == "" {
		toolName = "unknown_tool"
	}

	// Build markdown filename: {n}_{toolName}_{identifier}.md
	var mdFileName string
	if destinationIdentifier != "" {
		mdFileName = fmt.Sprintf("%d_%s_%s.md", toolCallNumber, toolName, destinationIdentifier)
	} else {
		mdFileName = fmt.Sprintf("%d_%s.md", toolCallNumber, toolName)
	}

	// Build full file path: task_{index}_{name}/tool_calls/{n}_{tool}_{id}.md
	taskDirName := BuildTaskDirName(taskIndex, taskName)
	toolCallsDir := filepath.Join(workdir, taskDirName, "tool_calls")
	toolCallFilePath := filepath.Join(toolCallsDir, mdFileName)
	t.emitter.EmitToolCallLogDir(callToolId, toolCallFilePath)

	// Ensure tool_calls directory exists
	if err := os.MkdirAll(toolCallsDir, 0755); err != nil {
		log.Errorf("failed to create tool_calls directory %s: %v", toolCallsDir, err)
		return
	}

	// Build the markdown report content
	var md strings.Builder

	// --- Header ---
	md.WriteString(fmt.Sprintf("# Tool Call Report: %s\n\n", tool.Name))

	// --- Basic Info ---
	md.WriteString("## Basic Info\n\n")
	md.WriteString(fmt.Sprintf("- **Tool**: %s\n", tool.Name))
	md.WriteString(fmt.Sprintf("- **Task**: %s\n", taskIndex))
	if destinationIdentifier != "" {
		md.WriteString(fmt.Sprintf("- **Identifier**: %s\n", destinationIdentifier))
	}
	md.WriteString(fmt.Sprintf("- **Call ID**: %s\n", callToolId))
	md.WriteString("\n")

	// --- Parameters ---
	md.WriteString("## Parameters\n\n")
	if paramGenDuration > 0 {
		md.WriteString(fmt.Sprintf("Parameter generation took **%.2fs**\n\n", paramGenDuration.Seconds()))
	}

	// Raw AI Response
	md.WriteString("### Raw AI Response\n\n")
	if rawAIParamResponse != "" {
		md.WriteString(markdownCodeFence + "\n")
		md.WriteString(rawAIParamResponse)
		if !strings.HasSuffix(rawAIParamResponse, "\n") {
			md.WriteString("\n")
		}
		md.WriteString(markdownCodeFence + "\n\n")
	} else {
		md.WriteString("(not available - preset params mode)\n\n")
	}

	// Parsed Parameters (YAML)
	md.WriteString("### Parsed Parameters (YAML)\n\n")
	md.WriteString(markdownCodeFence + "yaml\n")
	md.WriteString(renderParamsAsYAML(params))
	md.WriteString(markdownCodeFence + "\n\n")

	// --- Execution Result ---
	md.WriteString("## Execution Result\n\n")
	resultText := extractResultHumanReadable(toolResult, t.emitter)
	if len(resultText) > 100 {
		md.WriteString(markdownCodeFence + "\n")
		md.WriteString(resultText)
		if !strings.HasSuffix(resultText, "\n") {
			md.WriteString("\n")
		}
		md.WriteString(markdownCodeFence + "\n\n")
	} else if resultText != "" {
		md.WriteString(resultText + "\n\n")
	} else {
		md.WriteString("(empty)\n\n")
	}

	// --- STDOUT ---
	md.WriteString("## STDOUT\n\n")
	stdoutContent := stdoutBuffer.Bytes()
	frameworkMsgPrefix := fmt.Sprintf("invoking tool[%s] ...\n", tool.Name)
	if bytes.HasPrefix(stdoutContent, []byte(frameworkMsgPrefix)) {
		stdoutContent = stdoutContent[len(frameworkMsgPrefix):]
	}
	if len(stdoutContent) > 0 {
		md.WriteString(markdownCodeFence + "\n")
		md.Write(stdoutContent)
		if !bytes.HasSuffix(stdoutContent, []byte("\n")) {
			md.WriteString("\n")
		}
		md.WriteString(markdownCodeFence + "\n\n")
	} else {
		md.WriteString("(empty)\n\n")
	}

	// --- STDERR ---
	md.WriteString("## STDERR\n\n")
	if stderrBuffer.Len() > 0 {
		md.WriteString(markdownCodeFence + "\n")
		md.Write(stderrBuffer.Bytes())
		if !bytes.HasSuffix(stderrBuffer.Bytes(), []byte("\n")) {
			md.WriteString("\n")
		}
		md.WriteString(markdownCodeFence + "\n")
	} else {
		md.WriteString("(empty)\n")
	}

	// Write the single markdown file
	if err := os.WriteFile(toolCallFilePath, []byte(md.String()), 0644); err != nil {
		log.Errorf("failed to save tool call report file: %v", err)
	} else {
		t.emitter.EmitPinFilename(toolCallFilePath)
		log.Infof("saved tool call report to file: %s", toolCallFilePath)
	}
}

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

// extractResultHumanReadable extracts tool result as human-readable text.
// It handles large content saved in files and avoids raw JSON output.
func extractResultHumanReadable(toolResult *aitool.ToolResult, emitter *Emitter) string {
	if toolResult == nil {
		return ""
	}

	// Try to get the original result from ToolExecutionResult
	if toolResult.Data != nil {
		toolExecutionResult, ok := toolResult.Data.(*aitool.ToolExecutionResult)
		if ok && toolExecutionResult.Result != nil {
			resultStr := utils.InterfaceToString(toolExecutionResult.Result)
			// Check if result contains a file path (from handleLargeContent)
			filePathRegex := regexp.MustCompile(`saved in file\[([^\]]+)\]`)
			matches := filePathRegex.FindStringSubmatch(resultStr)
			if len(matches) > 1 {
				filePath := matches[1]
				if fileContent, err := os.ReadFile(filePath); err == nil {
					if emitter != nil {
						emitter.EmitPinFilename(filePath)
					}
					log.Infof("found large result file from tool_invoke.go: %s, also emitting it", filePath)
					return string(fileContent)
				}
				log.Warnf("failed to read large result file %s, using raw string", filePath)
			}
			return resultStr
		}
	}

	// Fallback: build a readable summary from ToolResult fields
	var buf strings.Builder
	if toolResult.Name != "" {
		buf.WriteString(fmt.Sprintf("Tool: %s\n", toolResult.Name))
	}
	if toolResult.Success {
		buf.WriteString("Status: Success\n")
	} else {
		buf.WriteString("Status: Failed\n")
	}
	if toolResult.Error != "" {
		buf.WriteString(fmt.Sprintf("Error: %s\n", toolResult.Error))
	}
	if toolResult.Data != nil {
		dataStr := utils.InterfaceToString(toolResult.Data)
		if dataStr != "" {
			buf.WriteString(fmt.Sprintf("Data: %s\n", dataStr))
		}
	}
	return buf.String()
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
