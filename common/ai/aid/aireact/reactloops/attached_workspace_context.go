package reactloops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// AttachedResourceTypeFile matches AIInputEvent AttachedResourceInfo.Type for file resources.
	AttachedResourceTypeFile = aicommon.AttachedResourceTypeFile

	// AttachedResourceKeyDirectoryPath is the frontend workspace / current directory key.
	AttachedResourceKeyDirectoryPath = aicommon.CONTEXT_PROVIDER_KEY_DIRECTORY_PATH

	// AttachedResourceKeyFilePath is the frontend open-file key.
	AttachedResourceKeyFilePath = aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH
)

// WorkspaceAttachedContext carries IDE workspace state parsed from AttachedResourceInfo.
//
// Standard frontend protocol:
//   - Type=file, Key=directory_path, Value=workspace directory absolute path
//   - Type=file, Key=file_path, Value=open file absolute path
//   - Type=selected, Key=content, Value=AttachedCodeSelection JSON
//
// Loop-specific legacy target keys (e.g. code_audit_target_path) are passed as targetKey
// to ParseWorkspaceAttachedContext / InitWorkspaceAttachedContext.
type WorkspaceAttachedContext struct {
	DirectoryPath string
	FilePath      string
	Selection     *aicommon.AttachedCodeSelection
	TargetPath    string
}

func (c *WorkspaceAttachedContext) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.DirectoryPath == "" && c.FilePath == "" && c.Selection == nil && c.TargetPath == ""
}

func (c *WorkspaceAttachedContext) HasDirectory() bool {
	return c != nil && strings.TrimSpace(c.DirectoryPath) != ""
}

func (c *WorkspaceAttachedContext) HasFile() bool {
	return c != nil && strings.TrimSpace(c.FilePath) != ""
}

func (c *WorkspaceAttachedContext) HasSelection() bool {
	return c != nil && c.Selection != nil && strings.TrimSpace(c.Selection.Content) != ""
}

func (c *WorkspaceAttachedContext) HasTargetPath() bool {
	return c != nil && strings.TrimSpace(c.TargetPath) != ""
}

// ResolveTargetPath returns the loop-specific target path attachment.
func (c *WorkspaceAttachedContext) ResolveTargetPath() string {
	if c == nil {
		return ""
	}
	path := strings.TrimSpace(c.TargetPath)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

// ResolveScanTarget picks the best directory to scan/explore.
// Priority: loop-specific target > workspace directory > parent of open file > parent of selection file.
func (c *WorkspaceAttachedContext) ResolveScanTarget() string {
	if c == nil {
		return ""
	}
	if path := c.ResolveTargetPath(); path != "" {
		return path
	}
	if path := strings.TrimSpace(c.DirectoryPath); path != "" {
		return filepath.Clean(path)
	}
	if path := strings.TrimSpace(c.FilePath); path != "" {
		return filepath.Clean(filepath.Dir(path))
	}
	if c.Selection != nil {
		if path := strings.TrimSpace(c.Selection.Path); path != "" {
			return filepath.Clean(filepath.Dir(path))
		}
	}
	return ""
}

// ResolveAttachedScanDirectory returns the scan root from attachments: loop target > directory_path.
// file_path / selection do not infer a scan directory (Phase2 focus only).
func (c *WorkspaceAttachedContext) ResolveAttachedScanDirectory() string {
	if c == nil {
		return ""
	}
	if path := c.ResolveTargetPath(); path != "" {
		return path
	}
	if path := strings.TrimSpace(c.DirectoryPath); path != "" {
		return filepath.Clean(path)
	}
	return ""
}

// ParseWorkspaceAttachedContextFromTask parses workspace attachments from a task.
func ParseWorkspaceAttachedContextFromTask(task aicommon.AIStatefulTask, targetKey string) *WorkspaceAttachedContext {
	if task == nil {
		return nil
	}
	return ParseWorkspaceAttachedContext(task.GetAttachedDatas(), targetKey)
}

// ParseWorkspaceAttachedContext builds workspace context from AttachedResource payloads.
func ParseWorkspaceAttachedContext(attached []*aicommon.AttachedResource, targetKey string) *WorkspaceAttachedContext {
	targetKey = strings.TrimSpace(targetKey)
	ctx := &WorkspaceAttachedContext{}
	for _, data := range attached {
		if data == nil {
			continue
		}
		switch {
		case data.HasType(aicommon.AttachedResourceTypeSelected):
			if !data.HasKey(aicommon.AttachedResourceKeyContent) {
				continue
			}
			if sel, ok := aicommon.ParseAttachedCodeSelection(data); ok {
				ctx.Selection = sel
				if ctx.FilePath == "" {
					if path := strings.TrimSpace(sel.Path); path != "" {
						ctx.FilePath = filepath.Clean(path)
					}
				}
			}
		case data.HasType(AttachedResourceTypeFile):
			switch {
			case targetKey != "" && data.HasKey(targetKey):
				if path := strings.TrimSpace(data.Value); path != "" {
					ctx.TargetPath = filepath.Clean(path)
				}
			case data.HasKey(AttachedResourceKeyDirectoryPath):
				if path := strings.TrimSpace(data.Value); path != "" {
					ctx.DirectoryPath = filepath.Clean(path)
				}
			case data.HasKey(AttachedResourceKeyFilePath):
				if ctx.FilePath == "" {
					if path := strings.TrimSpace(data.Value); path != "" {
						ctx.FilePath = filepath.Clean(path)
					}
				}
			}
		}
	}
	if ctx.IsEmpty() {
		return nil
	}
	return ctx
}

// InitWorkspaceAttachedContext parses attachments, records rendered resources to timeline,
// and returns the parsed workspace context.
func InitWorkspaceAttachedContext(
	r aicommon.AIInvokeRuntime,
	loop *ReActLoop,
	task aicommon.AIStatefulTask,
	targetKey string,
) *WorkspaceAttachedContext {
	if task == nil {
		return nil
	}
	attached := task.GetAttachedDatas()
	RunAttachedExtraResourcesInit(r, loop, attached)
	return ParseWorkspaceAttachedContext(attached, targetKey)
}

// ValidateAttachedDirectoryTarget ensures path exists and is a directory.
func ValidateAttachedDirectoryTarget(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("attached directory path is empty")
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("attached directory %q is not accessible: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("attached path %q is not a directory", path)
	}
	return nil
}

// WithExploreTargetPath returns exploreOpts with target_path injected when scanPath is set.
func WithExploreTargetPath(exploreOpts []ReActLoopOption, scanPath string) []ReActLoopOption {
	scanPath = strings.TrimSpace(scanPath)
	if scanPath == "" {
		return exploreOpts
	}
	return append(exploreOpts, WithVar("target_path", scanPath))
}

// RecordWorkspaceAttachedTimeline writes timeline entries for parsed workspace attachments.
func RecordWorkspaceAttachedTimeline(r aicommon.AIInvokeRuntime, ws *WorkspaceAttachedContext, tagPrefix string) {
	if r == nil || ws == nil {
		return
	}
	tagPrefix = strings.TrimSpace(tagPrefix)
	if tagPrefix == "" {
		tagPrefix = "WORKSPACE"
	}
	if ws.HasDirectory() {
		r.AddToTimeline("["+tagPrefix+"_WORKSPACE]", "工作区目录(附件): "+ws.DirectoryPath)
	}
	if ws.HasFile() {
		r.AddToTimeline("["+tagPrefix+"_OPEN_FILE]", "当前打开文件(附件): "+ws.FilePath)
	}
	if ws.HasSelection() {
		sel := ws.Selection
		summary := fmt.Sprintf("用户选中代码片段: %s", utils.ShrinkTextBlock(sel.Content, 120))
		if sel.StartLine > 0 && sel.EndLine > 0 {
			summary = fmt.Sprintf("用户选中 %s:%d-%d\n%s", sel.Path, sel.StartLine, sel.EndLine, utils.ShrinkTextBlock(sel.Content, 120))
		}
		r.AddToTimeline("["+tagPrefix+"_SELECTION]", summary)
	}
}

// RecordExploreTargetTimeline records which path Phase1 dir_explore will use.
func RecordExploreTargetTimeline(r aicommon.AIInvokeRuntime, ws *WorkspaceAttachedContext, scanPath, tagPrefix string) {
	scanPath = strings.TrimSpace(scanPath)
	if r == nil || scanPath == "" {
		return
	}
	tagPrefix = strings.TrimSpace(tagPrefix)
	if tagPrefix == "" {
		tagPrefix = "WORKSPACE"
	}
	label := "扫描目标目录: "
	if ws != nil && ws.HasTargetPath() {
		label = "扫描目标目录(loop target): "
	} else if ws != nil && ws.HasDirectory() {
		label = "扫描目标目录(directory_path): "
	}
	r.AddToTimeline("["+tagPrefix+"_TARGET]", label+scanPath)
}

// FormatAttachedDirectoryValidationError builds a user-facing error when attached scan path is invalid.
func FormatAttachedDirectoryValidationError(scanPath, targetKey string, err error) string {
	targetKey = strings.TrimSpace(targetKey)
	if targetKey == "" {
		targetKey = "(loop target key)"
	}
	return fmt.Sprintf(
		"附件指定的扫描目录无效: %q（%v）。请确认 Type=%q、Key=%q 或 Key=%q Value 为存在的目录绝对路径",
		scanPath, err, AttachedResourceTypeFile, AttachedResourceKeyDirectoryPath, targetKey,
	)
}

// FormatAttachedCodeSelection renders selection for loop prompts.
func FormatAttachedCodeSelection(sel *aicommon.AttachedCodeSelection) string {
	if sel == nil || strings.TrimSpace(sel.Content) == "" {
		return ""
	}
	var b strings.Builder
	if path := strings.TrimSpace(sel.Path); path != "" {
		b.WriteString(fmt.Sprintf("- 文件: `%s`\n", path))
	}
	if sel.StartLine > 0 && sel.EndLine > 0 {
		b.WriteString(fmt.Sprintf("- 行号: %d-%d\n", sel.StartLine, sel.EndLine))
	}
	if lang := strings.TrimSpace(sel.Language); lang != "" {
		b.WriteString(fmt.Sprintf("- 语言: %s\n", lang))
	}
	b.WriteString("```\n")
	b.WriteString(utils.ShrinkTextBlock(sel.Content, 2000))
	b.WriteString("\n```")
	return b.String()
}

// FocusPromptVars returns template variables for audit loop prompts (open file / selection focus).
func FocusPromptVars(focusFilePath string, selection *aicommon.AttachedCodeSelection) map[string]any {
	focusFilePath = strings.TrimSpace(focusFilePath)
	selectionText := FormatAttachedCodeSelection(selection)
	hasSelectionFocus := selectionText != ""
	hasFocus := focusFilePath != "" || hasSelectionFocus
	return map[string]any{
		"FocusFilePath":     focusFilePath,
		"Selection":         selectionText,
		"HasFocus":          hasFocus,
		"HasSelectionFocus": hasSelectionFocus,
		"HasOpenFileFocus":  focusFilePath != "",
	}
}

// ResolveFocusFilePath returns the best file path for frontend focus (open file beats selection path).
func ResolveFocusFilePath(focusFilePath string, selection *aicommon.AttachedCodeSelection) string {
	if path := strings.TrimSpace(focusFilePath); path != "" {
		return path
	}
	if selection != nil {
		if path := strings.TrimSpace(selection.Path); path != "" {
			return path
		}
	}
	return ""
}
