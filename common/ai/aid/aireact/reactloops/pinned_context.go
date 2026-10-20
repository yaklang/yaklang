package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// WithPinnedSkills snapshots the complete SKILL.md and file tree at loop creation.
// These loop-owned instructions never enter Timeline compression or view paging.
// Referenced resources remain available through the normal Skill loader.
func WithPinnedSkills(names ...string) ReActLoopOption {
	return func(r *ReActLoop) { r.pinnedSkillNames = append(r.pinnedSkillNames, names...) }
}

// WithPinnedTools snapshots enabled tool schemas outside the recent-tool LRU.
// Pinning loads definitions only and never enables or executes a tool.
func WithPinnedTools(names ...string) ReActLoopOption {
	return func(r *ReActLoop) { r.pinnedToolNames = append(r.pinnedToolNames, names...) }
}

// WithStaticInstruction adds trusted host instructions to the high-static system
// section. It replaces an earlier focus instruction, preserving core system rules.
// User attachments and retrieved documents must not be passed as instructions.
func WithStaticInstruction(instruction string) ReActLoopOption {
	return func(r *ReActLoop) { r.staticInstruction = instruction }
}

// HasPinnedContext reports whether this loop owns an explicit focus overlay.
func (r *ReActLoop) HasPinnedContext() bool {
	return r != nil && (len(r.pinnedSkillNames)+len(r.pinnedToolNames) > 0 || r.staticInstruction != "")
}

// HasToolSchema includes the loop's non-evictable definitions. Execution still
// resolves the tool through the manager and its current authorization policy.
func (r *ReActLoop) HasToolSchema(name string) bool {
	if r.pinnedToolEntries[name] != nil {
		return true
	}
	return r.config.GetAiToolManager() != nil && r.config.GetAiToolManager().IsRecentlyUsedTool(name)
}

func (r *ReActLoop) preparePinnedContext() error {
	r.pinnedSkillNames = uniqueToolSchemaNames(r.pinnedSkillNames)
	r.pinnedToolNames = uniqueToolSchemaNames(r.pinnedToolNames)
	if !r.HasPinnedContext() {
		return nil
	}
	var body strings.Builder
	if len(r.pinnedSkillNames) > 0 {
		var loader aiskillloader.SkillLoader
		if r.skillsContextManager != nil {
			loader = r.skillsContextManager.GetLoader()
		} else if cfg, ok := r.config.(*aicommon.Config); ok {
			loader = cfg.GetSkillLoader()
		}
		missing := false
		for _, name := range r.pinnedSkillNames {
			found := false
			if loader != nil {
				for _, meta := range loader.AllSkillMetas() {
					found = found || (meta != nil && meta.Name == name)
				}
			}
			missing = missing || !found
		}
		if missing {
			provider, ok := r.invoker.(interface {
				GetBuiltinSkillLoader(aiskillloader.SkillLoader, ...string) (aiskillloader.SkillLoader, error)
			})
			if !ok {
				return fmt.Errorf("focus %q: required Skill loader is unavailable", r.loopName)
			}
			var err error
			loader, err = provider.GetBuiltinSkillLoader(loader, r.pinnedSkillNames...)
			if err != nil {
				return err
			}
		}
		if r.skillsContextManager == nil || missing {
			WithSkillLoader(loader)(r)
		}
		for _, name := range r.pinnedSkillNames {
			skill, err := loader.LoadSkill(name)
			if err != nil {
				return fmt.Errorf("focus %q: load pinned Skill %q: %w", r.loopName, name, err)
			}
			if skill == nil || strings.TrimSpace(skill.SkillMDContent) == "" {
				return fmt.Errorf("focus %q: pinned Skill %q has no content", r.loopName, name)
			}
			fmt.Fprintf(&body, "\n## Pinned Skill: %s\n%s\n\nFile Tree:\n%s\n", name,
				skill.SkillMDContent, aiskillloader.RenderFileSystemTreeFull(skill.FileSystem))
		}
	}
	r.pinnedToolEntries = make(map[string]*buildinaitools.RecentToolEntry)
	if len(r.pinnedToolNames) > 0 {
		manager := r.config.GetAiToolManager()
		if manager == nil {
			return fmt.Errorf("focus %q: tool manager is unavailable", r.loopName)
		}
		tools, err := manager.GetEnableTools()
		if err != nil {
			return err
		}
		enabled := make(map[string]*aitool.Tool, len(tools))
		for _, tool := range tools {
			if tool != nil {
				enabled[tool.Name] = tool
			}
		}
		for _, name := range r.pinnedToolNames {
			tool := enabled[name]
			if tool == nil || buildinaitools.IsMCPPendingStub(tool) ||
				(buildinaitools.IsMCPRelatedToolName(name) && !aicommon.IsMCPServersAllowedConfig(r.config)) {
				return fmt.Errorf("focus %q: pinned tool %q is unavailable or disabled", r.loopName, name)
			}
			entry := buildinaitools.NewRecentToolEntry(tool, r.functionCallMode)
			r.pinnedToolEntries[name] = entry
			body.WriteString("\n" + buildinaitools.RenderRecentToolEntryForPromotion(entry) + "\n")
			r.syncRecentToolParamAITagFields(manager.GetRecentToolParamNamesByTool(name))
		}
	}
	// Reject an oversized preset atomically rather than silently losing required
	// instructions. This fixed budget is independent of ordinary cache pressure.
	if aicommon.TokenCountExceeds(body.String()+r.staticInstruction, 16*1024) {
		return fmt.Errorf("focus %q: pinned context exceeds the 16384-token budget", r.loopName)
	}
	if body.Len() > 0 {
		r.pinnedContext = "# Pinned Focus Context\nDefinitions below remain available throughout this mode; reuse them without loading again. Tool schemas do not imply execution.\n" + body.String()
	}
	return nil
}
