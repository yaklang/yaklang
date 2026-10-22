package aicommon

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils"
)

// SessionSkillDetail 是 session 级 SKILL 详情。已加载技能返回 Timeline 中当前
// 可见的正文及分片；未加载技能返回 SKILL.md 和文件系统预览。
//
// FileSystemAvailable 是后续按需读取资源文件的预留边界。FileSystem 本身是运行时
// 句柄，不进入 JSON 响应，也不持久化到 Timeline。
type SessionSkillDetail struct {
	Name                string                           `json:"name"`
	Loaded              bool                             `json:"loaded"`
	Meta                *aiskillloader.SkillMeta         `json:"meta,omitempty"`
	PromptContent       string                           `json:"prompt_content,omitempty"`
	Parts               []aiskillloader.SkillContextPart `json:"parts,omitempty"`
	SkillMDContent      string                           `json:"skill_md,omitempty"`
	FileSystemAvailable bool                             `json:"file_system_available"`
	FileTree            string                           `json:"file_tree,omitempty"`
}

// BuildSessionSkillInventory 返回当前 Timeline 中已加载的技能以及 loader 中可加载
// 的技能。返回项沿用 capability_inventory 的扁平结构，调用方可按 Stage 区分。
func BuildSessionSkillInventory(mgr *aiskillloader.SkillsContextManager) []CapabilityInventoryItem {
	if mgr == nil {
		return nil
	}
	return BuildCapabilityInventorySkillItems(BuildInventorySkillsFromManager(mgr))
}

// BuildSessionSkillDetail 查询指定技能。加载状态只由 Timeline 决定，因此即使技能
// 来自另一个 loop 的专用 loader，也能返回当前 prompt 可见内容。
func BuildSessionSkillDetail(mgr *aiskillloader.SkillsContextManager, name string) (*SessionSkillDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, utils.Error("skill name is empty")
	}
	if mgr == nil {
		return nil, utils.Error("skills context manager is not configured")
	}

	for _, selected := range mgr.GetCurrentSelectedSkills() {
		if selected == nil || selected.Name != name {
			continue
		}
		parts := mgr.VisibleSkillParts(name)
		return &SessionSkillDetail{
			Name:                name,
			Loaded:              true,
			Meta:                selected.SkillMeta,
			PromptContent:       RenderSessionSkillPromptContent(name, parts),
			Parts:               parts,
			FileSystemAvailable: managerHasSkillFileSystem(mgr, name),
		}, nil
	}

	loader := mgr.GetLoader()
	if loader == nil {
		return nil, utils.Errorf("skill %q is not available", name)
	}
	loaded, err := loader.LoadSkill(name)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load skill %q", name)
	}
	if loaded == nil || loaded.Meta == nil {
		return nil, utils.Errorf("skill %q has no metadata", name)
	}
	detail := &SessionSkillDetail{
		Name:                name,
		Loaded:              false,
		Meta:                loaded.Meta,
		SkillMDContent:      loaded.SkillMDContent,
		FileSystemAvailable: loaded.FileSystem != nil,
	}
	if loaded.FileSystem != nil {
		detail.FileTree = aiskillloader.RenderFileSystemTreeFull(loaded.FileSystem)
	}
	return detail, nil
}

func managerHasSkillFileSystem(mgr *aiskillloader.SkillsContextManager, name string) bool {
	loader := mgr.GetLoader()
	if loader == nil {
		return false
	}
	fs, err := loader.GetFileSystem(name)
	return err == nil && fs != nil
}

// RenderSessionSkillPromptContent 把 Timeline 可见分片组织成与技能块一致的可读
// 内容：metadata 在前，文件范围按路径和起始行排序。
func RenderSessionSkillPromptContent(name string, parts []aiskillloader.SkillContextPart) string {
	name = strings.TrimSpace(name)
	if name == "" || len(parts) == 0 {
		return ""
	}
	sorted := append([]aiskillloader.SkillContextPart(nil), parts...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind == aiskillloader.SkillContentKindMetadata
		}
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine < sorted[j].StartLine
		}
		return sorted[i].EndLine < sorted[j].EndLine
	})

	var content strings.Builder
	fmt.Fprintf(&content, "=== Skill: %s ===\n", name)
	for _, part := range sorted {
		if strings.TrimSpace(part.Content) == "" {
			continue
		}
		if part.Kind == aiskillloader.SkillContentKindMetadata {
			content.WriteString("[metadata]\n")
		} else {
			path := part.Path
			if path == "" {
				path = "SKILL.md"
			}
			fmt.Fprintf(&content, "[%s", path)
			if part.EndLine > 0 {
				fmt.Fprintf(&content, ":%d-%d/%d", part.StartLine, part.EndLine, part.Total)
				if part.ReachedEnd {
					content.WriteString(" END")
				}
			}
			content.WriteString("]\n")
		}
		content.WriteString(strings.TrimSpace(part.Content))
		content.WriteString("\n")
	}
	return strings.TrimSpace(content.String())
}
