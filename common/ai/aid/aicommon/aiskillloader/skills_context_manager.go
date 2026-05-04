package aiskillloader

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/ytoken"
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
	// Ordering is by load time (oldest first).
	loadedSkills *omap.OrderedMap[string, *skillContextState]

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

// GetCurrentSelectedSkills returns currently loaded (selected) skills.
func (m *SkillsContextManager) GetCurrentSelectedSkills() []*SkillMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	selected := make([]*SkillMeta, 0, m.loadedSkills.Len())
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		selected = append(selected, state.Skill.Meta)
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

// IsSkillLoaded returns true if the given skill is currently loaded (either folded or unfolded).
func (m *SkillsContextManager) IsSkillLoaded(skillName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.loadedSkills.Get(skillName)
	return ok
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

// renderWithTag 输出固定双段结构, 让 0->1 加载只增量改"Currently Loaded"上半段,
// "Available Skills" registry listing 字节稳定。这是 Prompt 按稳定性分层路径下
// SkillsContext 进入 AI_CACHE_FROZEN 块时保持前缀缓存命中的核心前置:
//
//	<|SKILLS_CONTEXT_<tag>|>
//	== Currently Loaded Skills ==
//	(none)                               // 或 sortedLoadedSkillStates 渲染输出
//
//	== Available Skills (use loading_skills action to load) ==
//	  - skill-a: ...
//	  - skill-b: ...
//	  ... and N more skills.             // 大列表时尾段提示
//	  ... plus N database-backed skills.
//	<|SKILLS_CONTEXT_END_<tag>|>
//
// 注意: 上半段在零加载时仅一行 "(none)"; 加载后包含具体内容。这样无论加载与否,
// 整体段落结构 (header / 上半段 / 分隔空行 / 下半段) 都保持。
//
// 关键词: SkillsContext renderWithTag, dual-section, 字节稳定, prefix cache
func (m *SkillsContextManager) renderWithTag(tag string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hasRegistered := m.HasRegisteredSkills()
	hasLoaded := m.loadedSkills.Len() > 0

	if !hasRegistered && !hasLoaded {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", tag))

	buf.WriteString("== Currently Loaded Skills ==\n")
	if hasLoaded {
		for _, item := range m.sortedLoadedSkillStates() {
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

	if hasRegistered {
		buf.WriteString("\n")
		m.appendAvailableSkillsSection(&buf)
	}

	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", tag))
	return buf.String()
}

// appendAvailableSkillsSection 写入"Available Skills"下半段。
// 内容仅依赖 loader.AllSkillMetas() + GetSkillSourceStats(loader), 不读 loadedSkills,
// 保证字节稳定性 (registry 不变 -> 输出不变)。
//
// 关键词: SkillsContext appendAvailableSkillsSection, registry listing 稳定
func (m *SkillsContextManager) appendAvailableSkillsSection(buf *bytes.Buffer) {
	skills := sortSkillMetasByName(m.loader.AllSkillMetas())
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

	buf.WriteString("== Available Skills (use loading_skills action to load) ==\n")
	listed := 0
	for _, s := range skills {
		line := fmt.Sprintf("  - %s: %s\n", s.Name, s.Description)
		if m.measureSize(buf.String())+m.measureSize(line) > MetadataListMaxTokens {
			remaining := len(skills) - listed
			buf.WriteString(fmt.Sprintf("  ... and %d more skills. Use search_capabilities to find specific skills.\n", remaining))
			break
		}
		buf.WriteString(line)
		listed++
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
// Uses the custom tokenEstimator if set, otherwise falls back to ytoken.CalcTokenCount.
func (m *SkillsContextManager) measureSize(rendered string) int {
	if m.tokenEstimator != nil {
		return m.tokenEstimator(rendered)
	}
	return ytoken.CalcTokenCount(rendered)
}

func buildKeywordsString(meta *SkillMeta) string {
	var parts []string
	for k, v := range meta.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, ",")
}
