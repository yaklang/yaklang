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

const (
	DefaultSessionSkillAvailablePageSize = 100
	MaxSessionSkillAvailablePageSize     = 500
)

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

// SessionSkillList 是完整、不分页的技能集合，用于已加载状态和旧版清单。
type SessionSkillList struct {
	Skills []CapabilityInventoryItem `json:"skills"`
	Total  int                       `json:"total"`
}

// SessionSkillAvailablePage 只分页可加载且尚未加载的技能。已加载技能由
// skill_loaded_sync 独立返回，不占用这里的页码。Query 非空时表示本次结果经过
// 名称和描述过滤。
type SessionSkillAvailablePage struct {
	Skills   []CapabilityInventoryItem `json:"skills"`
	Query    string                    `json:"query,omitempty"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int                       `json:"total"`
	HasMore  bool                      `json:"has_more"`
}

// BuildSessionLoadedSkillInventory 返回 Timeline 中全部已加载技能，不分页。
func BuildSessionLoadedSkillInventory(mgr *aiskillloader.SkillsContextManager) *SessionSkillList {
	result := &SessionSkillList{Skills: []CapabilityInventoryItem{}}
	if mgr == nil {
		return result
	}
	items := loadedSkillInventory(mgr)
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Skills = append(result.Skills, BuildCapabilityInventorySkillItems([]CapabilityInventoryNamedItem{items[name]})...)
	}
	result.Total = len(result.Skills)
	return result
}

// BuildSessionAvailableSkillInventory 分页返回可加载且未加载的技能。query 为空时
// 返回全部未加载技能；非空时只保留名称或描述匹配的技能。过滤发生在完整目录上，
// 不使用 prompt token 预算，也不使用数据库搜索的默认条数上限。文件系统、压缩包、
// 自定义 loader 和数据库 skillmd 先合并、去重、排除已加载，再按名称排序切片。
// 目录不变时同一页稳定；加载、卸载或技能源变化后，前端应回到第 1 页。
func BuildSessionAvailableSkillInventory(mgr *aiskillloader.SkillsContextManager, query string, page, pageSize int) (*SessionSkillAvailablePage, error) {
	query = strings.TrimSpace(query)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = DefaultSessionSkillAvailablePageSize
	}
	if pageSize > MaxSessionSkillAvailablePageSize {
		pageSize = MaxSessionSkillAvailablePageSize
	}
	result := &SessionSkillAvailablePage{
		Skills: []CapabilityInventoryItem{}, Query: query, Page: page, PageSize: pageSize,
	}
	if mgr == nil {
		return result, nil
	}

	loaded := loadedSkillInventory(mgr)
	metas, err := allLoadableSkillMetas(mgr)
	if err != nil {
		return nil, err
	}
	available := make(map[string]CapabilityInventoryNamedItem)
	for _, meta := range metas {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		if _, isLoaded := loaded[meta.Name]; isLoaded {
			continue
		}
		if query != "" && !aiskillloader.SkillMetaMatchesQuery(meta, query) {
			continue
		}
		available[meta.Name] = skillMetaToInventoryItem(meta, CapabilityInventorySkillLoadMetadata)
	}

	names := make([]string, 0, len(available))
	for name := range available {
		names = append(names, name)
	}
	sort.Strings(names)
	result.Total = len(names)
	start := (page - 1) * pageSize
	if start >= result.Total {
		result.Skills = []CapabilityInventoryItem{}
		return result, nil
	}
	end := start + pageSize
	if end > result.Total {
		end = result.Total
	}
	result.Skills = make([]CapabilityInventoryItem, 0, end-start)
	for _, name := range names[start:end] {
		result.Skills = append(result.Skills, BuildCapabilityInventorySkillItems([]CapabilityInventoryNamedItem{available[name]})...)
	}
	result.HasMore = end < result.Total
	return result, nil
}

func loadedSkillInventory(mgr *aiskillloader.SkillsContextManager) map[string]CapabilityInventoryNamedItem {
	result := make(map[string]CapabilityInventoryNamedItem)
	if mgr == nil {
		return result
	}
	for _, skill := range mgr.GetCurrentSelectedSkills() {
		if skill == nil || strings.TrimSpace(skill.Name) == "" {
			continue
		}
		meta := skill.SkillMeta
		if meta == nil {
			meta = &aiskillloader.SkillMeta{Name: skill.Name}
		}
		item := skillMetaToInventoryItem(meta, CapabilityInventorySkillLoadLoaded)
		item.TokenCount = skill.TokenCount
		result[skill.Name] = item
	}
	return result
}

func allLoadableSkillMetas(mgr *aiskillloader.SkillsContextManager) ([]*aiskillloader.SkillMeta, error) {
	if mgr == nil {
		return nil, nil
	}
	loader := mgr.GetLoader()
	if loader == nil {
		return nil, nil
	}
	if pager, ok := loader.(aiskillloader.SkillMetaPager); ok {
		metas, _, err := pager.ListSkillMetas(0, 0)
		return metas, err
	}
	return loader.AllSkillMetas(), nil
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
