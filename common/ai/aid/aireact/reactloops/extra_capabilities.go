package reactloops

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// ExtraForgeInfo holds metadata for a dynamically discovered forge/blueprint.
type ExtraForgeInfo struct {
	Name        string
	VerboseName string
	Description string
}

// ExtraSkillInfo holds metadata for a dynamically discovered skill.
type ExtraSkillInfo struct {
	Name        string
	Description string
}

// ExtraFocusModeInfo holds metadata for a dynamically discovered focus mode.
type ExtraFocusModeInfo struct {
	Name        string
	Description string
}

// ExtraCapabilitiesManager manages dynamically discovered capabilities
// (tools, forges, skills, focus modes) from intent recognition.
// These are rendered as a dedicated section in the prompt, separate from core tools.
type ExtraCapabilitiesManager struct {
	mu sync.RWMutex

	tools      []*aitool.Tool
	forges     []ExtraForgeInfo
	skills     []ExtraSkillInfo
	focusModes []ExtraFocusModeInfo

	// MaxExtraTools limits the number of extra tools. Default: 50.
	MaxExtraTools int

	// track seen names for deduplication
	seenTools      map[string]bool
	seenForges     map[string]bool
	seenSkills     map[string]bool
	seenFocusModes map[string]bool
}

// NewExtraCapabilitiesManager creates a new ExtraCapabilitiesManager with default limits.
func NewExtraCapabilitiesManager() *ExtraCapabilitiesManager {
	return &ExtraCapabilitiesManager{
		MaxExtraTools:  50,
		seenTools:      make(map[string]bool),
		seenForges:     make(map[string]bool),
		seenSkills:     make(map[string]bool),
		seenFocusModes: make(map[string]bool),
	}
}

// AddTools adds tools to the extra capabilities, deduplicating by name
// and respecting MaxExtraTools limit.
func (ecm *ExtraCapabilitiesManager) AddTools(tools ...*aitool.Tool) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, t := range tools {
		if t == nil || t.Name == "" {
			continue
		}
		if ecm.seenTools[t.Name] {
			continue
		}
		if len(ecm.tools) >= ecm.MaxExtraTools {
			log.Infof("extra capabilities: max tool limit (%d) reached, skipping tool: %s", ecm.MaxExtraTools, t.Name)
			break
		}
		ecm.seenTools[t.Name] = true
		ecm.tools = append(ecm.tools, t)
	}
}

// AddForges adds forges to the extra capabilities, deduplicating by name.
func (ecm *ExtraCapabilitiesManager) AddForges(forges ...ExtraForgeInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, f := range forges {
		if f.Name == "" {
			continue
		}
		if ecm.seenForges[f.Name] {
			continue
		}
		ecm.seenForges[f.Name] = true
		ecm.forges = append(ecm.forges, f)
	}
}

// AddSkills adds skills to the extra capabilities, deduplicating by name.
func (ecm *ExtraCapabilitiesManager) AddSkills(skills ...ExtraSkillInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, s := range skills {
		if s.Name == "" {
			continue
		}
		if ecm.seenSkills[s.Name] {
			continue
		}
		ecm.seenSkills[s.Name] = true
		ecm.skills = append(ecm.skills, s)
	}
}

// AddFocusModes adds focus modes to the extra capabilities, deduplicating by name.
func (ecm *ExtraCapabilitiesManager) AddFocusModes(modes ...ExtraFocusModeInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, m := range modes {
		if m.Name == "" {
			continue
		}
		if ecm.seenFocusModes[m.Name] {
			continue
		}
		ecm.seenFocusModes[m.Name] = true
		ecm.focusModes = append(ecm.focusModes, m)
	}
}

// HasCapabilities returns true if any extra capabilities have been added.
func (ecm *ExtraCapabilitiesManager) HasCapabilities() bool {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()

	return len(ecm.tools) > 0 || len(ecm.forges) > 0 ||
		len(ecm.skills) > 0 || len(ecm.focusModes) > 0
}

// ToolCount returns the current number of extra tools.
func (ecm *ExtraCapabilitiesManager) ToolCount() int {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	return len(ecm.tools)
}

// ListForges returns a copy of the current extra forges.
func (ecm *ExtraCapabilitiesManager) ListForges() []ExtraForgeInfo {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	result := make([]ExtraForgeInfo, len(ecm.forges))
	copy(result, ecm.forges)
	return result
}

// ListSkills returns a copy of the current extra skills.
func (ecm *ExtraCapabilitiesManager) ListSkills() []ExtraSkillInfo {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	result := make([]ExtraSkillInfo, len(ecm.skills))
	copy(result, ecm.skills)
	return result
}

// ListFocusModes returns a copy of the current extra focus modes.
func (ecm *ExtraCapabilitiesManager) ListFocusModes() []ExtraFocusModeInfo {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	result := make([]ExtraFocusModeInfo, len(ecm.focusModes))
	copy(result, ecm.focusModes)
	return result
}

// Render produces a formatted text block for the extra capabilities section in the prompt.
// Each category (tools, forges, skills, focus modes) is rendered as a separate subsection.
func (ecm *ExtraCapabilitiesManager) Render(nonce string) string {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()

	if len(ecm.tools) == 0 && len(ecm.forges) == 0 &&
		len(ecm.skills) == 0 && len(ecm.focusModes) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("# Extra Capabilities / 额外能力 (via intent recognition)\n")
	sb.WriteString("Below are additional capabilities discovered based on your intent. They are separate from the core tools above.\n")
	sb.WriteString("以下是根据你的意图自动发现的额外能力，与上方的核心工具列表完全独立。\n\n")

	// Tools
	if len(ecm.tools) > 0 {
		sb.WriteString(fmt.Sprintf("## Additional Tools / 额外工具 (%d/%d)\n", len(ecm.tools), ecm.MaxExtraTools))
		for _, t := range ecm.tools {
			desc := t.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			if t.VerboseName != "" {
				sb.WriteString(fmt.Sprintf("* `%s` (%s): %s\n", t.Name, t.VerboseName, desc))
			} else {
				sb.WriteString(fmt.Sprintf("* `%s`: %s\n", t.Name, desc))
			}
		}
		sb.WriteString("\n")
	}

	// Forges / Blueprints
	if len(ecm.forges) > 0 {
		sb.WriteString("## Blueprints / AI Forges / 蓝图\n")
		for _, f := range ecm.forges {
			desc := f.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			if f.VerboseName != "" {
				sb.WriteString(fmt.Sprintf("* `%s` (%s): %s\n", f.Name, f.VerboseName, desc))
			} else {
				sb.WriteString(fmt.Sprintf("* `%s`: %s\n", f.Name, desc))
			}
		}
		sb.WriteString("\n")
	}

	// Skills
	if len(ecm.skills) > 0 {
		sb.WriteString("## Skills / 技能\n")
		for _, s := range ecm.skills {
			desc := s.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			sb.WriteString(fmt.Sprintf("* `%s`: %s\n", s.Name, desc))
		}
		sb.WriteString("\n")
	}

	// Focus Modes
	if len(ecm.focusModes) > 0 {
		sb.WriteString("## Focus Modes / 专注模式\n")
		for _, m := range ecm.focusModes {
			desc := m.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			sb.WriteString(fmt.Sprintf("* `%s`: %s\n", m.Name, desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
