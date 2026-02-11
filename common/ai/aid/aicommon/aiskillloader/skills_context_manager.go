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
