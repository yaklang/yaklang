package loopinfra

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// FileChangedCallback is called after file content is modified
// Returns: (errorMessage string, hasBlockingErrors bool)
type FileChangedCallback func(content string, operator *reactloops.LoopActionHandlerOperator) (string, bool)

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
	fileChangedCb    FileChangedCallback
	codePrettifyCb   CodePrettifyCallback
	spinDetectionCb  func(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string)
	reflectionPrompt func(startLine, endLine int, reason string) string

	// Runtime reference (set by GetActions)
	runtime aicommon.AIInvokeRuntime

	// Event type for emitting JSON events
	eventType string

	// Behavior flags
	exitAfterWrite bool // whether to call operator.Exit() after successful write (default: true)
}

// SingleFileModificationOption is an option for configuring the factory
type SingleFileModificationOption func(*SingleFileModificationSuiteFactory)

// NewSingleFileModificationSuiteFactory creates a new factory with the given options
func NewSingleFileModificationSuiteFactory(runtime aicommon.AIInvokeRuntime, opts ...SingleFileModificationOption) *SingleFileModificationSuiteFactory {
	f := &SingleFileModificationSuiteFactory{
		prefix:        "content",
		actionSuffix:  "content", // default suffix: write_content, modify_content, etc.
		fileExtension: ".txt",
		aiTagName:     "GEN_CONTENT",
		aiTagVariable: "content",
		aiNodeId:      "content",
		contentType:   "text/plain",
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
