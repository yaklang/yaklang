package aiskillloader

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// SkillsContextMaxBytes is the total size limit for all skills context.
	SkillsContextMaxBytes = 20 * 1024 // 20KB
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
}

// ManagerOption configures SkillsContextManager.
type ManagerOption func(*SkillsContextManager)

// WithManagerDB enables optional DB persistence and BM25 search.
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

// WithManagerMaxBytes sets context max bytes at initialization.
func WithManagerMaxBytes(maxBytes int) ManagerOption {
	return func(m *SkillsContextManager) {
		if maxBytes > 0 {
			m.maxBytes = maxBytes
		}
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

	// maxBytes is the total size limit for the skills context.
	maxBytes int

	// Optional DB and AI callback for manager-level search capabilities.
	db               *gorm.DB
	searchAICallback SkillSearchAICallback
}

// NewSkillsContextManager creates a new SkillsContextManager.
func NewSkillsContextManager(loader SkillLoader, opts ...ManagerOption) *SkillsContextManager {
	m := &SkillsContextManager{
		loader:       loader,
		loadedSkills: omap.NewOrderedMap[string, *skillContextState](map[string]*skillContextState{}),
		maxBytes:     SkillsContextMaxBytes,
	}
	for _, opt := range opts {
		opt(m)
	}
	m.initializeSkillSearchPersistence()
	return m
}

// initializeSkillSearchPersistence prepares optional DB persistence/indexes.
func (m *SkillsContextManager) initializeSkillSearchPersistence() {
	if m.db == nil || m.loader == nil {
		return
	}
	m.db.AutoMigrate(&schema.AISkill{})
	if err := yakit.EnsureAISkillFTS5(m.db); err != nil {
		log.Warnf("failed to setup ai_skills FTS5 index: %v", err)
	}
	if err := m.persistAllMetasToDB(); err != nil {
		log.Warnf("failed to persist initial skills into DB: %v", err)
	}
}

// persistAllMetasToDB syncs all loader metadata to DB with hash deduplication.
func (m *SkillsContextManager) persistAllMetasToDB() error {
	if m.db == nil || m.loader == nil {
		return nil
	}
	for _, meta := range m.loader.AllSkillMetas() {
		fsys, err := m.loader.GetFileSystem(meta.Name)
		if err != nil {
			log.Warnf("failed to get filesystem for skill %q: %v", meta.Name, err)
			continue
		}
		hash := ComputeSkillHash(fsys)
		existing, err := yakit.GetAISkillByName(m.db, meta.Name)
		if err == nil && existing != nil && existing.Hash == hash {
			continue
		}
		skill := &schema.AISkill{
			Name:                   meta.Name,
			Description:            meta.Description,
			License:                meta.License,
			Keywords:               buildKeywordsString(meta),
			Body:                   meta.Body,
			Hash:                   hash,
			DisableModelInvocation: meta.DisableModelInvocation,
		}
		if err := yakit.CreateOrUpdateAISkill(m.db, skill); err != nil {
			log.Warnf("failed to persist skill %q: %v", meta.Name, err)
		}
	}
	return nil
}

// SetMaxBytes sets the total context size limit.
func (m *SkillsContextManager) SetMaxBytes(maxBytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxBytes = maxBytes
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
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil, nil
	}
	var result []*SkillMeta
	for _, meta := range m.loader.AllSkillMetas() {
		nameMatch := strings.Contains(strings.ToLower(meta.Name), query)
		descMatch := strings.Contains(strings.ToLower(meta.Description), query)
		if nameMatch || descMatch {
			result = append(result, meta)
		}
	}
	return result, nil
}

// SearchKeywordBM25 uses SQLite FTS5 BM25 ranking to search skills by keyword.
// If manager has persistent DB configured, search against ai_skills table.
// Otherwise create an in-memory SQLite DB for temporary search.
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

	allMetas := m.loader.AllSkillMetas()
	if len(allMetas) == 0 {
		return nil, nil
	}

	filter := &yakit.AISkillSearchFilter{Keywords: query}
	db := m.db
	if db == nil {
		memDB, err := gorm.Open("sqlite3", ":memory:")
		if err != nil {
			return nil, utils.Wrapf(err, "failed to create in-memory SQLite for BM25 search")
		}
		defer memDB.Close()
		memDB.AutoMigrate(&schema.AISkill{})
		if err := yakit.EnsureAISkillFTS5(memDB); err != nil {
			log.Warnf("failed to setup FTS5 on in-memory DB: %v", err)
		}
		for _, meta := range allMetas {
			skill := &schema.AISkill{
				Name:                   meta.Name,
				Description:            meta.Description,
				License:                meta.License,
				Keywords:               buildKeywordsString(meta),
				Body:                   meta.Body,
				DisableModelInvocation: meta.DisableModelInvocation,
			}
			_ = memDB.Create(skill).Error
		}
		_ = yakit.EnsureAISkillFTS5(memDB)
		db = memDB
	} else {
		_ = m.persistAllMetasToDB()
	}

	results, err := yakit.SearchAISkillBM25(db, filter, limit, 0)
	if err != nil {
		return nil, utils.Wrapf(err, "BM25 search failed")
	}

	metaMap := make(map[string]*SkillMeta, len(allMetas))
	for _, meta := range allMetas {
		metaMap[meta.Name] = meta
	}
	var metas []*SkillMeta
	for _, r := range results {
		if meta, ok := metaMap[r.Name]; ok {
			metas = append(metas, meta)
		}
	}
	return metas, nil
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
// If loading the new skill exceeds the size limit, older skills are folded.
func (m *SkillsContextManager) LoadSkill(skillName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loader == nil {
		return utils.Error("skills context manager: no loader configured")
	}

	// Check if already loaded
	if existing, ok := m.loadedSkills.Get(skillName); ok {
		// If folded, unfold it
		if existing.IsFolded {
			existing.IsFolded = false
			m.ensureContextFits()
		}
		return nil
	}

	// Load the skill
	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return utils.Wrapf(err, "failed to load skill %q", skillName)
	}

	nonce := GenerateNonce(skillName, skillMDFilename)
	skillMDWindow := NewViewWindow(skillName, skillMDFilename, loaded.SkillMDContent, nonce)

	state := &skillContextState{
		Skill:    loaded,
		IsFolded: false,
		ViewWindows: map[string]*ViewWindow{
			skillMDFilename: skillMDWindow,
		},
	}

	m.loadedSkills.Set(skillName, state)
	log.Infof("loaded skill %q into context", skillName)

	// Ensure total context fits within the limit
	m.ensureContextFits()
	return nil
}

// ChangeViewOffset changes the view offset for a file in a loaded skill.
func (m *SkillsContextManager) ChangeViewOffset(skillName, filePath string, offset int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		return utils.Errorf("skill %q is not loaded", skillName)
	}

	if filePath == "" {
		filePath = skillMDFilename
	}

	vw, ok := state.ViewWindows[filePath]
	if !ok {
		// Try to load the file from the skill filesystem
		content, err := state.Skill.FileSystem.ReadFile(filePath)
		if err != nil {
			return utils.Wrapf(err, "failed to read file %q from skill %q", filePath, skillName)
		}
		nonce := GenerateNonce(skillName, filePath)
		vw = NewViewWindow(skillName, filePath, string(content), nonce)
		state.ViewWindows[filePath] = vw
	}

	vw.SetOffset(offset)
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
	for filePath, vw := range state.ViewWindows {
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.loadedSkills.Len() == 0 {
		// If there are registered skills, show a hint
		if m.HasRegisteredSkills() {
			skills := m.loader.AllSkillMetas()
			if len(skills) == 0 {
				return ""
			}
			var buf bytes.Buffer
			buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", nonce))
			buf.WriteString("Available Skills (use loading_skills action to load):\n")
			for _, s := range skills {
				buf.WriteString(fmt.Sprintf("  - %s: %s\n", s.Name, s.Description))
			}
			buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", nonce))
			return buf.String()
		}
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", nonce))

	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state.IsFolded {
			buf.WriteString(m.renderFolded(state))
		} else {
			buf.WriteString(m.renderFull(state))
		}
		buf.WriteString("\n")
		return true
	})

	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", nonce))
	return buf.String()
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
	buf.WriteString("\nFile Tree:\n")
	buf.WriteString(RenderFileSystemTreeFull(state.Skill.FileSystem))

	// Render all active view windows
	for _, vw := range state.ViewWindows {
		buf.WriteString("\n")
		buf.WriteString(vw.RenderWithInfo())
	}

	return buf.String()
}

// ensureContextFits folds oldest skills if the total context exceeds the limit.
// Must be called with m.mu held.
func (m *SkillsContextManager) ensureContextFits() {
	for {
		totalSize := m.estimateContextSize()
		if totalSize <= m.maxBytes {
			return
		}

		// Find the oldest non-folded skill and fold it
		folded := false
		m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
			if !state.IsFolded {
				state.IsFolded = true
				log.Infof("folded skill %q to fit context limit (total: %d, limit: %d)", name, totalSize, m.maxBytes)
				folded = true
				return false // break
			}
			return true
		})

		if !folded {
			// All skills are already folded, nothing more to do
			log.Warnf("all skills are folded but context still exceeds limit (total: %d, limit: %d)", totalSize, m.maxBytes)
			return
		}
	}
}

// estimateContextSize estimates the total bytes of the current context.
// Must be called with m.mu held.
func (m *SkillsContextManager) estimateContextSize() int {
	total := 0
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state.IsFolded {
			total += len(m.renderFolded(state))
		} else {
			total += len(m.renderFull(state))
		}
		return true
	})
	return total
}

func buildKeywordsString(meta *SkillMeta) string {
	var parts []string
	for k, v := range meta.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, ",")
}

/*
package aiskillloader

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const (
	// SkillsContextMaxBytes is the total size limit for all skills context.
	SkillsContextMaxBytes = 20 * 1024 // 20KB
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
}

// SkillsContextManager manages the skills context window in the ReAct loop prompt.
// It maintains loaded skills, handles folding/unfolding, and renders the context.
type SkillsContextManager struct {
	mu sync.RWMutex

	// loader is the skill loader used to load skills on demand.
	loader SkillLoader

	// loadedSkills is an ordered map of loaded skills, keyed by skill name.
	// Ordering is by load time (oldest first).
	loadedSkills *omap.OrderedMap[string, *skillContextState]

	// maxBytes is the total size limit for the skills context.
	maxBytes int
}

// NewSkillsContextManager creates a new SkillsContextManager.
func NewSkillsContextManager(loader SkillLoader) *SkillsContextManager {
	return &SkillsContextManager{
		loader:       loader,
		loadedSkills: omap.NewOrderedMap[string, *skillContextState](map[string]*skillContextState{}),
		maxBytes:     SkillsContextMaxBytes,
	}
}

// SetMaxBytes sets the total context size limit.
func (m *SkillsContextManager) SetMaxBytes(maxBytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxBytes = maxBytes
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

// LoadSkill loads a skill into the context manager.
// If loading the new skill exceeds the size limit, older skills are folded.
func (m *SkillsContextManager) LoadSkill(skillName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loader == nil {
		return utils.Error("skills context manager: no loader configured")
	}

	// Check if already loaded
	if existing, ok := m.loadedSkills.Get(skillName); ok {
		// If folded, unfold it
		if existing.IsFolded {
			existing.IsFolded = false
			m.ensureContextFits()
		}
		return nil
	}

	// Load the skill
	loaded, err := m.loader.LoadSkill(skillName)
	if err != nil {
		return utils.Wrapf(err, "failed to load skill %q", skillName)
	}

	nonce := GenerateNonce(skillName, skillMDFilename)
	skillMDWindow := NewViewWindow(skillName, skillMDFilename, loaded.SkillMDContent, nonce)

	state := &skillContextState{
		Skill:    loaded,
		IsFolded: false,
		ViewWindows: map[string]*ViewWindow{
			skillMDFilename: skillMDWindow,
		},
	}

	m.loadedSkills.Set(skillName, state)
	log.Infof("loaded skill %q into context", skillName)

	// Ensure total context fits within the limit
	m.ensureContextFits()
	return nil
}

// ChangeViewOffset changes the view offset for a file in a loaded skill.
func (m *SkillsContextManager) ChangeViewOffset(skillName, filePath string, offset int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		return utils.Errorf("skill %q is not loaded", skillName)
	}

	if filePath == "" {
		filePath = skillMDFilename
	}

	vw, ok := state.ViewWindows[filePath]
	if !ok {
		// Try to load the file from the skill filesystem
		content, err := state.Skill.FileSystem.ReadFile(filePath)
		if err != nil {
			return utils.Wrapf(err, "failed to read file %q from skill %q", filePath, skillName)
		}
		nonce := GenerateNonce(skillName, filePath)
		vw = NewViewWindow(skillName, filePath, string(content), nonce)
		state.ViewWindows[filePath] = vw
	}

	vw.SetOffset(offset)
	return nil
}

// GetLoader returns the skill loader.
func (m *SkillsContextManager) GetLoader() SkillLoader {
	return m.loader
}

// Render generates the full skills context string for injection into the prompt.
func (m *SkillsContextManager) Render(nonce string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.loadedSkills.Len() == 0 {
		// If there are registered skills, show a hint
		if m.HasRegisteredSkills() {
			skills, err := m.loader.ListSkills()
			if err != nil || len(skills) == 0 {
				return ""
			}
			var buf bytes.Buffer
			buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", nonce))
			buf.WriteString("Available Skills (use loading_skills action to load):\n")
			for _, s := range skills {
				buf.WriteString(fmt.Sprintf("  - %s: %s\n", s.Name, s.Description))
			}
			buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", nonce))
			return buf.String()
		}
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n", nonce))

	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state.IsFolded {
			buf.WriteString(m.renderFolded(state))
		} else {
			buf.WriteString(m.renderFull(state))
		}
		buf.WriteString("\n")
		return true
	})

	buf.WriteString(fmt.Sprintf("<|SKILLS_CONTEXT_END_%s|>", nonce))
	return buf.String()
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
	buf.WriteString("\nFile Tree:\n")
	buf.WriteString(RenderFileSystemTreeFull(state.Skill.FileSystem))

	// Render all active view windows
	for _, vw := range state.ViewWindows {
		buf.WriteString("\n")
		buf.WriteString(vw.RenderWithInfo())
	}

	return buf.String()
}

// ensureContextFits folds oldest skills if the total context exceeds the limit.
// Must be called with m.mu held.
func (m *SkillsContextManager) ensureContextFits() {
	for {
		totalSize := m.estimateContextSize()
		if totalSize <= m.maxBytes {
			return
		}

		// Find the oldest non-folded skill and fold it
		folded := false
		m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
			if !state.IsFolded {
				state.IsFolded = true
				log.Infof("folded skill %q to fit context limit (total: %d, limit: %d)", name, totalSize, m.maxBytes)
				folded = true
				return false // break
			}
			return true
		})

		if !folded {
			// All skills are already folded, nothing more to do
			log.Warnf("all skills are folded but context still exceeds limit (total: %d, limit: %d)", totalSize, m.maxBytes)
			return
		}
	}
}

// estimateContextSize estimates the total bytes of the current context.
// Must be called with m.mu held.
func (m *SkillsContextManager) estimateContextSize() int {
	total := 0
	m.loadedSkills.ForEach(func(name string, state *skillContextState) bool {
		if state.IsFolded {
			total += len(m.renderFolded(state))
		} else {
			total += len(m.renderFull(state))
		}
		return true
	})
	return total
}
*/
