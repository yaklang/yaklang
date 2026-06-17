package aicommon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// Yaklang code editor attachment protocol (write_yaklang_code / Yak Runner):
//
//   - Type=file, Key=directory_path, Value=workspace directory absolute path
//   - Type=file, Key=file_path, Value=open file absolute path
//   - Type=selected, Key=content, Value=AttachedCodeSelection JSON (path/content/line numbers)
//
// Other loops use domain-specific keys (e.g. code_security_audit uses code_audit_target_path).
// Frontend strings must match CONTEXT_PROVIDER_* and AttachedResource* constants.

const (
	YaklangAttachedResourceKeyWorkspaceDirectory = CONTEXT_PROVIDER_KEY_DIRECTORY_PATH
	YaklangAttachedResourceKeyEditorFile         = CONTEXT_PROVIDER_KEY_FILE_PATH
)

// YaklangEditorContext carries IDE workspace state parsed from frontend attachments.
type YaklangEditorContext struct {
	WorkspacePath string
	EditorFile    string
	Selection     *AttachedCodeSelection
}

func (c *YaklangEditorContext) HasWorkspace() bool {
	return c != nil && strings.TrimSpace(c.WorkspacePath) != ""
}

func (c *YaklangEditorContext) HasEditorFile() bool {
	return c != nil && strings.TrimSpace(c.EditorFile) != ""
}

func (c *YaklangEditorContext) HasSelection() bool {
	return c != nil && c.Selection != nil && strings.TrimSpace(c.Selection.Content) != ""
}

func (c *YaklangEditorContext) IsEmpty() bool {
	if c == nil {
		return true
	}
	return !c.HasWorkspace() && !c.HasEditorFile() && !c.HasSelection()
}

// IsCreateMode is true when no editor target file is attached
// (nil context, directory_path only, or selection without resolvable path).
// Delivery uses yaklang_code_change op=create and gen_code_* at loop flush.
func (c *YaklangEditorContext) IsCreateMode() bool {
	return c == nil || !c.HasEditorFile()
}

// IsCodePreviewOnly is deprecated; use IsCreateMode.
func (c *YaklangEditorContext) IsCodePreviewOnly() bool {
	return c.IsCreateMode()
}

// ParseYaklangEditorContextFromAttached builds editor context from AttachedResourceInfo payloads.
func ParseYaklangEditorContextFromAttached(attachedDatas []*AttachedResource) *YaklangEditorContext {
	ctx := &YaklangEditorContext{}
	for _, data := range attachedDatas {
		if data == nil {
			continue
		}
		switch {
		case data.HasType(AttachedResourceTypeSelected):
			if !data.HasKey(AttachedResourceKeyContent) {
				continue
			}
			if sel, ok := ParseAttachedCodeSelection(data); ok {
				ctx.Selection = sel
				if path := strings.TrimSpace(sel.Path); path != "" {
					ctx.EditorFile = filepath.Clean(path)
				}
			}
		case data.HasType(AttachedResourceTypeFile):
			switch {
			case data.HasKey(YaklangAttachedResourceKeyWorkspaceDirectory):
				if path := strings.TrimSpace(data.Value); path != "" {
					ctx.WorkspacePath = filepath.Clean(path)
				}
			case data.HasKey(YaklangAttachedResourceKeyEditorFile):
				if ctx.EditorFile == "" {
					if path := strings.TrimSpace(data.Value); path != "" {
						ctx.EditorFile = filepath.Clean(path)
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

// FormatYaklangEditorContextMarkdown renders editor context for loop timeline import.
func FormatYaklangEditorContextMarkdown(ctx *YaklangEditorContext) string {
	if ctx == nil || ctx.IsEmpty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Yaklang Editor Context\n\n")
	if ctx.HasWorkspace() {
		b.WriteString(fmt.Sprintf("- Workspace: `%s`\n", ctx.WorkspacePath))
	}
	if ctx.HasEditorFile() {
		b.WriteString(fmt.Sprintf("- Open File: `%s`\n", ctx.EditorFile))
	}
	if ctx.HasSelection() {
		sel := ctx.Selection
		if sel.StartLine > 0 && sel.EndLine > 0 {
			b.WriteString(fmt.Sprintf("- Selection Lines: %d-%d\n", sel.StartLine, sel.EndLine))
		}
		if lang := strings.TrimSpace(sel.Language); lang != "" {
			b.WriteString(fmt.Sprintf("- Selection Language: %s\n", lang))
		}
	}
	b.WriteString("\nUse the workspace and open file paths above when deciding where to read, write, or modify Yaklang scripts.\n")
	return strings.TrimSpace(b.String())
}

// EnrichYaklangEditorContextFromUserInput fills EditorFile when the frontend only sent
// workspace/directory_path but the user explicitly named a .yak file in FreeInput.
func EnrichYaklangEditorContextFromUserInput(ctx *YaklangEditorContext, userInput string) {
	if ctx == nil || ctx.HasEditorFile() {
		return
	}
	inferred := InferYaklangEditorFileFromUserInput(userInput, ctx.WorkspacePath)
	if inferred == "" {
		return
	}
	ctx.EditorFile = inferred
	log.Infof("inferred yaklang editor file from user input: %s", inferred)
}

// Match *.yak basenames in natural language (ASCII or CJK delimiters).
var yaklangFileNameInUserInputPattern = regexp.MustCompile(`(?i)(?:^|[^\w.\-])([\w.\-]+\.yak)(?:[^\w.\-]|$)`)

// InferYaklangEditorFileFromUserInput resolves a .yak path mentioned in user text.
func InferYaklangEditorFileFromUserInput(userInput, workspacePath string) string {
	userInput = strings.TrimSpace(userInput)
	workspacePath = strings.TrimSpace(workspacePath)
	if userInput == "" {
		return ""
	}

	matches := yaklangFileNameInUserInputPattern.FindAllStringSubmatch(userInput, -1)
	if len(matches) == 0 {
		return ""
	}
	basename := strings.TrimSpace(matches[len(matches)-1][1])
	if basename == "" {
		return ""
	}

	if workspacePath != "" {
		if found := findYaklangFileByBasename(workspacePath, basename); found != "" {
			return filepath.Clean(found)
		}
		return filepath.Clean(filepath.Join(workspacePath, basename))
	}

	if filepath.IsAbs(basename) || strings.ContainsRune(basename, filepath.Separator) {
		return filepath.Clean(basename)
	}
	return ""
}

func findYaklangFileByBasename(root, basename string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), basename) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// YaklangAttachedInitialCode returns editor-attached selection content for seeding full_code.
func YaklangAttachedInitialCode(ctx *YaklangEditorContext) (code string, ok bool) {
	if ctx == nil || !ctx.HasSelection() {
		return "", false
	}
	code = strings.TrimSpace(ctx.Selection.Content)
	return code, code != ""
}

// ResolveYaklangInitFullCode picks the in-memory buffer for modify_code / delete_code / insert_code.
// When an editor file is attached and diskContent is non-empty, disk wins so line numbers match the
// on-disk file. Otherwise attached selection content is used when present (e.g. unsaved buffer).
func ResolveYaklangInitFullCode(editorCtx *YaklangEditorContext, diskContent string) (code string, fromAttachedSelection bool) {
	if editorCtx != nil && editorCtx.HasEditorFile() {
		if trimmed := strings.TrimSpace(diskContent); trimmed != "" {
			return trimmed, false
		}
	}
	if attachedCode, ok := YaklangAttachedInitialCode(editorCtx); ok {
		return attachedCode, true
	}
	return diskContent, false
}

// YaklangCodeLineBase returns the 0-based offset between full_code line indices and absolute editor
// file line numbers. Non-zero only when full_code is a selection snippet (not the whole file).
func YaklangCodeLineBase(editorCtx *YaklangEditorContext, fullCodeFromSelection bool) int {
	if !fullCodeFromSelection || editorCtx == nil || !editorCtx.HasSelection() {
		return 0
	}
	if editorCtx.Selection.StartLine > 0 {
		return editorCtx.Selection.StartLine - 1
	}
	return 0
}

// ResolveYaklangInitTargetPath picks the init target file path (attachment beats liteforge).
func ResolveYaklangInitTargetPath(editorCtx *YaklangEditorContext, liteforgePath string) (targetPath string, fromAttached bool) {
	if editorCtx != nil && editorCtx.HasEditorFile() {
		return editorCtx.EditorFile, true
	}
	liteforgePath = strings.TrimSpace(liteforgePath)
	if liteforgePath != "" {
		return liteforgePath, false
	}
	return "", false
}
