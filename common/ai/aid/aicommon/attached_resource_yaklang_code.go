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
// Role separation:
//   - Writable delivery (open editor script):
//       Type=code, Key=file_path, Value=.yak absolute path
//   - Read-only reference / @mention (do NOT write):
//       Type=file, Key=file_path, Value=any reference path
//   - Workspace:
//       Type=file, Key=directory_path, Value=workspace absolute path
//   - Selection chip:
//       Type=selected, Key=content, Value=AttachedCodeSelection JSON
//
// Legacy: some clients still send the open .yak as Type=file Key=file_path.
// Backend accepts that only when no Type=code is present and the path ends with .yak.
//
// @mention reference files (e.g. .md reports) must NOT become editor_file_path /
// yaklang_code_change delivery targets — they are context-only via ContextProvider.
//
// yaklang_code_change delivery (backend → frontend):
//   - op=patch: code.content is the changed fragment; code.patch describes how to apply it
//     (kind: line_range | snippet | insert | delete | full; optional old_snippet; 1-based absolute file lines).
//   - op=create|replace: code.content is the full script (create for new files; replace on loop flush).
//   - code.version / code.change_id: monotonic dedup for multi-round edits.
//
// Other loops use domain-specific keys (e.g. code_security_audit uses code_audit_target_path).
// Frontend strings must match CONTEXT_PROVIDER_* and AttachedResource* constants.

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
	return c != nil && IsYaklangScriptDeliveryPath(c.EditorFile)
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

// IsYaklangScriptDeliveryPath reports whether path is a Yaklang script suitable for
// editor delivery (replace / seed). Non-.yak paths (e.g. @mention .md reports) are rejected.
func IsYaklangScriptDeliveryPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	return strings.EqualFold(filepath.Ext(path), ".yak")
}

// :mention[path]{mentionId="path"} or :mention[path]
var yaklangMentionPathPattern = regexp.MustCompile(`(?i):mention\[([^\]]+)\](?:\{[^}]*mentionId="([^"]*)"[^}]*\})?`)

// ExtractMentionPathsFromUserInput collects absolute/relative paths from :mention[...] chips.
func ExtractMentionPathsFromUserInput(userInput string) []string {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return nil
	}
	matches := yaklangMentionPathPattern.FindAllStringSubmatch(userInput, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		// Mention display may escape underscores as \_
		raw = strings.ReplaceAll(raw, `\_`, "_")
		cleaned := filepath.Clean(raw)
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	for _, m := range matches {
		if len(m) > 1 {
			add(m[1])
		}
		if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
			add(m[2])
		}
	}
	return out
}

func pathInMentionSet(path string, mentionPaths []string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || len(mentionPaths) == 0 {
		return false
	}
	pathLower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	for _, m := range mentionPaths {
		m = filepath.Clean(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if strings.EqualFold(path, m) || pathLower == strings.ToLower(m) {
			return true
		}
		// mention path may use escaped underscores or partial display forms
		if base != "" && strings.EqualFold(base, filepath.Base(m)) {
			return true
		}
	}
	return false
}

func isPathUnderWorkspace(path, workspace string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if path == "" || workspace == "" {
		return false
	}
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pickYaklangEditorFile chooses the best .yak delivery path from candidates.
// Prefer workspace-scoped .yak files; never pick mention-only reference paths.
func pickYaklangEditorFile(candidates []string, workspace string, mentionPaths []string) string {
	var yakCandidates []string
	for _, c := range candidates {
		c = filepath.Clean(strings.TrimSpace(c))
		if !IsYaklangScriptDeliveryPath(c) {
			continue
		}
		if pathInMentionSet(c, mentionPaths) {
			continue
		}
		yakCandidates = append(yakCandidates, c)
	}
	if len(yakCandidates) == 0 {
		return ""
	}
	if workspace != "" {
		workspace = filepath.Clean(workspace)
		for _, c := range yakCandidates {
			if isPathUnderWorkspace(c, workspace) {
				return c
			}
		}
	}
	return yakCandidates[0]
}

// ParseYaklangEditorContextFromAttached builds editor context from AttachedResourceInfo payloads.
// Type=code Key=file_path is the canonical writable delivery slot.
// Type=file Key=file_path is reference/@ context and must not become EditorFile unless legacy fallback.
func ParseYaklangEditorContextFromAttached(attachedDatas []*AttachedResource) *YaklangEditorContext {
	return ParseYaklangEditorContextFromAttachedWithUserInput(attachedDatas, "")
}

// ParseYaklangEditorContextFromAttachedWithUserInput is like ParseYaklangEditorContextFromAttached
// but also excludes paths appearing in FreeInput :mention[...] chips.
func ParseYaklangEditorContextFromAttachedWithUserInput(attachedDatas []*AttachedResource, userInput string) *YaklangEditorContext {
	ctx := &YaklangEditorContext{}
	mentionPaths := ExtractMentionPathsFromUserInput(userInput)
	var codeDeliveryCandidates []string
	var legacyFileYakCandidates []string
	var selectionPath string

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
					selectionPath = filepath.Clean(path)
				}
			}
		case data.HasType(AttachedResourceTypeCode):
			if data.HasKey(CONTEXT_PROVIDER_KEY_FILE_PATH) {
				if path := strings.TrimSpace(data.Value); path != "" {
					codeDeliveryCandidates = append(codeDeliveryCandidates, filepath.Clean(path))
				}
			}
		case data.HasType(AttachedResourceTypeFile):
			switch {
			case data.HasKey(CONTEXT_PROVIDER_KEY_DIRECTORY_PATH):
				if path := strings.TrimSpace(data.Value); path != "" {
					ctx.WorkspacePath = filepath.Clean(path)
				}
			case data.HasKey(CONTEXT_PROVIDER_KEY_FILE_PATH):
				path := strings.TrimSpace(data.Value)
				if path == "" {
					continue
				}
				cleaned := filepath.Clean(path)
				// Type=file is read-only reference context. Keep .yak only as legacy delivery fallback.
				if IsYaklangScriptDeliveryPath(path) {
					legacyFileYakCandidates = append(legacyFileYakCandidates, cleaned)
				} else {
					// Mention-only attachments often reuse file_path; track for exclusion even without FreeInput parse.
					mentionPaths = append(mentionPaths, cleaned)
				}
			}
		}
	}

	switch {
	case len(codeDeliveryCandidates) > 0:
		// Type=code is the authoritative writable slot; do not exclude via mention chips.
		ctx.EditorFile = pickYaklangEditorFile(codeDeliveryCandidates, ctx.WorkspacePath, nil)
	case IsYaklangScriptDeliveryPath(selectionPath) && !pathInMentionSet(selectionPath, mentionPaths):
		ctx.EditorFile = filepath.Clean(selectionPath)
	case len(legacyFileYakCandidates) > 0:
		ctx.EditorFile = pickYaklangEditorFile(legacyFileYakCandidates, ctx.WorkspacePath, mentionPaths)
		if ctx.EditorFile != "" {
			log.Infof("legacy Type=file file_path used as yak delivery target; prefer Type=code: %s", ctx.EditorFile)
		}
	}

	if ctx.IsEmpty() {
		return nil
	}
	return ctx
}

// CollectYaklangDeliveryPathsFromAttached returns writable delivery paths (Type=code / resolved EditorFile).
func CollectYaklangDeliveryPathsFromAttached(attachedDatas []*AttachedResource) []string {
	ctx := ParseYaklangEditorContextFromAttached(attachedDatas)
	if ctx == nil || !ctx.HasEditorFile() {
		return nil
	}
	return []string{filepath.Clean(ctx.EditorFile)}
}

// FilterAttachedResourcesExcludeYaklangDelivery drops Type=code delivery entries so they are not
// mixed into generic attached_* timeline as reference files. Type=file references are kept.
func FilterAttachedResourcesExcludeYaklangDelivery(attachedDatas []*AttachedResource) []*AttachedResource {
	if len(attachedDatas) == 0 {
		return attachedDatas
	}
	out := make([]*AttachedResource, 0, len(attachedDatas))
	for _, data := range attachedDatas {
		if data == nil {
			continue
		}
		if data.HasType(AttachedResourceTypeCode) {
			continue
		}
		out = append(out, data)
	}
	return out
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
		b.WriteString(fmt.Sprintf("- Open File (writable Type=code): `%s`\n", ctx.EditorFile))
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
	b.WriteString("Type=file attachments are reference-only; do not overwrite them.\n")
	return strings.TrimSpace(b.String())
}

// EnrichYaklangEditorContextFromUserInput fills EditorFile when the frontend only sent
// workspace/directory_path but the user explicitly named a .yak file in FreeInput.
// Non-.yak EditorFile values (e.g. @mention .md mis-bound as file_path) are cleared first.
func EnrichYaklangEditorContextFromUserInput(ctx *YaklangEditorContext, userInput string) {
	if ctx == nil {
		return
	}
	if ctx.EditorFile != "" && !IsYaklangScriptDeliveryPath(ctx.EditorFile) {
		log.Infof("clearing non-yak editor file from attachments: %s", ctx.EditorFile)
		ctx.EditorFile = ""
	}
	if ctx.HasEditorFile() {
		return
	}
	inferred := InferYaklangEditorFileFromUserInput(userInput, ctx.WorkspacePath)
	if inferred == "" || !IsYaklangScriptDeliveryPath(inferred) {
		return
	}
	// Do not treat mention-chip paths as delivery targets.
	if pathInMentionSet(inferred, ExtractMentionPathsFromUserInput(userInput)) {
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
// Non-.yak targets never seed from disk (avoids loading markdown reports into full_code).
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
// Non-.yak attached paths are ignored so markdown @mention files never become seed targets.
func ResolveYaklangInitTargetPath(editorCtx *YaklangEditorContext, liteforgePath string) (targetPath string, fromAttached bool) {
	if editorCtx != nil && editorCtx.HasEditorFile() {
		return filepath.Clean(editorCtx.EditorFile), true
	}
	liteforgePath = strings.TrimSpace(liteforgePath)
	if IsYaklangScriptDeliveryPath(liteforgePath) {
		return filepath.Clean(liteforgePath), false
	}
	return "", false
}
