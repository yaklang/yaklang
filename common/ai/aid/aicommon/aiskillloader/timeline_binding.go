package aiskillloader

import (
	"strings"
	"sync"
)

// SkillContentSource* 区分技能正文的来源, 仅用于压缩提示与观测; 它不是渲染分组,
// 也不再对应独立的展示块或独立的压缩保护。
const (
	SkillContentSourceAuto   = "auto"
	SkillContentSourceForced = "forced"
	SkillContentSourceManual = "manual"

	SkillContentKindMetadata = "metadata"
	SkillContentKindContent  = "content"
)

// SkillContextPart 是 Timeline 中仍可见的一个技能部分。
type SkillContextPart struct {
	Name       string
	Kind       string
	Path       string
	StartLine  int
	EndLine    int
	Total      int
	ReachedEnd bool
	// TokenCount 是该部分当前可见正文的 token 之和，不包含渲染标签或目录。
	TokenCount int
}

// SkillContentSink 是 aiskillloader 侧对“运行时上下文 Timeline”的最小依赖。
// 具体实现由 aicommon 提供, 避免 aiskillloader 反向 import aicommon 造成循环依赖。
//
// 语义:
//   - AppendSkillMetadata 追加技能的说明、关联技能和文件树。
//   - AppendSkillFileContent 追加一个文件的一段行范围, 不替换其它范围。
//   - AppendSkillCatalog 追加意图识别选出的技能目录。
//   - DeleteSkillContent 退休该技能当前可见的正文, 不触碰冻结边界。
//   - VisibleSkillParts 枚举目标 Timeline 中仍可见的元信息、文件范围及正文 token 数,
//     技能是否仍然可见由它派生, 不单独维护查询入口。
//   - HasSkillContentHistory 判断是否存在加载历史, 包括已经退休的正文。
//   - RenderSkillContentForPrompt 返回已冻结且被保留的单一技能块 (只读)。
//
// 关键词: SkillContentSink, 窄接口, 避免包循环依赖, 只加载与查看
type SkillContentSink interface {
	AppendSkillMetadata(name, content, source string) bool
	AppendSkillFileContent(name, path, content, source string, startLine, endLine, total int) bool
	AppendSkillCatalog(rendered string) bool
	DeleteSkillContent(name string) bool
	DeleteSkillCatalog() bool
	VisibleSkillParts(name string) []SkillContextPart
	HasSkillContentHistory(name string) bool
	RenderSkillContentForPrompt() string
}

// skillTimelineBinding 保存 manager -> Timeline 的读写引用。manager 自身仍然
// 负责加载/校验/阅读窗口计算, Timeline 负责正文的展示与压缩。
type skillTimelineBinding struct {
	mu   sync.RWMutex
	sink SkillContentSink
}

// BindTimelineSink 绑定具体的 Timeline 适配器。允许在构造之后调用 (loop 构造
// 顺序决定 config/Timeline 可能晚于 manager 创建)。重复绑定以最后一次为准,
// 便于 fork/子 Agent 绑定到实际执行分支。
//
// 绑定只切换引用, 不加载、不补写、不改变 Timeline 的内容或冻结边界。
// 后续查询和渲染以目标 Timeline 为准; 默认加载与历史导入由初始化入口显式执行。
func (m *SkillsContextManager) BindTimelineSink(sink SkillContentSink) {
	if m == nil {
		return
	}
	m.timeline.mu.Lock()
	m.timeline.sink = sink
	m.timeline.mu.Unlock()
}

func (m *SkillsContextManager) timelineSink() SkillContentSink {
	if m == nil {
		return nil
	}
	m.timeline.mu.RLock()
	defer m.timeline.mu.RUnlock()
	return m.timeline.sink
}

func (m *SkillsContextManager) appendSkillMetadataToTimeline(name, content, source string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.AppendSkillMetadata(name, content, source)
}

func (m *SkillsContextManager) appendSkillFileContentToTimeline(name, path, content, source string, startLine, endLine, total int) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.AppendSkillFileContent(name, path, content, source, startLine, endLine, total)
}

func (m *SkillsContextManager) appendViewSegmentToTimeline(name, path, source string, vw *ViewWindow) bool {
	if vw == nil {
		return false
	}
	content, start, end, total, _ := vw.Segment()
	if strings.TrimSpace(content) == "" {
		return false
	}
	return m.appendSkillFileContentToTimeline(name, path, content, source, start, end, total)
}

// skillContentVisibleInTimeline 表示该技能正文当前仍可被模型读到 (Open 或已提升)。
// 可见性直接从仍未被退休的 metadata/content 派生, 不额外维护状态。
func (m *SkillsContextManager) skillContentVisibleInTimeline(name string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return len(sink.VisibleSkillParts(name)) > 0
}

// appendSkillCatalogToTimeline 把意图识别命中的「最相关 SKILL 目录」交给 Timeline。
// 目录是派生信息, 同样遵守“先 Open、冻结后才进前缀”的边界, 不能用实时目录提前改变
// 可缓存前缀。
func (m *SkillsContextManager) appendSkillCatalogToTimeline(rendered string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.AppendSkillCatalog(rendered)
}
