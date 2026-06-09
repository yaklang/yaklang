package loop_yaklangcode

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// YaklangEditorContext carries IDE workspace state from frontend attached resources.
type YaklangEditorContext struct {
	WorkspacePath string
	EditorFile    string
	Selection     *aicommon.AttachedCodeSelection
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

func yaklangEditorContextFromAttached(attachedDatas []*aicommon.AttachedResource) *YaklangEditorContext {
	ctx := &YaklangEditorContext{}
	for _, data := range attachedDatas {
		if data == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(data.Type)) {
		case aicommon.AttachedResourceTypeSelected:
			switch data.Key {
			case aicommon.AttachedResourceKeyContent:
				if sel, ok := aicommon.ParseAttachedCodeSelection(data); ok {
					ctx.Selection = sel
					if path := strings.TrimSpace(sel.Path); path != "" {
						ctx.EditorFile = filepath.Clean(path)
					}
				}
			}
		case AttachedResourceTypeFile:
			switch data.Key {
			case AttachedResourceKeyWorkspaceDirectory:
				if path := strings.TrimSpace(data.Value); path != "" {
					ctx.WorkspacePath = filepath.Clean(path)
				}
			case AttachedResourceKeyEditorFile:
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

func formatYaklangEditorContextMarkdown(ctx *YaklangEditorContext) string {
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

func initYaklangEditorContextFromAttached(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	attachedDatas []*aicommon.AttachedResource,
) *YaklangEditorContext {
	ctx := yaklangEditorContextFromAttached(attachedDatas)
	if ctx == nil {
		return nil
	}

	if ctx.HasWorkspace() {
		loop.Set("workspace_path", ctx.WorkspacePath)
	}
	if ctx.HasEditorFile() {
		loop.Set("editor_file_path", ctx.EditorFile)
	}

	payload := formatYaklangEditorContextMarkdown(ctx)
	if payload != "" {
		r.AddToTimeline("yaklang_editor_context", payload)
		r.AddToTimeline(
			"import notice",
			"yaklang_editor_context records the user's open workspace and file; prefer these paths over guessed paths.",
		)
	}
	return ctx
}
