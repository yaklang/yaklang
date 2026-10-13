package aiskillloader

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// SkillsContextMaxTokens is the total size limit (in tokens) for all skills context.
	SkillsContextMaxTokens = 32 * 1024 // 32k tokens

	// MetadataListMaxTokens caps the metadata listing (in tokens) in the prompt when no skills are loaded.
	MetadataListMaxTokens = 8 * 1024 // 8k tokens
)

// skillContextState tracks the display state of a loaded skill.
type skillContextState struct {
	// Skill is the loaded skill data.
	Skill *LoadedSkill

	// ViewWindows holds active view windows for files in this skill.
	// Key is the file path within the skill filesystem.
	ViewWindows map[string]*ViewWindow
}

// ManagerOption configures SkillsContextManager.
type ManagerOption func(*SkillsContextManager)

// WithManagerDB enables optional database-backed skill loading/search context.
func WithManagerDB(db *gorm.DB) ManagerOption {
	return func(m *SkillsContextManager) {
		m.db = db
	}
}

// WithManagerSearchAICallback sets AI search callback.
func WithManagerSearchAICallback(cb SkillSearchAICallback) ManagerOption {
	return func(m *SkillsContextManager) {
		m.searchAICallback = cb
	}
}

// WithManagerMaxTokens 兼容入口。技能正文进入 Timeline 后由统一压缩控制体积,
// manager 不再单独维护 32K 折叠预算, 该选项保留只是为了不破坏既有调用方签名。
func WithManagerMaxTokens(maxTokens int) ManagerOption {
	return func(m *SkillsContextManager) {}
}

// WithManagerTokenEstimator sets an optional token estimator function.
// When nil (default), ytoken.CalcTokenCount is used.
func WithManagerTokenEstimator(estimator func(string) int) ManagerOption {
	return func(m *SkillsContextManager) {
		m.tokenEstimator = estimator
	}
}

// SkillsContextManager 是技能加载与查看工具, 不是正文托管者。
//
// 改造后的职责边界:
//   - 知道当前有哪些 skill 可以加载 (loader / 元信息 / 搜索 / 目录);
//   - 把 skill 加载进 Timeline (通过 SkillContentSink 窄接口, 只追加, 不渲染);
//   - 在 Timeline 中查看/翻页/删除已经加载的技能正文。
//
// 它不再维护 loadedSkills / forcedSkills / autoLoadedSkills 三套正文容器, 也不再
// 拼装 SKILLS_CONTEXT / USER_FORCED_SKILL / AUTO_LOADED_SKILLS 段: 正文的唯一权威
// 来源是 Timeline, 冻结后由 Timeline 的单一技能块统一渲染。
//
// 关键词: 不托管正文, 只加载与查看, 单一 Timeline 来源
type SkillsContextManager struct {
	mu sync.RWMutex

	// loader is the skill loader used to load skills on demand.
	loader SkillLoader

	// states 只记录已加载技能的轻量查看状态 (文件树 / 阅读窗口 / 翻页 offset)。
	// 它不参与 prompt 拼装, 也不决定展示位置。
	states *omap.OrderedMap[string, *skillContextState]

	// catalogSkills 是意图识别写入的「最相关 top-N」skill 元信息, 渲染为精简目录.
	catalogSkills []*SkillMeta

	// tokenEstimator overrides the default ytoken.CalcTokenCount for measuring size.
	// When nil, the built-in ytoken.CalcTokenCount is used.
	tokenEstimator func(string) int

	// Optional DB and AI callback for manager-level search capabilities.
	db               *gorm.DB
	searchAICallback SkillSearchAICallback

	// timeline 是运行上下文 Timeline 的窄接口绑定。绑定后, 技能正文的展示与
	// 压缩完全由 Timeline 承载, manager 不再独立拼装模型正文。
	timeline skillTimelineBinding
}

// NewSkillsContextManager creates a new SkillsContextManager.
func NewSkillsContextManager(loader SkillLoader, opts ...ManagerOption) *SkillsContextManager {
	m := &SkillsContextManager{
		loader: loader,
		states: omap.NewOrderedMap[string, *skillContextState](map[string]*skillContextState{}),
	}
	for _, opt := range opts {
		opt(m)
	}
	m.initializeSkillSearchPersistence()
	return m
}

// LoadDefaultSkills 在绑定之后由初始化入口显式加载 metadata auto_load 技能。
// 已有历史的技能完全遵循 Timeline 的保留结果, 不重新展开已退休的正文。
func (m *SkillsContextManager) LoadDefaultSkills() {
	if m == nil || m.loader == nil || m.timelineSink() == nil {
		return
	}
	for _, meta := range m.loader.AllSkillMetas() {
		if meta == nil || meta.DisableModelInvocation ||
			!strings.EqualFold(strings.TrimSpace(meta.Metadata[SkillMetadataAutoLoad]), "true") ||
			m.HasSkillContentHistory(meta.Name) {
			continue
		}
		if _, err := m.LoadAutoSkill(meta.Name); err != nil {
			log.Warnf("failed to load default skill %q: %v", meta.Name, err)
		}
	}
}

// initializeSkillSearchPersistence prepares optional indexes for DB-backed skill sources.
func (m *SkillsContextManager) initializeSkillSearchPersistence() {
	if m.db == nil {
		return
	}
	if err := yakit.EnsureAIForgeFTS5(m.db); err != nil {
		log.Warnf("failed to setup ai_forges FTS5 index: %v", err)
	}
}

// SetMaxTokens 兼容入口。正文体积改由 Timeline 统一压缩管理, 单个 manager 不再
// 维护独立的 32K 折叠预算。
func (m *SkillsContextManager) SetMaxTokens(maxTokens int) {}

// HasRegisteredSkills returns true if the loader has any skills available.
// This is used to determine whether the loading_skills action should be visible.
func (m *SkillsContextManager) HasRegisteredSkills() bool {
	if m.loader == nil {
		return false
	}
	return m.loader.HasSkills()
}

// HasTruncatedViews 返回是否存在被截断的阅读窗口, 用于决定 change_skill_view_offset
// 动作是否可见。
func (m *SkillsContextManager) HasTruncatedViews() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hasTruncated := false
	m.states.ForEach(func(name string, state *skillContextState) bool {
		for _, vw := range state.ViewWindows {
			if vw != nil && vw.IsTruncatedFlag() {
				hasTruncated = true
				return false
			}
		}
		return true
	})
	return hasTruncated
}

// SetCatalogSkills 写入意图识别命中的「最相关 top-N」skill 元信息 (渲染为精简目录).
// 内部会截断到 MaxCatalogSkills. 传入 nil / 空等价于清空 + 隐藏目录.
func (m *SkillsContextManager) SetCatalogSkills(metas []*SkillMeta) {
	m.mu.Lock()
	m.catalogSkills = dedupAndTruncateCatalog(metas, MaxCatalogSkills)
	rendered := renderCatalogSection(m.catalogSkills)
	m.mu.Unlock()

	// 目录是派生但需要展示的信息, 以 Timeline 为唯一正文来源: 先进入 Open,
	// 冻结提交后才提升进与技能正文相同的单一技能块。清空目录等价于移除该 item。
	if strings.TrimSpace(rendered) == "" {
		m.DeleteSkillCatalogFromTimeline()
		return
	}
	m.AppendSkillCatalogToTimeline(strings.TrimSpace(rendered))
}

// DeleteSkillCatalogFromTimeline 退休 Timeline 中当前可见的技能目录 item
// (目录是派生信息, 清空/隐藏时不应继续渲染)。不影响任何技能正文。
func (m *SkillsContextManager) DeleteSkillCatalogFromTimeline() bool {
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.DeleteSkillCatalog()
}

// renderCatalogSection 渲染「最相关 SKILL 目录」正文 (含目录头 + 逐条元信息 +
// 加载提示)。目录作为 skill-content 类型的一条 item 进入 Timeline。
func renderCatalogSection(skills []*SkillMeta) string {
	if len(skills) == 0 {
		return ""
	}
	// 按 name 排序保证字节稳定 (利于缓存)。
	sorted := sortSkillMetasByName(skills)
	if len(sorted) > MaxCatalogSkills {
		sorted = sorted[:MaxCatalogSkills]
	}
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf(CatalogHeader, len(sorted)))
	for _, meta := range sorted {
		buf.WriteString(FormatAvailableSkillRegistryLine(meta))
	}
	buf.WriteString("Use loading_skills action to load any of these when needed.\n")
	return buf.String()
}

// GetCatalogSkills 返回当前目录 skill 元信息副本.
func (m *SkillsContextManager) GetCatalogSkills() []*SkillMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.catalogSkills) == 0 {
		return nil
	}
	out := make([]*SkillMeta, len(m.catalogSkills))
	copy(out, m.catalogSkills)
	return out
}

// ListSkills lists all skills from loader metadata.
func (m *SkillsContextManager) ListSkills() ([]*SkillMeta, error) {
	if m.loader == nil {
		return nil, utils.Error("skills context manager: no loader configured")
	}
	return m.loader.AllSkillMetas(), nil
}

// SearchSkills searches skills by keyword against name and description.
func (m *SkillsContextManager) SearchSkills(query string) ([]*SkillMeta, error) {
	if m.loader == nil {
		return nil, utils.Error("skills context manager: no loader configured")
	}
	return SearchSkillMetas(m.loader, query, 20)
}

// SearchKeywordBM25 uses SQLite FTS5 BM25 ranking to search skills by keyword.
// Search is built from the current loader view so filesystem skills and database skills
// participate uniformly without implicitly persisting transient sources.
func (m *SkillsContextManager) SearchKeywordBM25(query string, limit int) ([]*SkillMeta, error) {
	if m.loader == nil {
		return nil, utils.Error("skills context manager: no loader configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	return SearchSkillMetas(m.loader, query, limit)
}

// SearchByAI performs AI-based skill selection through manager callback.
func (m *SkillsContextManager) SearchByAI(userNeed string) ([]*SkillMeta, error) {
	if m.searchAICallback == nil {
		return nil, utils.Error("search AI callback is not configured")
	}
	metas, err := m.ListSkills()
	if err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return nil, nil
	}
	return SearchByAI(metas, userNeed, m.searchAICallback)
}

// GetCurrentSelectedSkills 以目标 Timeline 的可见技能名单为准。
// loader 和文件/窗口缓存仅补充元信息, 不决定技能是否仍在上下文中。
func (m *SkillsContextManager) GetCurrentSelectedSkills() []*SkillMeta {
	sink := m.timelineSink()
	if sink == nil {
		return nil
	}
	names := sink.VisibleSkillContentNames()
	if len(names) == 0 {
		return nil
	}
	metas := make(map[string]*SkillMeta, len(names))
	m.mu.RLock()
	m.states.ForEach(func(name string, state *skillContextState) bool {
		if state != nil && state.Skill != nil && state.Skill.Meta != nil {
			metas[name] = state.Skill.Meta
		}
		return true
	})
	m.mu.RUnlock()
	if m.loader != nil {
		for _, meta := range m.loader.AllSkillMetas() {
			if meta != nil {
				metas[meta.Name] = meta
			}
		}
	}
	selected := make([]*SkillMeta, 0, len(names))
	for _, name := range names {
		meta := metas[name]
		if meta == nil {
			if lookup, ok := m.loader.(SkillMetaLookup); ok {
				meta, _ = lookup.GetSkillMeta(name)
			}
		}
		if meta == nil {
			meta = &SkillMeta{Name: name}
		}
		selected = append(selected, meta)
	}
	return selected
}

// stateFor returns the lightweight view state for a loaded skill.
func (m *SkillsContextManager) stateFor(skillName string) (*skillContextState, bool) {
	if m == nil || m.states == nil {
		return nil, false
	}
	return m.states.Get(skillName)
}

// newSkillState 构造一个技能的查看状态: 说明 + 文件树 + SKILL.md 阅读窗口。
func newSkillState(loaded *LoadedSkill) *skillContextState {
	if loaded == nil {
		return nil
	}
	transformedContent := TransformIncludesToResourceHints(loaded.SkillMDContent, loaded.Meta.Name)
	nonce := GenerateNonce(loaded.Meta.Name, skillMDFilename)
	skillMDWindow := NewViewWindow(loaded.Meta.Name, skillMDFilename, transformedContent, nonce)
	return &skillContextState{
		Skill: loaded,
		ViewWindows: map[string]*ViewWindow{
			skillMDFilename: skillMDWindow,
		},
	}
}

// ensureLoaded 读取 (必要时加载) 一个技能的查看状态。
// 已绑定 Timeline 时正文写入目标 Timeline; 未绑定时只保留本地查看状态,
// 绑定不会把这些本地状态补写或覆盖到 Timeline。
//
// 返回 (state, loaded, error): loaded=true 表示本次新建了查看状态。
func (m *SkillsContextManager) ensureLoaded(skillName string) (*skillContextState, bool, error) {
	if m.loader == nil {
		return nil, false, utils.Error("skills context manager: no loader configured")
	}
	if state, ok := m.stateFor(skillName); ok && state != nil && state.Skill != nil {
		return state, false, nil
	}
	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return nil, false, utils.Wrapf(err, "failed to load skill %q", skillName)
	}
	state := newSkillState(loaded)
	if state == nil {
		return nil, false, utils.Errorf("failed to build view state for skill %q", skillName)
	}
	m.states.Set(skillName, state)
	return state, true, nil
}

// LoadSkill 加载一个技能: 读取说明与文件树, 把正文写进 Timeline。
// 相同正文仍在 Timeline 时不重复写入; 已退休的正文或新阅读内容正常追加。
func (m *SkillsContextManager) LoadSkill(skillName string) error {
	return m.loadSkillFrom(skillName, SkillContentSourceManual)
}

// LoadForcedSkill 加载一个由用户显式指定的技能。它与普通加载的唯一区别是来源
// 标记 (user-forced), 该标记进入正文 payload 供压缩提示与观测使用; 展示位置与
// 压缩规则与其它来源完全一致。
//
// 返回 (新增?, error): false 表示本次正文完全重复, 无需再次写入。
func (m *SkillsContextManager) LoadForcedSkill(skillName string) (bool, error) {
	loaded, err := m.loadSkillFromWithNew(skillName, SkillContentSourceForced)
	if err != nil {
		return false, err
	}
	return loaded, nil
}

// LoadAutoSkill 加载一个 AI 意图驱动的技能。与普通加载同构, 仅来源标记不同。
//
// 返回 (新增?, error): false 表示本次正文完全重复, 无需再次写入。
func (m *SkillsContextManager) LoadAutoSkill(skillName string) (bool, error) {
	loaded, err := m.loadSkillFromWithNew(skillName, SkillContentSourceAuto)
	if err != nil {
		return false, err
	}
	return loaded, nil
}

func (m *SkillsContextManager) loadSkillFrom(skillName, source string) error {
	_, err := m.loadSkillFromWithNew(skillName, source)
	return err
}

// loadSkillFromWithNew 是三条加载路径的统一实现: 加载/复用查看状态, 并把正文追加
// 到 Timeline。由写入入口逐项检查正文是否完全重复, 不能仅按技能名短路。
// 返回 true 表示实际追加了正文; 未绑定 Timeline 时只建立本地查看状态并返回 false。
func (m *SkillsContextManager) loadSkillFromWithNew(skillName, source string) (bool, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false, utils.Error("skill name is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, _, err := m.ensureLoaded(skillName)
	if err != nil {
		return false, err
	}
	return m.appendSkillBodyToTimelineLocked(state, source), nil
}

// HasSkillContentHistory 判断该技能是否已经在本 Timeline 里留下过正文记录 (即使
// 已被显式移除)。持久会话恢复用它避免用旧名单把已移除正文重新展开。
func (m *SkillsContextManager) HasSkillContentHistory(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	return sink.HasSkillContentHistory(skillName)
}

// LoadSkills loads multiple skills into the context manager in batch.
// Returns a map of skill name to error (nil error means success).
func (m *SkillsContextManager) LoadSkills(names []string) map[string]error {
	results := make(map[string]error, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		results[name] = m.LoadSkill(name)
	}
	return results
}

// UnloadSkill 从 Timeline 中移除该技能的正文 item。删除只影响后续冻结事务的
// 成员集合, 不回滚已提交的冻结边界; 再次加载会重新追加一份新正文。
func (m *SkillsContextManager) UnloadSkill(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	sink := m.timelineSink()
	if sink == nil {
		return false
	}
	m.mu.Lock()
	m.states.Delete(skillName)
	m.mu.Unlock()
	removed := sink.DeleteSkillContent(skillName)
	if removed {
		log.Infof("unloaded skill %q from timeline", skillName)
	}
	return removed
}

// UnloadForcedSkill 与 UnloadSkill 等价: forced 只是来源标记, 不再是独立容器。
func (m *SkillsContextManager) UnloadForcedSkill(skillName string) bool {
	return m.UnloadSkill(skillName)
}

// UnloadAutoSkill 与 UnloadSkill 等价。
func (m *SkillsContextManager) UnloadAutoSkill(skillName string) bool {
	return m.UnloadSkill(skillName)
}

// HasAutoLoadedSkills 返回是否存在任何已加载技能; 加载来源不再区分展示位置,
// 该判断保留给既有调用方表达「当前有技能正文进入上下文」。
func (m *SkillsContextManager) HasAutoLoadedSkills() bool {
	return len(m.GetCurrentSelectedSkills()) > 0
}

// ChangeViewOffset 重新定位某个技能文件的阅读窗口, 并把新的阅读窗口作为一条
// 新的 Timeline item 追加进去 (不替换旧 item)。旧窗口在之后由统一压缩按
// 现有筛选+总结机制决定去留, 缓存前缀不会被热改。
func (m *SkillsContextManager) ChangeViewOffset(skillName, filePath string, offset int) error {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return utils.Error("skill name is empty")
	}
	m.mu.Lock()
	state, ok := m.stateFor(skillName)
	if !ok || state == nil || state.Skill == nil {
		m.mu.Unlock()
		return utils.Errorf("skill %q is not loaded", skillName)
	}
	if filePath == "" {
		filePath = skillMDFilename
	}
	vw, ok := state.ViewWindows[filePath]
	if !ok {
		content, err := state.Skill.FileSystem.ReadFile(filePath)
		if err != nil {
			m.mu.Unlock()
			return utils.Wrapf(err, "failed to read file %q from skill %q", filePath, skillName)
		}
		nonce := GenerateNonce(skillName, filePath)
		vw = NewViewWindow(skillName, filePath, string(content), nonce)
		state.ViewWindows[filePath] = vw
	}
	vw.SetOffset(offset)
	body := m.renderViewWindowForTimeline(state, filePath)
	m.mu.Unlock()

	m.appendSkillContentToTimeline(skillName, filePath, body, SkillContentSourceManual)
	return nil
}

// IsSkillLoaded 返回该技能正文当前是否可见。
// 只查询目标 Timeline 的可见性, 不使用本地文件/窗口缓存判断上下文是否已加载。
func (m *SkillsContextManager) IsSkillLoaded(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	return m.skillContentVisibleInTimeline(skillName)
}

// IsSkillLoadedAndUnfolded 与 IsSkillLoaded 等价: 折叠/展开由统一压缩取代,
// 不存在 manager 侧的折叠状态。
func (m *SkillsContextManager) IsSkillLoadedAndUnfolded(skillName string) bool {
	return m.IsSkillLoaded(skillName)
}

// IsAutoSkillLoadedAndUnfolded 与 IsSkillLoaded 等价。
func (m *SkillsContextManager) IsAutoSkillLoadedAndUnfolded(skillName string) bool {
	return m.IsSkillLoaded(skillName)
}

// GetSkillViewSummary 返回该技能当前可见的阅读窗口摘要。
func (m *SkillsContextManager) GetSkillViewSummary(skillName string) string {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return ""
	}
	if !m.skillContentVisibleInTimeline(skillName) {
		return ""
	}
	m.mu.RLock()
	state, ok := m.stateFor(skillName)
	if !ok || state == nil {
		m.mu.RUnlock()
		return ""
	}
	windows := sortedViewWindows(state.ViewWindows)
	m.mu.RUnlock()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Skill '%s' is loaded and ACTIVE in the skills context of your prompt. ", skillName))
	buf.WriteString("View Windows:\n")
	for _, vw := range windows {
		truncInfo := ""
		if vw.IsTruncatedFlag() {
			truncInfo = " (truncated, use change_skill_view_offset to see more)"
		}
		buf.WriteString(fmt.Sprintf("  - %s: %d total lines, viewing from line %d%s\n",
			vw.FilePath, vw.TotalLines(), vw.GetOffset(), truncInfo))
	}
	return buf.String()
}

// GetLoader returns the skill loader.
func (m *SkillsContextManager) GetLoader() SkillLoader {
	return m.loader
}

// TokenEstimator returns the optional custom token estimator for prompt sizing.
func (m *SkillsContextManager) TokenEstimator() func(string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tokenEstimator
}

// Render 是兼容入口: 技能正文已统一由 Timeline 渲染, 此处返回 Timeline 的
// 冻结技能块; nonce 参数保留只为兼容既有调用方。
func (m *SkillsContextManager) Render(nonce string) string {
	return m.RenderStable()
}

// RenderStable 返回 Timeline 中已冻结、仍被保留的技能正文块 (单一技能块, 位于
// SemiDynamic1, 与 prompted_recent_tools 同段)。
func (m *SkillsContextManager) RenderStable() string {
	sink := m.timelineSink()
	if sink == nil {
		return ""
	}
	return sink.RenderSkillContentForPrompt()
}

// renderSkillBody 渲染一个技能的说明 + 文件树。
//
// 调用方需持有 m.mu。
func (m *SkillsContextManager) renderSkillBody(state *skillContextState) string {
	if state == nil || state.Skill == nil || state.Skill.Meta == nil {
		return ""
	}
	var buf bytes.Buffer
	buf.WriteString(state.Skill.Meta.BriefString())

	relatedSkills := DetectCrossSkillReferences(state.Skill.SkillMDContent, state.Skill.Meta.Name)
	if len(relatedSkills) > 0 {
		buf.WriteString(fmt.Sprintf("\nRelated Skills (referenced in SKILL.md): %s\n", strings.Join(relatedSkills, ", ")))
		buf.WriteString("Use loading_skills to load these related skills if needed.\n")
	}

	buf.WriteString("\nFile Tree:\n")
	buf.WriteString(RenderFileSystemTreeFull(state.Skill.FileSystem))
	return buf.String()
}

// renderViewWindowForTimeline 渲染一个阅读窗口 (含文件信息头)。
//
// 调用方需持有 m.mu。
func (m *SkillsContextManager) renderViewWindowForTimeline(state *skillContextState, filePath string) string {
	if state == nil {
		return ""
	}
	if vw, ok := state.ViewWindows[filePath]; ok && vw != nil {
		return vw.RenderWithInfo()
	}
	return ""
}

// appendSkillBodyToTimelineLocked 分别写入技能说明/文件树和各阅读窗口, 返回是否
// 实际追加了正文。重复检查由 Timeline 写入入口执行, 不维护 manager 侧正文副本。
// 调用方需持有 m.mu。sink 只访问 Timeline 自己的锁, 不会回调 manager。
func (m *SkillsContextManager) appendSkillBodyToTimelineLocked(state *skillContextState, source string) bool {
	if m == nil || state == nil || state.Skill == nil || state.Skill.Meta == nil {
		return false
	}
	added := false
	name := state.Skill.Meta.Name
	if body := m.renderSkillBody(state); strings.TrimSpace(body) != "" {
		if m.appendSkillContentToTimeline(name, SkillMainBodyPath, body, source) {
			added = true
		}
	}
	for _, path := range sortedViewWindowPaths(state.ViewWindows) {
		rendered := m.renderViewWindowForTimeline(state, path)
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		if m.appendSkillContentToTimeline(name, path, rendered, source) {
			added = true
		}
	}
	return added
}

// sortedViewWindowPaths 返回按路径排序的阅读窗口 key 快照。
func sortedViewWindowPaths(viewWindows map[string]*ViewWindow) []string {
	if len(viewWindows) == 0 {
		return nil
	}
	paths := make([]string, 0, len(viewWindows))
	for path := range viewWindows {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// dedupAndTruncateCatalog 对 catalog skill 元信息去重 (按 name) 并截断到 max.
func dedupAndTruncateCatalog(metas []*SkillMeta, max int) []*SkillMeta {
	if len(metas) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(metas))
	out := make([]*SkillMeta, 0, len(metas))
	for _, meta := range metas {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		if seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		out = append(out, meta)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func sortSkillMetasByName(metas []*SkillMeta) []*SkillMeta {
	if len(metas) <= 1 {
		return metas
	}
	sorted := append([]*SkillMeta(nil), metas...)
	sort.Slice(sorted, func(i, j int) bool {
		leftName := ""
		rightName := ""
		if sorted[i] != nil {
			leftName = sorted[i].Name
		}
		if sorted[j] != nil {
			rightName = sorted[j].Name
		}
		if leftName == rightName {
			leftDesc := ""
			rightDesc := ""
			if sorted[i] != nil {
				leftDesc = sorted[i].Description
			}
			if sorted[j] != nil {
				rightDesc = sorted[j].Description
			}
			return leftDesc < rightDesc
		}
		return leftName < rightName
	})
	return sorted
}

func sortedViewWindows(viewWindows map[string]*ViewWindow) []*ViewWindow {
	if len(viewWindows) == 0 {
		return nil
	}
	keys := make([]string, 0, len(viewWindows))
	for key := range viewWindows {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]*ViewWindow, 0, len(keys))
	for _, key := range keys {
		if vw := viewWindows[key]; vw != nil {
			result = append(result, vw)
		}
	}
	return result
}

var crossSkillRefRegexp = regexp.MustCompile(`\.\.\/([a-zA-Z0-9][a-zA-Z0-9_-]*)\/`)

// DetectCrossSkillReferences scans SKILL.md content for cross-skill
// references (e.g. ../other-skill/file.md) and returns deduplicated,
// sorted skill names, excluding the current skill itself.
func DetectCrossSkillReferences(content string, currentSkillName string) []string {
	matches := crossSkillRefRegexp.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var skills []string
	for _, match := range matches {
		skillName := match[1]
		if skillName == currentSkillName || seen[skillName] {
			continue
		}
		seen[skillName] = true
		skills = append(skills, skillName)
	}

	sort.Strings(skills)
	return skills
}

func buildKeywordsString(meta *SkillMeta) string {
	var parts []string
	for k, v := range meta.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, ",")
}
