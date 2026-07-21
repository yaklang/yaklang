package aiskillloader

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

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

	// IsFolded indicates whether this skill is displayed in folded mode.
	IsFolded bool

	// ViewWindows holds active view windows for files in this skill.
	// Key is the file path within the skill filesystem.
	ViewWindows map[string]*ViewWindow

	// LastAccessedAt tracks when this skill was last accessed (loaded, unfolded, or queried).
	// Used by the LRU folding strategy to keep frequently-used skills unfolded.
	LastAccessedAt time.Time
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

// WithManagerMaxTokens sets the context max size (in tokens) at initialization.
func WithManagerMaxTokens(maxTokens int) ManagerOption {
	return func(m *SkillsContextManager) {
		if maxTokens > 0 {
			m.maxTokens = maxTokens
		}
	}
}

// WithManagerTokenEstimator sets an optional token estimator function.
// When nil (default), ytoken.CalcTokenCount is used.
func WithManagerTokenEstimator(estimator func(string) int) ManagerOption {
	return func(m *SkillsContextManager) {
		m.tokenEstimator = estimator
	}
}

// SkillsContextManager manages the skills context window in the ReAct loop prompt.
// It maintains loaded skills, handles folding/unfolding, and renders the context.
// It also provides manager-level listing and search capabilities.
type SkillsContextManager struct {
	mu sync.RWMutex

	// loader is the skill loader used to load skills on demand.
	loader SkillLoader

	// loadedSkills is an ordered map of loaded skills, keyed by skill name.
	// 语义 (改造后): 持久会话恢复的历史加载 + 用户强制加载也会在这里登记一份
	// (保证 IsSkillLoaded 查询一致, 避免 AI action 重复加载).
	// 渲染进 SKILLS_CONTEXT 上半段 (与 catalog 目录同处 SemiDynamic 1).
	// Ordering is by load time (oldest first).
	loadedSkills *omap.OrderedMap[string, *skillContextState]

	// forcedSkills 是「用户强制加载」专用容器 (满内容, 进 frozen_block 顶部).
	// 来源: load_skill sync 事件 / EnabledCapabilities / hotpatch skill handler.
	// 优先级最高, 独立于 loadedSkills / autoLoadedSkills 渲染.
	forcedSkills *ForcedSkillRegistry

	// autoLoadedSkills 是「AI 意图驱动加载」容器 (进 SemiDynamic 2 尾部).
	// 来源: loading_skills action (AI 自主加载). 走 LRU 折叠控制体积.
	autoLoadedSkills *omap.OrderedMap[string, *skillContextState]

	// catalogVisible 控制「最相关 SKILL 目录」(SKILLS_CONTEXT 下半段) 是否渲染.
	// 默认 false: 意图识别命中后才由 SetCatalogVisible(true) 开启, 节约 token.
	catalogVisible bool
	// catalogSkills 是意图识别写入的「最相关 top-N」skill 元信息, 渲染为精简目录.
	catalogSkills []*SkillMeta

	// maxTokens is the total size limit (in tokens) for the skills context.
	maxTokens int

	// cachedContextSize stores the last computed context size to avoid repeated rendering.
	// Invalidated by setting contextSizeDirty to true when skills are loaded/folded/changed.
	cachedContextSize int
	contextSizeDirty  bool

	// tokenEstimator overrides the default ytoken.CalcTokenCount for measuring size.
	// When nil, the built-in ytoken.CalcTokenCount is used.
	tokenEstimator func(string) int

	// Optional DB and AI callback for manager-level search capabilities.
	db               *gorm.DB
	searchAICallback SkillSearchAICallback
}

// NewSkillsContextManager creates a new SkillsContextManager.
func NewSkillsContextManager(loader SkillLoader, opts ...ManagerOption) *SkillsContextManager {
	m := &SkillsContextManager{
		loader:           loader,
		loadedSkills:     omap.NewOrderedMap[string, *skillContextState](map[string]*skillContextState{}),
		forcedSkills:     NewForcedSkillRegistry(),
		autoLoadedSkills: omap.NewOrderedMap[string, *skillContextState](map[string]*skillContextState{}),
		maxTokens:        SkillsContextMaxTokens,
		contextSizeDirty: true,
	}
	for _, opt := range opts {
		opt(m)
	}
	m.initializeSkillSearchPersistence()
	return m
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

// SetMaxTokens sets the total context size limit (in tokens).
func (m *SkillsContextManager) SetMaxTokens(maxTokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxTokens = maxTokens
}

// HasRegisteredSkills returns true if the loader has any skills available.
// This is used to determine whether the loading_skills action should be visible.
func (m *SkillsContextManager) HasRegisteredSkills() bool {
	if m.loader == nil {
		return false
	}
	return m.loader.HasSkills()
}

// HasTruncatedViews returns true if any loaded skill has truncated/folded content.
// This is used to determine whether the change_skill_view_offset action should be visible.
func (m *SkillsContextManager) HasTruncatedViews() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hasTruncated := false
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state.IsFolded {
			hasTruncated = true
			return false
		}
		for _, vw := range state.ViewWindows {
			if vw.IsTruncated {
				hasTruncated = true
				return false
			}
		}
		return true
	})
	return hasTruncated
}

// HasForcedSkills returns true if there is any user-forced skill (rendered into frozen_block).
func (m *SkillsContextManager) HasForcedSkills() bool {
	if m == nil || m.forcedSkills == nil {
		return false
	}
	return !m.forcedSkills.IsEmpty()
}

// HasAutoLoadedSkills returns true if there is any AI-intent-driven auto-loaded skill
// (rendered into SemiDynamic 2 tail).
func (m *SkillsContextManager) HasAutoLoadedSkills() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.autoLoadedSkills.Len() > 0
}

// SetCatalogVisible 控制「最相关 SKILL 目录」(SKILLS_CONTEXT 下半段) 是否渲染.
// 默认 false; 意图识别命中后由调用方 SetCatalogVisible(true) 开启.
func (m *SkillsContextManager) SetCatalogVisible(visible bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.catalogVisible = visible
}

// IsCatalogVisible 返回目录是否可见.
func (m *SkillsContextManager) IsCatalogVisible() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.catalogVisible
}

// SetCatalogSkills 写入意图识别命中的「最相关 top-N」skill 元信息 (渲染为精简目录).
// 内部会截断到 MaxCatalogSkills. 传入 nil / 空等价于清空 + 隐藏目录.
func (m *SkillsContextManager) SetCatalogSkills(metas []*SkillMeta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.catalogSkills = dedupAndTruncateCatalog(metas, MaxCatalogSkills)
	if len(m.catalogSkills) > 0 {
		m.catalogVisible = true
	}
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

// GetCurrentSelectedSkills returns currently loaded/selected skills across all
// three containers (loadedSkills = persistent-restore + forced 副本, autoLoadedSkills
// = AI 意图驱动). Forced skills also live in loadedSkills, so no separate iteration
// needed for them.
func (m *SkillsContextManager) GetCurrentSelectedSkills() []*SkillMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	selected := make([]*SkillMeta, 0, m.loadedSkills.Len()+m.autoLoadedSkills.Len())
	seen := make(map[string]bool, m.loadedSkills.Len()+m.autoLoadedSkills.Len())
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state != nil && state.Skill != nil && state.Skill.Meta != nil && !seen[name] {
			seen[name] = true
			selected = append(selected, state.Skill.Meta)
		}
		return true
	})
	m.autoLoadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state != nil && state.Skill != nil && state.Skill.Meta != nil && !seen[name] {
			seen[name] = true
			selected = append(selected, state.Skill.Meta)
		}
		return true
	})
	return selected
}

// LoadSkill loads a skill into the context manager.
// If loading the new skill exceeds the size limit, least-recently-used skills are folded.
func (m *SkillsContextManager) LoadSkill(skillName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loader == nil {
		return utils.Error("skills context manager: no loader configured")
	}

	now := time.Now()

	// Check if already loaded
	if existing, ok := m.loadedSkills.Get(skillName); ok {
		existing.LastAccessedAt = now
		if existing.IsFolded {
			existing.IsFolded = false
			m.contextSizeDirty = true
			m.ensureContextFits()
		}
		return nil
	}

	// Load the skill
	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return utils.Wrapf(err, "failed to load skill %q", skillName)
	}

	transformedContent := TransformIncludesToResourceHints(loaded.SkillMDContent, skillName)

	nonce := GenerateNonce(skillName, skillMDFilename)
	skillMDWindow := NewViewWindow(skillName, skillMDFilename, transformedContent, nonce)

	state := &skillContextState{
		Skill:    loaded,
		IsFolded: false,
		ViewWindows: map[string]*ViewWindow{
			skillMDFilename: skillMDWindow,
		},
		LastAccessedAt: now,
	}

	m.loadedSkills.Set(skillName, state)
	m.contextSizeDirty = true
	log.Infof("loaded skill %q into context", skillName)

	m.ensureContextFits()
	return nil
}

// UnloadSkill removes a loaded skill from the context manager.
func (m *SkillsContextManager) UnloadSkill(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loadedSkills.Have(skillName) {
		return false
	}
	m.loadedSkills.Delete(skillName)
	m.contextSizeDirty = true
	log.Infof("unloaded skill %q from context", skillName)
	return true
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

// LoadForcedSkill 用户强制加载一个 SKILL (满内容, 进 frozen_block 顶部).
// 与 LoadAutoSkill (AI 意图驱动, 进 SemiDynamic 2) 区分开, 这是最高优先级路径.
// 同时在 loadedSkills 登记一份, 保证 IsSkillLoaded 查询一致, 避免 AI action 重复加载.
// 返回 (新增?, error): 新增=false 表示该 skill 已是 forced.
//
// 关键词: LoadForcedSkill, 用户强制加载, frozen_block, 满内容
func (m *SkillsContextManager) LoadForcedSkill(skillName string) (bool, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false, utils.Error("skill name is empty")
	}
	m.mu.Lock()
	unlock := true
	defer func() {
		if unlock {
			m.mu.Unlock()
		}
	}()

	if m.loader == nil {
		return false, utils.Error("skills context manager: no loader configured")
	}

	// 已是 forced: 幂等返回.
	if m.forcedSkills.Has(skillName) {
		return false, nil
	}

	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return false, utils.Wrapf(err, "failed to load skill %q", skillName)
	}

	m.forcedSkills.Add(skillName, loaded)

	// 同时登记进 loadedSkills (若尚未存在), 保证 IsSkillLoaded 查询一致.
	if _, ok := m.loadedSkills.Get(skillName); !ok {
		now := time.Now()
		transformedContent := TransformIncludesToResourceHints(loaded.SkillMDContent, skillName)
		nonce := GenerateNonce(skillName, skillMDFilename)
		skillMDWindow := NewViewWindow(skillName, skillMDFilename, transformedContent, nonce)
		state := &skillContextState{
			Skill:    loaded,
			IsFolded: false,
			ViewWindows: map[string]*ViewWindow{
				skillMDFilename: skillMDWindow,
			},
			LastAccessedAt: now,
		}
		m.loadedSkills.Set(skillName, state)
		m.contextSizeDirty = true
		// ensureContextFits 需要 mu, 但我们在锁内, 直接传 false 解锁由调用者释放不安全,
		// 故这里先解锁再调用 (ensureContextFits 内部会自己加锁).
		m.mu.Unlock()
		unlock = false
		m.ensureContextFits()
	}
	log.Infof("force-loaded skill %q into frozen_block", skillName)
	return true, nil
}

// UnloadForcedSkill 移除一个 forced skill (仅从 frozen_block 容器移除, 不动 loadedSkills).
// 返回是否曾存在.
func (m *SkillsContextManager) UnloadForcedSkill(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	return m.forcedSkills.Remove(skillName)
}

// IsForcedSkill 返回该 skill 是否为用户强制加载.
func (m *SkillsContextManager) IsForcedSkill(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	return m.forcedSkills.Has(skillName)
}

// LoadAutoSkill AI 意图驱动加载一个 SKILL (进 SemiDynamic 2 尾部, LRU 折叠).
// 来源: loading_skills action. 若 skill 已是 forced, 短路返回成功 (不重复加载).
// 返回 (新增?, error): 新增=false 表示该 skill 已是 forced 或已 auto-loaded.
//
// 关键词: LoadAutoSkill, AI 意图驱动, SemiDynamic 2, LRU 折叠
func (m *SkillsContextManager) LoadAutoSkill(skillName string) (bool, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false, utils.Error("skill name is empty")
	}
	m.mu.Lock()
	unlock := true
	defer func() {
		if unlock {
			m.mu.Unlock()
		}
	}()

	if m.loader == nil {
		return false, utils.Error("skills context manager: no loader configured")
	}

	// 已是 forced: 幂等短路 (forced 优先级更高, 满内容已可见, 无需重复加载).
	if m.forcedSkills.Has(skillName) {
		return false, nil
	}

	now := time.Now()

	// 已 auto-loaded: 刷新 LRU 时间, 若被折叠则展开.
	if existing, ok := m.autoLoadedSkills.Get(skillName); ok {
		existing.LastAccessedAt = now
		if existing.IsFolded {
			existing.IsFolded = false
			m.contextSizeDirty = true
			m.ensureAutoSkillsFits()
		}
		return false, nil
	}

	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return false, utils.Wrapf(err, "failed to load skill %q", skillName)
	}

	transformedContent := TransformIncludesToResourceHints(loaded.SkillMDContent, skillName)
	nonce := GenerateNonce(skillName, skillMDFilename)
	skillMDWindow := NewViewWindow(skillName, skillMDFilename, transformedContent, nonce)
	state := &skillContextState{
		Skill:    loaded,
		IsFolded: false,
		ViewWindows: map[string]*ViewWindow{
			skillMDFilename: skillMDWindow,
		},
		LastAccessedAt: now,
	}
	m.autoLoadedSkills.Set(skillName, state)
	m.ensureAutoSkillsFits()
	log.Infof("auto-loaded skill %q into SemiDynamic 2", skillName)
	return true, nil
}

// UnloadAutoSkill 从 auto-loaded 容器移除一个 skill.
func (m *SkillsContextManager) UnloadAutoSkill(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.autoLoadedSkills.Have(skillName) {
		return false
	}
	m.autoLoadedSkills.Delete(skillName)
	log.Infof("unloaded auto skill %q", skillName)
	return true
}

// IsAutoSkillLoadedAndUnfolded 返回 auto-loaded skill 是否已加载且未折叠.
// 用于 loading_skills action 的 ActionVerifier 短路 (避免重复加载).
func (m *SkillsContextManager) IsAutoSkillLoadedAndUnfolded(skillName string) bool {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	// forced 也算「已加载且满可见」, action 应短路.
	if m.forcedSkills.Has(skillName) {
		return true
	}
	state, ok := m.autoLoadedSkills.Get(skillName)
	if !ok {
		return false
	}
	return !state.IsFolded
}

// ChangeViewOffset changes the view offset for a file in a loaded skill.
func (m *SkillsContextManager) ChangeViewOffset(skillName, filePath string, offset int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		return utils.Errorf("skill %q is not loaded", skillName)
	}

	state.LastAccessedAt = time.Now()

	if filePath == "" {
		filePath = skillMDFilename
	}

	vw, ok := state.ViewWindows[filePath]
	if !ok {
		content, err := state.Skill.FileSystem.ReadFile(filePath)
		if err != nil {
			return utils.Wrapf(err, "failed to read file %q from skill %q", filePath, skillName)
		}
		nonce := GenerateNonce(skillName, filePath)
		vw = NewViewWindow(skillName, filePath, string(content), nonce)
		state.ViewWindows[filePath] = vw
	}

	vw.SetOffset(offset)
	m.contextSizeDirty = true
	return nil
}

// IsSkillLoaded returns true if the given skill is currently loaded in ANY container
// (loadedSkills = persistent-restore + forced 副本, autoLoadedSkills = AI 意图驱动,
// or forcedSkills). This is the unified "is this skill present in the manager" check.
func (m *SkillsContextManager) IsSkillLoaded(skillName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.loadedSkills.Get(skillName); ok {
		return true
	}
	if _, ok := m.autoLoadedSkills.Get(skillName); ok {
		return true
	}
	if m.forcedSkills != nil && m.forcedSkills.Has(skillName) {
		return true
	}
	return false
}

// IsSkillLoadedAndUnfolded returns true if the skill is loaded and currently unfolded (fully visible).
func (m *SkillsContextManager) IsSkillLoadedAndUnfolded(skillName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		return false
	}
	return !state.IsFolded
}

// GetSkillViewSummary returns a human-readable summary of a loaded skill's view window state.
// This is used to inform the AI about what content is already visible in the context.
// Returns empty string if the skill is not loaded.
func (m *SkillsContextManager) GetSkillViewSummary(skillName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		return ""
	}

	var buf bytes.Buffer
	if state.IsFolded {
		buf.WriteString(fmt.Sprintf("Skill '%s' is loaded but FOLDED. Use loading_skills to unfold it.", skillName))
		return buf.String()
	}

	buf.WriteString(fmt.Sprintf("Skill '%s' is loaded and ACTIVE in the SKILLS_CONTEXT section of your prompt. ", skillName))
	buf.WriteString("View Windows:\n")
	for _, vw := range sortedViewWindows(state.ViewWindows) {
		filePath := vw.FilePath
		totalLines := vw.TotalLines()
		offset := vw.GetOffset()
		truncInfo := ""
		if vw.IsTruncated {
			truncInfo = " (truncated, use change_skill_view_offset to see more)"
		}
		buf.WriteString(fmt.Sprintf("  - %s: %d total lines, viewing from line %d%s\n", filePath, totalLines, offset, truncInfo))
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

// Render generates the full skills context string for injection into the prompt.
func (m *SkillsContextManager) Render(nonce string) string {
	if strings.TrimSpace(nonce) == "" {
		nonce = "skills_context"
	}
	return m.renderWithTag(nonce)
}

// RenderStable generates a deterministic skills context block so unchanged skill
// state can participate in prompt prefix caching.
func (m *SkillsContextManager) RenderStable() string {
	return m.renderWithTag("skills_context")
}

// RenderForcedSkills 渲染「用户强制加载」SKILL 段 (满内容, 进 frozen_block 顶部).
// 无 forced skill 时返回空串 (frozen_block 顶部子块不渲染).
// 内容委托 ForcedSkillRegistry.Render, 与本 manager 的 mu 解耦.
//
// 关键词: RenderForcedSkills, frozen_block, 用户强制加载满内容
func (m *SkillsContextManager) RenderForcedSkills() string {
	if m == nil || m.forcedSkills == nil {
		return ""
	}
	return m.forcedSkills.Render()
}

// RenderAutoLoadedSkills 渲染「AI 意图驱动加载」SKILL 段 (进 SemiDynamic 2 尾部).
// 输出形如:
//
//	<|AUTO_LOADED_SKILLS|>
//	=== Skill: X ===
//	<...>
//	=== Skill: Y (folded) ===
//	<...>
//	<|AUTO_LOADED_SKILLS_END|>
//
// 无 auto-loaded skill 时返回空串. 段头/尾用 stable nonce, 保 SemiDynamic 2 前缀缓存.
func (m *SkillsContextManager) RenderAutoLoadedSkills() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.autoLoadedSkills.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString("<|AUTO_LOADED_SKILLS|>\n")
	for _, item := range m.sortedAutoLoadedSkillStates() {
		state := item.state
		if state.IsFolded {
			buf.WriteString(m.renderFolded(state))
		} else {
			buf.WriteString(m.renderFull(state))
		}
		buf.WriteString("\n")
	}
	buf.WriteString("<|AUTO_LOADED_SKILLS_END|>")
	return buf.String()
}

// renderWithTag 输出 SKILLS_CONTEXT 段 (SemiDynamic 1).
//
// 改造后 (默认隐藏目录 + 意图驱动目录 + 三态分离):
//
//	<|SKILLS_CONTEXT_<tag>|>
//	== Currently Loaded Skills ==
//	(loadedSkills 渲染: 持久会话恢复 + forced 登记副本; 或 "(none)")
//
//	== Relevant Skills (intent-matched, top N) ==    // 仅当 catalogVisible 时
//	  - skill-a: ...
//	  - skill-b: ...
//	<|SKILLS_CONTEXT_END_<tag>|>
//
// 默认 catalogVisible=false → 下半段完全不输出 (节约 token).
// autoLoadedSkills 不在此处 (它们进 SemiDynamic 2, 见 RenderAutoLoadedSkills).
// forcedSkills 不在此处 (它们进 frozen_block, 见 RenderForcedSkills).
//
// 关键词: SkillsContext renderWithTag, 默认隐藏目录, catalog top-N, 三态分离
func (m *SkillsContextManager) renderWithTag(tag string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// forced skill 的满内容已通过 RenderForcedSkills 进 frozen_block 顶部,
	// 此处需排除 forced 副本, 避免同一份内容在 SKILLS_CONTEXT 中重复渲染.
	visibleLoaded := make([]namedSkillState, 0, m.loadedSkills.Len())
	for _, item := range m.sortedLoadedSkillStates() {
		if m.forcedSkills != nil && m.forcedSkills.Has(item.name) {
			continue
		}
		visibleLoaded = append(visibleLoaded, item)
	}

	hasLoaded := len(visibleLoaded) > 0
	hasForced := m.forcedSkills != nil && !m.forcedSkills.IsEmpty()
	showCatalog := m.catalogVisible && len(m.catalogSkills) > 0

	// 零状态: 不渲染整段.
	if !hasLoaded && !hasForced && !showCatalog {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", tag))

	buf.WriteString("== Currently Loaded Skills ==\n")
	if hasLoaded {
		for _, item := range visibleLoaded {
			state := item.state
			if state.IsFolded {
				buf.WriteString(m.renderFolded(state))
			} else {
				buf.WriteString(m.renderFull(state))
			}
			buf.WriteString("\n")
		}
	} else {
		buf.WriteString("(none)\n")
	}

	if showCatalog {
		buf.WriteString("\n")
		m.appendCatalogSection(&buf)
	}

	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", tag))
	return buf.String()
}

// appendCatalogSection 写入意图识别驱动的「最相关 SKILL 目录」段 (默认隐藏).
// 内容来自 catalogSkills (意图识别写入的 top-N), 截断到 MaxCatalogSkills.
//
// 关键词: appendCatalogSection, 意图驱动目录, top-N
func (m *SkillsContextManager) appendCatalogSection(buf *bytes.Buffer) {
	skills := m.catalogSkills
	if len(skills) == 0 {
		return
	}
	// 按 name 排序保证字节稳定 (利于缓存), 与旧 Available Skills registry 行为一致.
	sorted := sortSkillMetasByName(skills)
	if len(sorted) > MaxCatalogSkills {
		sorted = sorted[:MaxCatalogSkills]
	}
	buf.WriteString(fmt.Sprintf(CatalogHeader, len(sorted)))
	for _, meta := range sorted {
		buf.WriteString(FormatAvailableSkillRegistryLine(meta))
	}
	buf.WriteString("Use loading_skills action to load any of these when needed.\n")
}

// appendAvailableSkillsSection 写入"Available Skills"下半段。
// 内容仅依赖 loader.AllSkillMetas() + GetSkillSourceStats(loader), 不读 loadedSkills,
// 保证字节稳定性 (registry 不变 -> 输出不变)。
//
// 关键词: SkillsContext appendAvailableSkillsSection, registry listing 稳定
func (m *SkillsContextManager) appendAvailableSkillsSection(buf *bytes.Buffer) {
	skills := m.loader.AllSkillMetas()
	stats := GetSkillSourceStats(m.loader)

	if len(skills) == 0 {
		buf.WriteString("== Available Skills ==\n")
		if stats.DatabaseCount > 0 {
			buf.WriteString(fmt.Sprintf("Available database-backed skills: %d. Use search_capabilities or loading_skills with an exact skill name to access them.\n", stats.DatabaseCount))
		} else {
			buf.WriteString("(none)\n")
		}
		return
	}

	listed, omitted := SelectSkillMetasForPromptRegistry(skills, m.tokenEstimator)
	buf.WriteString(AvailableSkillsRegistryHeader)
	for _, meta := range listed {
		buf.WriteString(FormatAvailableSkillRegistryLine(meta))
	}
	if omitted > 0 {
		buf.WriteString(AvailableSkillsOverflowHint(omitted))
	}
	if stats.DatabaseCount > 0 {
		buf.WriteString(fmt.Sprintf("  ... plus %d database-backed skills available via search.\n", stats.DatabaseCount))
	}
}

// renderFolded renders a skill in folded mode (metadata + compact file tree).
func (m *SkillsContextManager) renderFolded(state *skillContextState) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Skill: %s (folded) ===\n", state.Skill.Meta.Name))
	buf.WriteString(state.Skill.Meta.BriefString())
	buf.WriteString("File Tree:\n")
	buf.WriteString(RenderFileSystemTreeFolded(state.Skill.FileSystem))
	buf.WriteString("\n[Use loading_skills action to expand this skill]\n")
	return buf.String()
}

// renderFull renders a skill in full mode (metadata + full file tree + SKILL.md content).
func (m *SkillsContextManager) renderFull(state *skillContextState) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Skill: %s ===\n", state.Skill.Meta.Name))
	buf.WriteString(state.Skill.Meta.BriefString())

	relatedSkills := DetectCrossSkillReferences(state.Skill.SkillMDContent, state.Skill.Meta.Name)
	if len(relatedSkills) > 0 {
		buf.WriteString(fmt.Sprintf("\nRelated Skills (referenced in SKILL.md): %s\n", strings.Join(relatedSkills, ", ")))
		buf.WriteString("Use loading_skills to load these related skills if needed.\n")
	}

	buf.WriteString("\nFile Tree:\n")
	buf.WriteString(RenderFileSystemTreeFull(state.Skill.FileSystem))

	// Render all active view windows
	for _, vw := range sortedViewWindows(state.ViewWindows) {
		buf.WriteString("\n")
		buf.WriteString(vw.RenderWithInfo())
	}

	return buf.String()
}

type namedSkillState struct {
	name  string
	state *skillContextState
}

func (m *SkillsContextManager) sortedLoadedSkillStates() []namedSkillState {
	items := make([]namedSkillState, 0, m.loadedSkills.Len())
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		items = append(items, namedSkillState{name: name, state: state})
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		left := items[i].name
		right := items[j].name
		if items[i].state != nil && items[i].state.Skill != nil && items[i].state.Skill.Meta != nil && items[i].state.Skill.Meta.Name != "" {
			left = items[i].state.Skill.Meta.Name
		}
		if items[j].state != nil && items[j].state.Skill != nil && items[j].state.Skill.Meta != nil && items[j].state.Skill.Meta.Name != "" {
			right = items[j].state.Skill.Meta.Name
		}
		if left == right {
			return items[i].name < items[j].name
		}
		return left < right
	})
	return items
}

// sortedAutoLoadedSkillStates 返回按名排序的 auto-loaded skill 状态 (用于 RenderAutoLoadedSkills).
func (m *SkillsContextManager) sortedAutoLoadedSkillStates() []namedSkillState {
	items := make([]namedSkillState, 0, m.autoLoadedSkills.Len())
	m.autoLoadedSkills.ForEach(func(name string, state *skillContextState) bool {
		items = append(items, namedSkillState{name: name, state: state})
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		return items[i].name < items[j].name
	})
	return items
}

// ensureAutoSkillsFits 对 autoLoadedSkills 做 LRU 折叠, 控制 SemiDynamic 2 体积.
// 复用 ensureContextFits 的 LRU 策略, 但作用于 autoLoadedSkills 容器 (独立预算).
// 必须在 m.mu 持有时调用.
func (m *SkillsContextManager) ensureAutoSkillsFits() {
	for {
		totalSize := m.estimateAutoSkillsSize()
		if totalSize <= m.maxTokens {
			return
		}
		var lruName string
		var lruTime time.Time
		m.autoLoadedSkills.ForEach(func(name string, state *skillContextState) bool {
			if !state.IsFolded {
				if lruName == "" || state.LastAccessedAt.Before(lruTime) {
					lruName = name
					lruTime = state.LastAccessedAt
				}
			}
			return true
		})
		if lruName == "" {
			log.Warnf("all auto skills folded but still exceed limit (total: %d, limit: %d)", totalSize, m.maxTokens)
			return
		}
		if state, ok := m.autoLoadedSkills.Get(lruName); ok {
			state.IsFolded = true
			log.Infof("folded LRU auto skill %q to fit SemiDynamic 2 (total: %d, limit: %d)", lruName, totalSize, m.maxTokens)
		}
	}
}

// estimateAutoSkillsSize 估算 autoLoadedSkills 的总 token 体积. 必须在 m.mu 持有时调用.
func (m *SkillsContextManager) estimateAutoSkillsSize() int {
	total := 0
	m.autoLoadedSkills.ForEach(func(name string, state *skillContextState) bool {
		var rendered string
		if state.IsFolded {
			rendered = m.renderFolded(state)
		} else {
			rendered = m.renderFull(state)
		}
		total += m.measureSize(rendered)
		return true
	})
	return total
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

// ensureContextFits folds the least-recently-used unfolded skill when context exceeds the limit.
// Must be called with m.mu held.
func (m *SkillsContextManager) ensureContextFits() {
	for {
		totalSize := m.estimateContextSize()
		if totalSize <= m.maxTokens {
			return
		}

		var lruName string
		var lruTime time.Time
		m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
			if !state.IsFolded {
				if lruName == "" || state.LastAccessedAt.Before(lruTime) {
					lruName = name
					lruTime = state.LastAccessedAt
				}
			}
			return true
		})

		if lruName == "" {
			log.Warnf("all skills are folded but context still exceeds limit (total: %d, limit: %d)", totalSize, m.maxTokens)
			return
		}

		if state, ok := m.loadedSkills.Get(lruName); ok {
			state.IsFolded = true
			m.contextSizeDirty = true
			log.Infof("folded LRU skill %q to fit context limit (total: %d, limit: %d)", lruName, totalSize, m.maxTokens)
		}
	}
}

// estimateContextSize returns the total size of the current context.
// Uses a cached value when available to avoid redundant rendering.
// Must be called with m.mu held.
func (m *SkillsContextManager) estimateContextSize() int {
	if !m.contextSizeDirty {
		return m.cachedContextSize
	}

	total := 0
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		var rendered string
		if state.IsFolded {
			rendered = m.renderFolded(state)
		} else {
			rendered = m.renderFull(state)
		}
		total += m.measureSize(rendered)
		return true
	})

	m.cachedContextSize = total
	m.contextSizeDirty = false
	return total
}

// measureSize returns the size of a rendered string in tokens.
func (m *SkillsContextManager) measureSize(rendered string) int {
	return MeasureStringTokens(rendered, m.tokenEstimator)
}

func buildKeywordsString(meta *SkillMeta) string {
	var parts []string
	for k, v := range meta.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, ",")
}
