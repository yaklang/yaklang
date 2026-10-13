package aiskillloader

import "sync"

// SkillContentSource* 区分技能正文的来源, 仅用于压缩提示与观测; 它不是渲染分组,
// 也不再对应独立的展示块或独立的压缩保护。
const (
	SkillContentSourceAuto   = "auto"
	SkillContentSourceForced = "forced"
	SkillContentSourceManual = "manual"
	// SkillMainBodyPath 是“技能说明 + 文件树”这一非文件主体的 Key 后缀;
	// 具体文件/阅读窗口使用真实路径, 因此两者不会互相覆盖。
	SkillMainBodyPath = "\x00main"
)

// SkillContentSink 是 aiskillloader 侧对“运行时上下文 Timeline”的最小依赖。
// 具体实现由 aicommon 提供, 避免 aiskillloader 反向 import aicommon 造成循环依赖。
//
// 语义:
//   - AppendSkillContent 追加一段可供模型阅读的正文, 已存在相同正文时返回 false,
//     不触发冻结/提升, 不替换同一路径的其它阅读内容。
//   - DeleteSkillContent 退休该技能当前可见的正文, 不触碰冻结边界。
//   - HasVisibleSkillContent 判断当前上下文中是否仍有该技能正文。
//   - VisibleSkillContentNames 枚举目标 Timeline 中仍可见的技能。
//   - HasSkillContentHistory 判断是否存在加载历史, 包括已经退休的正文。
//   - RenderSkillContentForPrompt 返回已冻结且被保留的单一技能块 (只读)。
//
// 关键词: SkillContentSink, 窄接口, 避免包循环依赖, 只加载与查看
type SkillContentSink interface {
	AppendSkillContent(name, path, content, source string) bool
	DeleteSkillContent(name string) bool
	DeleteSkillCatalog() bool
	HasVisibleSkillContent(name string) bool
	VisibleSkillContentNames() []string
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

// appendSkillContentToTimeline 把一次成功读取的正文写入 Timeline。返回是否写入
// 成功; 未绑定 Timeline 时返回 false。
func (m *SkillsContextManager) appendSkillContentToTimeline(name, path, content, source string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.AppendSkillContent(name, path, content, source)
}

// skillContentVisibleInTimeline 表示该技能正文当前仍可被模型读到 (Open 或已提升)。
func (m *SkillsContextManager) skillContentVisibleInTimeline(name string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.HasVisibleSkillContent(name)
}

// AppendSkillCatalogToTimeline 把意图识别命中的「最相关 SKILL 目录」交给 Timeline。
// 目录是派生信息, 同样遵守“先 Open、冻结后才进前缀”的边界, 不能用实时目录提前改变
// 可缓存前缀。
func (m *SkillsContextManager) AppendSkillCatalogToTimeline(rendered string) bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	catalog, ok := sink.(interface {
		AppendSkillCatalog(rendered string) bool
	})
	if !ok {
		return false
	}
	return catalog.AppendSkillCatalog(rendered)
}
