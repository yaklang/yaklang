package aicommon

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils"
)

const SessionSkillResourceURLPath = "/api/ai/skill"

// SessionSkillDetail 是 session 级 SKILL 详情。已加载技能返回 Timeline 中当前
// 可见的正文；未加载技能返回 SKILL.md 和文件树。
// URL 标识 session 中的技能资源，后续可由服务端统一解析 Timeline、文件系统或
// 数据库来源，前端不需要理解技能的实际存储位置。
type SessionSkillDetail struct {
	Name           string                   `json:"name"`
	Loaded         bool                     `json:"loaded"`
	Meta           *aiskillloader.SkillMeta `json:"meta,omitempty"`
	PromptContent  string                   `json:"prompt_content,omitempty"`
	SkillMDContent string                   `json:"skill_md,omitempty"`
	FileTree       string                   `json:"file_tree,omitempty"`
	URL            string                   `json:"url,omitempty"`
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
			Name:          name,
			Loaded:        true,
			Meta:          selected.SkillMeta,
			PromptContent: RenderSessionSkillPromptContent(name, parts),
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
		Name:           name,
		Loaded:         false,
		Meta:           loaded.Meta,
		SkillMDContent: loaded.SkillMDContent,
	}
	if loaded.FileSystem != nil {
		detail.FileTree = aiskillloader.RenderFileSystemTreeFull(loaded.FileSystem)
	}
	return detail, nil
}

// BuildSessionSkillResourceURL 生成来源无关的 session 技能资源 URL。当前只定义
// 稳定访问契约，具体 HTTP handler 后续通过同一路径统一读取技能。
func BuildSessionSkillResourceURL(cfg *Config, skillName string) string {
	skillName = strings.TrimSpace(skillName)
	if cfg == nil || skillName == "" {
		return ""
	}
	query := url.Values{}
	query.Set("skill_name", skillName)
	if sessionID := strings.TrimSpace(cfg.GetPersistentSessionID()); sessionID != "" {
		query.Set("session_id", sessionID)
	}
	if runtimeID := strings.TrimSpace(cfg.GetRuntimeId()); runtimeID != "" {
		query.Set("runtime_id", runtimeID)
	}
	return SessionSkillResourceURLPath + "?" + query.Encode()
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
