package aiskillloader

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"
)

// forced_skills.go 实现「用户强制加载 SKILL」专用容器.
//
// 设计: 用户强制加载 (来源: load_skill sync 事件 / EnabledCapabilities) 的 SKILL
// 拥有最高优先级 —— 满内容渲染 (无折叠), 进入 prompt 的 frozen_block 顶部 (比
// SKILLS_CONTEXT 目录层级还高). 这与 AI 意图驱动的 LoadAutoSkill (进 SemiDynamic 2
// 尾部, LRU 折叠) 形成职责清晰的分流.
//
// 关键词: ForcedSkillRegistry, 用户强制加载, frozen_block, 满内容, 最高优先级

// forcedSkillBoundaryTagName / Nonce 是 forced skill 段的成对定界标记.
// 强制 stable nonce, 让该段在内容不变时字节稳定, 利于 frozen_block 缓存命中.
const (
	forcedSkillBoundaryTagName = "USER_FORCED_SKILL"
	forcedSkillBoundaryNonce   = "skills"
)

// ForcedSkillRegistry 维护用户强制加载的 SKILL (满内容, 进 frozen_block).
type ForcedSkillRegistry struct {
	mu       sync.RWMutex
	skills   *omap.OrderedMap[string, *LoadedSkill]
	loadedAt map[string]time.Time
}

// NewForcedSkillRegistry 创建空的 forced skill 注册表.
func NewForcedSkillRegistry() *ForcedSkillRegistry {
	return &ForcedSkillRegistry{
		skills:   omap.NewOrderedMap[string, *LoadedSkill](map[string]*LoadedSkill{}),
		loadedAt: make(map[string]time.Time),
	}
}

// Add 写入一个满内容 skill. 覆盖同名旧条目. 返回 true 表示新增 (之前不存在).
func (f *ForcedSkillRegistry) Add(name string, skill *LoadedSkill) bool {
	if f == nil || strings.TrimSpace(name) == "" || skill == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, existed := f.skills.Get(name)
	f.skills.Set(name, skill)
	f.loadedAt[name] = time.Now()
	return !existed
}

// Remove 移除一个 forced skill, 返回是否曾存在.
func (f *ForcedSkillRegistry) Remove(name string) bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.skills.Have(name) {
		return false
	}
	f.skills.Delete(name)
	delete(f.loadedAt, name)
	return true
}

// Has 返回是否包含指定 forced skill.
func (f *ForcedSkillRegistry) Has(name string) bool {
	if f == nil {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.skills.Have(name)
}

// IsEmpty 返回 forced skill 是否为空.
func (f *ForcedSkillRegistry) IsEmpty() bool {
	if f == nil {
		return true
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.skills.Len() == 0
}

// Names 返回 (按名排序的) forced skill 名列表.
func (f *ForcedSkillRegistry) Names() []string {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	names := make([]string, 0, f.skills.Len())
	f.skills.ForEach(func(name string, _ *LoadedSkill) bool {
		names = append(names, name)
		return true
	})
	sort.Strings(names)
	return names
}

// Render 渲染所有 forced skill 为 prompt 段落 (满内容, 无折叠).
// 输出形如:
//
//	<|USER_FORCED_SKILL_skills|>
//	# User Forced Skills
//	The following skills were explicitly loaded by the user. Highest priority.
//
//	=== Skill: X ===
//	<满内容 SKILL.md>
//	=== Skill: Y ===
//	<满内容 SKILL.md>
//	<|USER_FORCED_SKILL_END_skills|>
//
// 空注册表返回空串 (段不渲染).
func (f *ForcedSkillRegistry) Render() string {
	if f == nil || f.IsEmpty() {
		return ""
	}
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 按名排序保证字节稳定 (利于 frozen_block 缓存).
	type entry struct {
		name  string
		skill *LoadedSkill
	}
	items := make([]entry, 0, f.skills.Len())
	f.skills.ForEach(func(name string, skill *LoadedSkill) bool {
		items = append(items, entry{name: name, skill: skill})
		return true
	})
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|%s_%s|>\n", forcedSkillBoundaryTagName, forcedSkillBoundaryNonce))
	buf.WriteString("# User Forced Skills\n")
	buf.WriteString("The following skills were explicitly loaded by the user and have the HIGHEST priority. They are always fully visible.\n\n")
	for _, e := range items {
		buf.WriteString(renderForcedSkillFull(e.name, e.skill))
		buf.WriteString("\n")
	}
	buf.WriteString(fmt.Sprintf("<|%s_END_%s|>", forcedSkillBoundaryTagName, forcedSkillBoundaryNonce))
	return buf.String()
}

// renderForcedSkillFull 渲染单个 forced skill 的满内容: 元信息 + 文件树 + SKILL.md 全文.
func renderForcedSkillFull(name string, skill *LoadedSkill) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Skill: %s ===\n", name))
	if skill != nil && skill.Meta != nil {
		buf.WriteString(skill.Meta.BriefString())
	}
	if skill != nil && skill.FileSystem != nil {
		buf.WriteString("\nFile Tree:\n")
		buf.WriteString(RenderFileSystemTreeFull(skill.FileSystem))
	}
	if skill != nil && skill.SkillMDContent != "" {
		buf.WriteString("\nSKILL.md:\n")
		buf.WriteString(skill.SkillMDContent)
	}
	return buf.String()
}
