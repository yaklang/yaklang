package aiskillloader

import (
	"fmt"
	"strings"
	"sync"
)

// testTimelineSink 是测试用的 Timeline 替身: 记录 manager 追加的正文, 并按
// 生产端语义渲染 "已冻结的单一技能块"。
//
// 生产环境由 aicommon.Timeline 提供该实现; 这里只用于验证 manager 的
// "只加载/查看, 不托管正文" 契约。
type testTimelineSink struct {
	mu    sync.Mutex
	items []*testTimelineSinkItem
}

type testTimelineSinkItem struct {
	name    string
	path    string
	body    string
	src     string
	deleted bool
}

func newTestTimelineSink() *testTimelineSink {
	return &testTimelineSink{}
}

func (s *testTimelineSink) AppendSkillContent(name, path, content, source string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if !item.deleted && item.name == name && item.path == path && item.body == content {
			return false
		}
	}
	s.items = append(s.items, &testTimelineSinkItem{name: name, path: path, body: content, src: source})
	return true
}

func (s *testTimelineSink) DeleteSkillContent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for _, item := range s.items {
		if item.name == name && !item.deleted {
			item.deleted = true
			removed = true
		}
	}
	return removed
}

func (s *testTimelineSink) DeleteSkillCatalog() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for _, item := range s.items {
		if item.name == "__skill_catalog__" && !item.deleted {
			item.deleted = true
			removed = true
		}
	}
	return removed
}

func (s *testTimelineSink) HasVisibleSkillContent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.name == name && !item.deleted {
			return true
		}
	}
	return false
}

func (s *testTimelineSink) VisibleSkillContentNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	seen := make(map[string]bool)
	for _, item := range s.items {
		if !item.deleted && item.name != "__skill_catalog__" && !seen[item.name] {
			seen[item.name] = true
			names = append(names, item.name)
		}
	}
	return names
}

func (s *testTimelineSink) HasSkillContentHistory(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.name == name {
			return true
		}
	}
	return false
}

func (s *testTimelineSink) AppendSkillCatalog(rendered string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if !item.deleted && item.name == "__skill_catalog__" && item.body == rendered {
			return false
		}
	}
	s.items = append(s.items, &testTimelineSinkItem{
		name: "__skill_catalog__", body: rendered, src: "catalog",
	})
	return true
}

func (s *testTimelineSink) RenderSkillContentForPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var parts []string
	for _, item := range s.items {
		if item.deleted {
			continue
		}
		if item.name == "__skill_catalog__" {
			parts = append(parts, item.body)
			continue
		}
		header := fmt.Sprintf("=== Skill: %s ===", item.name)
		if item.path != "" && item.path != SkillMainBodyPath && item.path != "SKILL.md" {
			header = fmt.Sprintf("=== Skill: %s (%s) ===", item.name, item.path)
		}
		parts = append(parts, header+"\n"+item.body)
	}
	if len(parts) == 0 {
		return ""
	}
	return "<|SKILLS_CONTEXT_test|>\n== Currently Loaded Skills ==\n\n" + strings.Join(parts, "\n") + "\n<|SKILLS_CONTEXT_END_test|>"
}

func (s *testTimelineSink) appendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}
