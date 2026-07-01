package loopinfra

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// LoopVarCodeLineBase is the loop variable storing the 0-based offset between full_code line
// indices and absolute editor file line numbers when full_code is a selection snippet.
const LoopVarCodeLineBase = "code_line_base"

// LoopVarInitSeedFullCode stores the initial full_code seeded at loop start (e.g. from disk).
// Used to detect cross-session editor reuse and to allow one write_code replace in edit mode.
const LoopVarInitSeedFullCode = "init_seed_full_code"

// LoopVarCodeSeededOnly is true when full_code still equals the init seed and this loop has
// not committed a write/modify yet.
const LoopVarCodeSeededOnly = "code_seeded_only"

// NormalizeActionLineNumber maps AI-supplied line numbers onto full_code indices.
// When code_line_base > 0, the model may pass absolute file line numbers (e.g. 28) while
// full_code only holds a selection; convert to 1-based indices inside full_code (e.g. 1).
func NormalizeActionLineNumber(loop *reactloops.ReActLoop, fullCodeVar string, line int) int {
	if loop == nil || line <= 0 {
		return line
	}
	base := loop.GetInt(LoopVarCodeLineBase)
	if base <= 0 {
		return line
	}
	relative := line - base
	if relative < 1 {
		return line
	}
	fullCode := loop.Get(fullCodeVar)
	lineCount := len(utils.ParseStringToRawLines(fullCode))
	if relative <= lineCount {
		return relative
	}
	return line
}

// FileChangedCallback is called after file content is modified
// Returns: (errorMessage string, hasBlockingErrors bool)
type FileChangedCallback func(content string, operator *reactloops.LoopActionHandlerOperator) (string, bool)

// PostSyntaxCleanHook runs after static lint passes and before the loop may exit on clean syntax.
// Return (feedback, blockExit): when blockExit is true, feedback is sent to the model and the loop continues.
type PostSyntaxCleanHook func(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator) (feedback string, blockExit bool)

// CodePrettifyCallback is called to prettify AI-generated code (extract line numbers etc.)
// Returns: (startLine, endLine int, prettifiedCode string, fixed bool)
type CodePrettifyCallback func(code string) (int, int, string, bool)

// SingleFileModificationSuiteFactory creates a suite of file modification actions
type SingleFileModificationSuiteFactory struct {
	// Configuration
	prefix        string // loop variable prefix, e.g., "yak", "python"
	actionSuffix  string // action name suffix, e.g., "content", "code" -> write_content, write_code
	fileExtension string // file extension, e.g., ".yak", ".py"

	// AI Tag configuration
	aiTagName     string // AI tag name, e.g., "GEN_CODE"
	aiTagVariable string // AI tag variable name, e.g., "yak_code"
	aiNodeId      string // AI node ID, e.g., "yaklang-code"
	contentType   string // content type, e.g., "code/yaklang"

	// Callbacks
	fileChangedCb       FileChangedCallback
	postSyntaxCleanHook PostSyntaxCleanHook
	codePrettifyCb      CodePrettifyCallback
	spinDetectionCb  func(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string)
	reflectionPrompt func(startLine, endLine int, reason string) string

	// Runtime reference (set by GetActions)
	runtime aicommon.AIInvokeRuntime

	// Event type for emitting JSON events
	eventType string

	// Behavior flags
	exitAfterWrite      bool // whether to call operator.Exit() after successful write (default: true)
	exitWhenSyntaxClean bool // whether to call operator.Exit() after modify/insert/delete when syntax check passes
	deferDiskWrite      bool // keep code in loop memory only; frontend writes disk after user accepts diff
}

// SingleFileModificationOption is an option for configuring the factory
type SingleFileModificationOption func(*SingleFileModificationSuiteFactory)

// NewSingleFileModificationSuiteFactory creates a new factory with the given options
func NewSingleFileModificationSuiteFactory(runtime aicommon.AIInvokeRuntime, opts ...SingleFileModificationOption) *SingleFileModificationSuiteFactory {
	f := &SingleFileModificationSuiteFactory{
		prefix:         "content",
		actionSuffix:   "content", // default suffix: write_content, modify_content, etc.
		fileExtension:  ".txt",
		aiTagName:      "GEN_CONTENT",
		aiTagVariable:  "content",
		aiNodeId:       "content",
		contentType:    "text/plain",
		eventType:      "yaklang_code_editor",
		exitAfterWrite: true,
		runtime:        runtime,
		codePrettifyCb: func(code string) (int, int, string, bool) {
			return defaultPrettifyAITagCode(code)
		},
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// WithLoopVarsPrefix sets the loop variable prefix
// This affects variable names like "{prefix}_code", "full_code", "filename"
func WithLoopVarsPrefix(prefix string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.prefix = prefix
	}
}

// WithActionSuffix sets the action name suffix
// e.g., "code" will generate actions like "write_code", "modify_code", "insert_code", "delete_code"
// e.g., "content" (default) will generate "write_content", "modify_content", etc.
func WithActionSuffix(suffix string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.actionSuffix = suffix
	}
}

// WithFileExtension sets the file extension for generated files
func WithFileExtension(ext string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.fileExtension = ext
	}
}

// WithAITagConfig sets the AI tag configuration
func WithAITagConfig(tagName, variableName, nodeId, contentType string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.aiTagName = tagName
		f.aiTagVariable = variableName
		f.aiNodeId = nodeId
		f.contentType = contentType
	}
}

// WithFileChanged sets the callback for when file content is changed
func WithFileChanged(cb FileChangedCallback) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.fileChangedCb = cb
	}
}

// WithPostSyntaxCleanHook sets a hook invoked after lint passes and before exit-on-clean.
func WithPostSyntaxCleanHook(hook PostSyntaxCleanHook) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.postSyntaxCleanHook = hook
	}
}

// WithCodePrettify sets the callback for prettifying AI-generated code
func WithCodePrettify(cb CodePrettifyCallback) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.codePrettifyCb = cb
	}
}

// WithSpinDetection sets the callback for detecting spinning (repetitive small modifications)
func WithSpinDetection(cb func(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string)) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.spinDetectionCb = cb
	}
}

// WithReflectionPrompt sets the callback for generating reflection prompts when spinning is detected
func WithReflectionPrompt(cb func(startLine, endLine int, reason string) string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.reflectionPrompt = cb
	}
}

// WithEventType sets the event type for emitting JSON events
func WithEventType(eventType string) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.eventType = eventType
	}
}

// WithExitAfterWrite controls whether the loop should exit after a successful write action.
// Default is true (exit after write). Set to false for scenarios like report generation
// where the AI needs to continue modifying content after the initial write.
func WithExitAfterWrite(exit bool) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.exitAfterWrite = exit
	}
}

// ShouldExitAfterWrite returns whether the loop should exit after a successful write action
func (f *SingleFileModificationSuiteFactory) ShouldExitAfterWrite() bool {
	return f.exitAfterWrite
}

// WithExitWhenSyntaxClean controls whether modify/insert/delete should exit the loop
// once OnFileChanged reports no blocking syntax errors (e.g. Yak Runner static analysis).
func WithExitWhenSyntaxClean(exit bool) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.exitWhenSyntaxClean = exit
	}
}

func (f *SingleFileModificationSuiteFactory) ShouldExitWhenSyntaxClean() bool {
	return f.exitWhenSyntaxClean
}

// WithDeferDiskWrite skips os.WriteFile in write/modify/insert/delete actions.
// Loop state and yaklang_code_change events still update; the frontend applies to disk after review.
func WithDeferDiskWrite(deferWrite bool) SingleFileModificationOption {
	return func(f *SingleFileModificationSuiteFactory) {
		f.deferDiskWrite = deferWrite
	}
}

func (f *SingleFileModificationSuiteFactory) ShouldDeferDiskWrite() bool {
	return f.deferDiskWrite
}

func (f *SingleFileModificationSuiteFactory) applySyntaxLintResult(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	hasBlockingErrors bool,
	exitOnClean bool,
) {
	lintStatusVar := f.GetLintStatusVariableName()
	if hasBlockingErrors {
		loop.Set(lintStatusVar, "false")
		// 修复节点状态反馈: 检测到语法错误, 阻止本轮退出并提示前端进入修复
		// 关键词: EmitStatus, 语法错误, 修复节点, DisallowNextLoopExit
		reactloops.EmitStatus(loop, "检测到语法错误，修复中 / Syntax error detected, fixing...")
		op.DisallowNextLoopExit()
		op.Continue()
		return
	}
	loop.Set(lintStatusVar, "true")
	// 语法通过状态反馈, 让前端看到从"修复中"到"通过"的状态切换
	reactloops.EmitStatus(loop, "语法检查通过 / Syntax check passed")
	if exitOnClean && f.postSyntaxCleanHook != nil {
		feedback, blockExit := f.postSyntaxCleanHook(loop, op)
		if blockExit {
			if feedback != "" {
				op.Feedback(feedback)
			}
			op.DisallowNextLoopExit()
			op.Continue()
			return
		}
	}
	if exitOnClean {
		op.Exit()
	}
}

// CommitAfterCodeEdit persists code, runs lint, and records yaklang code change state.
func (f *SingleFileModificationSuiteFactory) CommitAfterCodeEdit(
	loop *reactloops.ReActLoop,
	op *reactloops.LoopActionHandlerOperator,
	filename, fullCode, sourceAction, changeReason, editorPartial string,
	successTimeline, failTimeline, successMsg string,
) error {
	runtime := f.GetRuntime()
	writeErr := f.replaceLoopFileContent(runtime, filename, fullCode, successTimeline, failTimeline, successMsg)
	if writeErr != nil {
		return writeErr
	}
	loop.Set(f.GetFullCodeVariableName(), fullCode)

	errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
	f.applySyntaxLintResult(loop, op, hasBlockingErrors, f.ShouldExitWhenSyntaxClean())

	loop.GetEmitter().EmitPinFilename(filename)
	_, _ = f.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      fullCode,
		Path:         filename,
		SourceAction: sourceAction,
		ChangeReason: changeReason,
		EventOp:      loopYaklangCodeEventOpReplace,
		EmitEvent:    true,
	})
	if editorPartial != "" {
		loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, sourceAction, editorPartial)
	}
	if errMsg != "" {
		op.Feedback(errMsg)
	}
	return nil
}

// handleModifyByOldSnippet replaces exact text matches via modify_code + old_snippet.
func (f *SingleFileModificationSuiteFactory) handleModifyByOldSnippet(
	loop *reactloops.ReActLoop,
	action *aicommon.Action,
	op *reactloops.LoopActionHandlerOperator,
	actionName, filename, fullCodeVar, codeVar string,
) {
	runtime := f.GetRuntime()
	invoker := loop.GetInvoker()

	oldSnippet := action.GetString("old_snippet")
	replaceAll := action.GetBool("replace_all")
	reason := action.GetString("modify_code_reason")
	newCode := loop.Get(codeVar)

	_, _, codeSegment, fixedCode := f.PrettifyCode(newCode)
	if fixedCode {
		newCode = codeSegment
	}

	if strings.TrimSpace(newCode) == "" {
		op.Fail("modify_code with old_snippet requires non-empty new code in GEN_CODE block")
		return
	}

	fullCode := loop.Get(fullCodeVar)
	editor := memedit.NewMemEditor(fullCode)

	var matches []*memedit.Range
	_ = editor.FindStringRange(oldSnippet, func(r *memedit.Range) error {
		matches = append(matches, r)
		return nil
	})

	if len(matches) == 0 {
		msg := fmt.Sprintf(`【modify_code 失败】未找到 old_snippet。

请确保 old_snippet 与 full_code 完全一致（含空格与换行）。
可改用行号 modify_start_line/modify_end_line，或扩大上下文后重试。

old_snippet 预览：
%s`, utils.ShrinkTextBlock(oldSnippet, 300))
		invoker.AddToTimeline("modify_snippet_not_found", msg)
		op.Feedback(msg)
		op.Continue()
		return
	}

	if len(matches) > 1 && !replaceAll {
		var lines []string
		for i, r := range matches {
			pos := editor.GetPositionByOffset(r.GetStartOffset())
			lines = append(lines, fmt.Sprintf("  match %d: line %d", i+1, pos.GetLine()))
		}
		msg := fmt.Sprintf(`【modify_code 失败】old_snippet 匹配 %d 处，不唯一。

%s

请提供更长的 old_snippet 以唯一定位，或设置 replace_all=true。`, len(matches), strings.Join(lines, "\n"))
		invoker.AddToTimeline("modify_snippet_ambiguous", msg)
		op.Feedback(msg)
		op.Continue()
		return
	}

	if replaceAll {
		for i := len(matches) - 1; i >= 0; i-- {
			if err := editor.UpdateTextByRange(matches[i], newCode); err != nil {
				op.Fail("failed to replace snippet: " + err.Error())
				return
			}
		}
	} else {
		if err := editor.UpdateTextByRange(matches[0], newCode); err != nil {
			op.Fail("failed to replace snippet: " + err.Error())
			return
		}
	}

	fullCode = editor.GetSourceCode()

	invoker.AddToTimeline("modify_code", fmt.Sprintf("replaced snippet (%d match(es), replace_all=%v)", len(matches), replaceAll))
	if reason != "" {
		runtime.AddToTimeline("modify_reason", reason)
	}

	successMsg := fmt.Sprintf("SUCCESS: replaced snippet, wrote %d bytes to file: %s", len(fullCode), filename)
	if err := f.CommitAfterCodeEdit(
		loop, op, filename, fullCode, actionName, reason, newCode,
		"modify_success", "modify_write_failed", successMsg,
	); err != nil {
		op.Fail(fmt.Sprintf("failed to write modified content: %v", err))
		return
	}

	log.Infof("modify_code (old_snippet) done")
	op.Continue()
}

// GetAITagOption returns the ReActLoopOption for configuring AI tag extraction
func (f *SingleFileModificationSuiteFactory) GetAITagOption() reactloops.ReActLoopOption {
	return reactloops.WithAITagFieldWithAINodeId(f.aiTagName, f.aiTagVariable, f.aiNodeId, f.contentType)
}

// GetActions returns all the file modification actions as ReActLoopOptions
func (f *SingleFileModificationSuiteFactory) GetActions() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		f.buildWriteAction(),
		f.buildModifyAction(),
		f.buildInsertAction(),
		f.buildDeleteAction(),
	}
}

// GetCodeVariableName returns the variable name for AI-generated code
func (f *SingleFileModificationSuiteFactory) GetCodeVariableName() string {
	return f.aiTagVariable
}

// GetFullCodeVariableName returns the variable name for full file content
// For backward compatibility, "yak" prefix uses "full_code", others use "full_{prefix}_code"
func (f *SingleFileModificationSuiteFactory) GetFullCodeVariableName() string {
	if f.prefix == "yak" || f.prefix == "code" {
		return "full_code"
	}
	return "full_" + f.prefix + "_code"
}

// GetFilenameVariableName returns the variable name for filename
// For backward compatibility, "yak" prefix uses "filename", others use "{prefix}_filename"
func (f *SingleFileModificationSuiteFactory) GetFilenameVariableName() string {
	if f.prefix == "yak" || f.prefix == "code" {
		return "filename"
	}
	return f.prefix + "_filename"
}

// GetRuntime returns the AIInvokeRuntime
func (f *SingleFileModificationSuiteFactory) GetRuntime() aicommon.AIInvokeRuntime {
	return f.runtime
}

// GetFileExtension returns the file extension
func (f *SingleFileModificationSuiteFactory) GetFileExtension() string {
	return f.fileExtension
}

// GetActionName returns the full action name with suffix
// e.g., GetActionName("write") with suffix "code" returns "write_code"
func (f *SingleFileModificationSuiteFactory) GetActionName(baseName string) string {
	return baseName + "_" + f.actionSuffix
}

// GetLintStatusVariableName returns the variable name for lint/syntax check status
// e.g., "yak" prefix returns "yak_lint_ok", "python" prefix returns "python_lint_ok"
func (f *SingleFileModificationSuiteFactory) GetLintStatusVariableName() string {
	return f.prefix + "_lint_ok"
}

// GetEventType returns the event type for emitting JSON events
func (f *SingleFileModificationSuiteFactory) GetEventType() string {
	return f.eventType
}

// OnFileChanged calls the file changed callback if configured
// Returns (errorMessage, hasBlockingErrors)
func (f *SingleFileModificationSuiteFactory) OnFileChanged(content string, operator *reactloops.LoopActionHandlerOperator) (string, bool) {
	if f.fileChangedCb == nil {
		return "", false
	}
	return f.fileChangedCb(content, operator)
}

// PrettifyCode calls the code prettify callback
func (f *SingleFileModificationSuiteFactory) PrettifyCode(code string) (int, int, string, bool) {
	if f.codePrettifyCb == nil {
		return 0, 0, code, false
	}
	return f.codePrettifyCb(code)
}

// DetectSpinning checks for spinning behavior
func (f *SingleFileModificationSuiteFactory) DetectSpinning(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string) {
	if f.spinDetectionCb == nil {
		return false, ""
	}
	return f.spinDetectionCb(loop, startLine, endLine)
}

// GetReflectionPrompt generates a reflection prompt for spinning
func (f *SingleFileModificationSuiteFactory) GetReflectionPrompt(startLine, endLine int, reason string) string {
	if f.reflectionPrompt == nil {
		return ""
	}
	return f.reflectionPrompt(startLine, endLine, reason)
}

// defaultPrettifyAITagCode is the default implementation for prettifying AI-generated code
// It extracts line numbers from the code and returns the cleaned code
var lineNumberRegex = regexp.MustCompile(`^(\d+)\s+\|\s`)

func defaultPrettifyAITagCode(i string) (start, end int, result string, fixed bool) {
	lines := utils.ParseStringToRawLines(i)
	if len(lines) == 0 {
		return 0, 0, i, false
	}

	// Skip leading empty lines
	startIdx := 0
	for startIdx < len(lines) && strings.TrimSpace(lines[startIdx]) == "" {
		startIdx++
	}

	// Skip trailing empty lines
	endIdx := len(lines) - 1
	for endIdx >= startIdx && strings.TrimSpace(lines[endIdx]) == "" {
		endIdx--
	}

	// If all lines are empty
	if startIdx > endIdx {
		return 0, 0, i, false
	}

	// Try to recognize line number from first line
	firstLine := lines[startIdx]
	match := lineNumberRegex.FindStringSubmatch(firstLine)
	if match == nil {
		// First line has no line number, quick fail
		return 0, 0, i, false
	}

	// Parse first line number
	firstLineNum, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, i, false
	}

	// Validate all lines and collect results
	var processedLines []string
	expectedLineNum := firstLineNum

	for j := startIdx; j <= endIdx; j++ {
		line := lines[j]

		// Allow empty lines
		if strings.TrimSpace(line) == "" {
			processedLines = append(processedLines, "")
			continue
		}

		// Try to match line number
		match := lineNumberRegex.FindStringSubmatch(line)
		if match == nil {
			// This line has no line number format, quick fail
			return 0, 0, i, false
		}

		// Parse line number
		lineNum, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, 0, i, false
		}

		// Check if line numbers are consecutive
		if lineNum != expectedLineNum {
			// Line numbers not consecutive, quick fail
			return 0, 0, i, false
		}

		expectedLineNum++

		// Extract code part (remove line number prefix)
		indexOfPipe := strings.Index(line, "|")
		if indexOfPipe == -1 {
			return 0, 0, i, false
		}
		// Skip | and the space after it
		codeStart := indexOfPipe + 2 // 1 for |, 1 for the required space
		var code string
		if codeStart < len(line) {
			code = line[codeStart:]
		}
		processedLines = append(processedLines, code)
	}

	// Build the fixed result
	var buf bytes.Buffer
	for _, line := range processedLines {
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	finalResult := strings.TrimSuffix(buf.String(), "\n")

	return firstLineNum, expectedLineNum - 1, finalResult, true
}
