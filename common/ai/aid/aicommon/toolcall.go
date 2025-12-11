package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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

	ctx    context.Context
	cancel context.CancelFunc

	generateToolParamsBuilder func(tool *aitool.Tool, toolName string) (string, error)

	m               *sync.Mutex
	onCallToolStart func(callToolId string)
	onCallToolEnd   func(callToolId string)

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

func (t *ToolCaller) GetParamGeneratingPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if t.generateToolParamsBuilder == nil {
		return "", fmt.Errorf("generateToolParamsBuilder is nil")
	}

	return t.generateToolParamsBuilder(tool, toolName)
}

func (t *ToolCaller) CallTool(tool *aitool.Tool) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	return t.CallToolWithExistedParams(tool, false, make(aitool.InvokeParams))
}

func (t *ToolCaller) generateParams(tool *aitool.Tool, handleError func(i any)) (aitool.InvokeParams, error) {
	emitter := t.emitter
	paramsPrompt, err := t.GetParamGeneratingPrompt(tool, tool.Name)
	if err != nil {
		emitter.EmitError("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
		handleError(fmt.Sprintf("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName()))
		return nil, err
	}
	invokeParams := aitool.InvokeParams{}
	err = CallAITransaction(t.config, paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("call-tools", false, emitter)
		callToolAction, err := ExtractValidActionFromStream(t.config.GetContext(), stream, "call-tool")
		if err != nil {
			emitter.EmitError("error extract tool params: %v", err)
			return utils.Errorf("error extracting action params: %v", err)
		}
		for k, v := range callToolAction.GetInvokeParams("params") {
			invokeParams.Set(k, v)
		}
		return nil
	})
	if err != nil {
		emitter.EmitError("error calling AI for tool[%v] params: %v", tool.Name, err)
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, err
	}
	return invokeParams, nil
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
		if t.onCallToolStart != nil {
			t.onCallToolStart(callToolId)
		}
		t.emitter.EmitToolCallStart(callToolId, tool) // should emit after call tool start callback , this call will bind call tool id for emitter
	})

	t.emitter.EmitInfo("start to generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
	handleDone := func() {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			t.emitter.EmitToolCallStatus(t.callToolId, "done")
			t.emitter.EmitToolCallDone(callToolId)
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleUserCancel := func(reason any) {
		t.done.Do(func() {
			t.emitter.EmitToolCallStatus(t.callToolId, fmt.Sprintf("cancelled by reason: %v", reason))
			t.emitter.EmitToolCallUserCancel(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleError := func(err any) {
		t.done.Do(func() {
			t.emitter.EmitToolCallError(callToolId, err)
			t.m.Lock()
			defer t.m.Unlock()

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
	if presetParams {
		invokeParams = presetInvokeParams
		t.emitter.EmitInfo("use preset params for tool[%v]: %v", tool.Name, invokeParams)
	} else {
		generatedParams, err := t.generateParams(tool, handleError)
		if err != nil {
			return nil, false, utils.Errorf("error generating params for tool[%v]: %v", tool.Name, err)
		}
		invokeParams = generatedParams
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

	t.emitter.EmitToolCallStd(tool.Name, stdoutReader, stderrReader, t.task.GetIndex())
	t.emitter.EmitInfo("start to invoke tool: %v", tool.Name)

	toolResult, err = t.invoke(tool, invokeParams, handleUserCancel, handleError, stdoutMultiWriter, stderrMultiWriter)
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

	// Save tool call files (params, stdout, stderr, result)
	t.saveToolCallFiles(tool, callToolId, invokeParams, stdoutBuffer, stderrBuffer, toolResult)

	t.emitter.EmitInfo("start to generate and feedback tool[%v] result in task: %#v", tool.Name, t.task.GetName())
	if toolResult.Data != nil {
		toolExecutionResult, ok := toolResult.Data.(*aitool.ToolExecutionResult)
		if ok {
			t.emitter.EmitToolCallResult(callToolId, toolExecutionResult.Result)

		}
	}

	return toolResult, false, nil
}

func (t *ToolCaller) saveToolCallFiles(
	tool *aitool.Tool,
	callToolId string,
	params aitool.InvokeParams,
	stdoutBuffer *bytes.Buffer,
	stderrBuffer *bytes.Buffer,
	toolResult *aitool.ToolResult,
) {
	// Get workdir - try to get from config if it's a Config type
	workdir := ""
	if cfg, ok := t.config.(*Config); ok {
		workdir = cfg.Workdir
	}
	if workdir == "" {
		workdir = consts.TempAIDir(t.config.GetRuntimeId())
	}
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}

	// Ensure workdir exists
	if err := os.MkdirAll(workdir, 0755); err != nil {
		log.Errorf("failed to create workdir %s: %v", workdir, err)
		return
	}

	// Get task index for file naming
	taskIndex := ""
	if t.task != nil {
		taskIndex = t.task.GetIndex()
	}
	if taskIndex == "" {
		taskIndex = "0"
	}

	// Get tool call count for this task (number of previous tool calls + 1)
	toolCallNumber := 1
	if t.task != nil {
		// Get count of existing tool call results, then add 1 for current call
		existingResults := t.task.GetAllToolCallResults()
		toolCallNumber = len(existingResults) + 1
	}

	// Generate file index in format: taskIndex_number
	fileIndex := fmt.Sprintf("%s_%d", taskIndex, toolCallNumber)

	// Generate tool identifier (sanitize tool name for filename)
	toolIdentifier := sanitizeFilename(tool.Name)
	if toolIdentifier == "" {
		toolIdentifier = "unknown-tool"
	}
	// For stderr and result, use original tool name (sanitized)
	toolName := sanitizeFilename(tool.Name)
	if toolName == "" {
		toolName = "unknown-tool"
	}

	// Generate file names
	// params and stdout use identifier, stderr and result use toolname
	paramsFilename := filepath.Join(workdir, fmt.Sprintf("tool-call-%s-params-%s.txt", toolIdentifier, fileIndex))
	stdoutFilename := filepath.Join(workdir, fmt.Sprintf("tool-call-%s-stdout-%s.txt", toolIdentifier, fileIndex))
	stderrFilename := filepath.Join(workdir, fmt.Sprintf("tool-call-%s-stderr-%s.txt", toolName, fileIndex))
	resultFilename := filepath.Join(workdir, fmt.Sprintf("tool-call-%s-result-%s.txt", toolName, fileIndex))

	// Save params file
	paramsJSON := utils.Jsonify(params)
	if err := os.WriteFile(paramsFilename, []byte(paramsJSON), 0644); err != nil {
		log.Errorf("failed to save tool call params file: %v", err)
	} else {
		t.emitter.EmitPinFilename(paramsFilename)
		log.Infof("saved tool call params to file: %s", paramsFilename)
	}

	// Save stdout file
	// Filter out framework message "invoking tool[xxx] ...\n" - only save tool callback's actual output
	stdoutContent := stdoutBuffer.Bytes()
	frameworkMsgPrefix := fmt.Sprintf("invoking tool[%s] ...\n", tool.Name)
	if bytes.HasPrefix(stdoutContent, []byte(frameworkMsgPrefix)) {
		stdoutContent = stdoutContent[len(frameworkMsgPrefix):]
	}
	if len(stdoutContent) > 0 {
		if err := os.WriteFile(stdoutFilename, stdoutContent, 0644); err != nil {
			log.Errorf("failed to save tool call stdout file: %v", err)
		} else {
			t.emitter.EmitPinFilename(stdoutFilename)
			log.Infof("saved tool call stdout to file: %s", stdoutFilename)
		}
	}

	// Save stderr file
	if stderrBuffer.Len() > 0 {
		if err := os.WriteFile(stderrFilename, stderrBuffer.Bytes(), 0644); err != nil {
			log.Errorf("failed to save tool call stderr file: %v", err)
		} else {
			t.emitter.EmitPinFilename(stderrFilename)
			log.Infof("saved tool call stderr to file: %s", stderrFilename)
		}
	}

	// Save result file
	// Always save the full result to file, even if it's large
	var resultContent []byte
	if toolResult != nil {
		// Try to get the original result from ToolExecutionResult
		if toolResult.Data != nil {
			toolExecutionResult, ok := toolResult.Data.(*aitool.ToolExecutionResult)
			if ok && toolExecutionResult.Result != nil {
				// Get the original result before it was truncated
				// If result was saved to a file in tool_invoke.go, we need to read it
				resultStr := utils.InterfaceToString(toolExecutionResult.Result)
				// Check if result contains a file path (from handleLargeContent)
				filePathRegex := regexp.MustCompile(`saved in file\[([^\]]+)\]`)
				matches := filePathRegex.FindStringSubmatch(resultStr)
				if len(matches) > 1 {
					// Extract file path and read it
					filePath := matches[1]
					if fileContent, err := os.ReadFile(filePath); err == nil {
						resultContent = fileContent
						// Also emit the original file
						t.emitter.EmitPinFilename(filePath)
						log.Infof("found large result file from tool_invoke.go: %s, also emitting it", filePath)
					} else {
						// Fallback to JSON
						resultContent = []byte(utils.Jsonify(toolExecutionResult.Result))
						log.Warnf("failed to read large result file %s: %v, using JSON fallback", filePath, err)
					}
				} else {
					// Result is not truncated, save as JSON
					resultContent = []byte(utils.Jsonify(toolExecutionResult.Result))
				}
			} else {
				// Fallback to full tool result
				resultContent = []byte(utils.Jsonify(toolResult))
			}
		} else {
			// Save full tool result
			resultContent = []byte(utils.Jsonify(toolResult))
		}
	}

	// Always save result file, even if empty
	if err := os.WriteFile(resultFilename, resultContent, 0644); err != nil {
		log.Errorf("failed to save tool call result file: %v", err)
	} else {
		t.emitter.EmitPinFilename(resultFilename)
		log.Infof("saved tool call result to file: %s", resultFilename)
	}
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
