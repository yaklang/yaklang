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

	MaxExtraTools      int
	MaxExtraForges     int
	MaxExtraSkills     int
	MaxExtraFocusModes int
}

// NewExtraCapabilitiesManager creates a new ExtraCapabilitiesManager with default limits.
func NewExtraCapabilitiesManager() *ExtraCapabilitiesManager {
	return &ExtraCapabilitiesManager{
		MaxExtraTools:      10,
		MaxExtraForges:     10,
		MaxExtraSkills:     10,
		MaxExtraFocusModes: 10,
	}
}

func indexToolByName(tools []*aitool.Tool, name string) int {
	for idx, tool := range tools {
		if tool != nil && tool.Name == name {
			return idx
		}
	}
	return -1
}

func indexForgeByName(forges []ExtraForgeInfo, name string) int {
	for idx, forge := range forges {
		if forge.Name == name {
			return idx
		}
	}
	return -1
}

func indexSkillByName(skills []ExtraSkillInfo, name string) int {
	for idx, skill := range skills {
		if skill.Name == name {
			return idx
		}
	}
	return -1
}

func indexFocusModeByName(modes []ExtraFocusModeInfo, name string) int {
	for idx, mode := range modes {
		if mode.Name == name {
			return idx
		}
	}
	return -1
}

func moveToolToLatest(tools []*aitool.Tool, idx int, tool *aitool.Tool) []*aitool.Tool {
	if idx < 0 || idx >= len(tools) {
		return append(tools, tool)
	}
	copy(tools[idx:], tools[idx+1:])
	tools[len(tools)-1] = tool
	return tools
}

func moveForgeToLatest(forges []ExtraForgeInfo, idx int, forge ExtraForgeInfo) []ExtraForgeInfo {
	if idx < 0 || idx >= len(forges) {
		return append(forges, forge)
	}
	copy(forges[idx:], forges[idx+1:])
	forges[len(forges)-1] = forge
	return forges
}

func moveSkillToLatest(skills []ExtraSkillInfo, idx int, skill ExtraSkillInfo) []ExtraSkillInfo {
	if idx < 0 || idx >= len(skills) {
		return append(skills, skill)
	}
	copy(skills[idx:], skills[idx+1:])
	skills[len(skills)-1] = skill
	return skills
}

func moveFocusModeToLatest(modes []ExtraFocusModeInfo, idx int, mode ExtraFocusModeInfo) []ExtraFocusModeInfo {
	if idx < 0 || idx >= len(modes) {
		return append(modes, mode)
	}
	copy(modes[idx:], modes[idx+1:])
	modes[len(modes)-1] = mode
	return modes
}

// AddTools adds tools to the extra capabilities, keeping the newest items and
// moving duplicate names to the newest position.
func (ecm *ExtraCapabilitiesManager) AddTools(tools ...*aitool.Tool) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, t := range tools {
		if t == nil || t.Name == "" {
			continue
		}
		if idx := indexToolByName(ecm.tools, t.Name); idx >= 0 {
			ecm.tools = moveToolToLatest(ecm.tools, idx, t)
			continue
		}
		ecm.tools = append(ecm.tools, t)
		if ecm.MaxExtraTools > 0 && len(ecm.tools) > ecm.MaxExtraTools {
			evicted := ecm.tools[0]
			ecm.tools = ecm.tools[1:]
			if evicted != nil {
				log.Infof("extra capabilities: max tool limit (%d) reached, evict oldest tool: %s, keep newest: %s", ecm.MaxExtraTools, evicted.Name, t.Name)
			}
		}
	}
}

// AddForges adds forges to the extra capabilities, keeping the newest items and
// moving duplicate names to the newest position.
func (ecm *ExtraCapabilitiesManager) AddForges(forges ...ExtraForgeInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, f := range forges {
		if f.Name == "" {
			continue
		}
		if idx := indexForgeByName(ecm.forges, f.Name); idx >= 0 {
			ecm.forges = moveForgeToLatest(ecm.forges, idx, f)
			continue
		}
		ecm.forges = append(ecm.forges, f)
		if ecm.MaxExtraForges > 0 && len(ecm.forges) > ecm.MaxExtraForges {
			evicted := ecm.forges[0]
			ecm.forges = ecm.forges[1:]
			log.Infof("extra capabilities: max forge limit (%d) reached, evict oldest forge: %s, keep newest: %s", ecm.MaxExtraForges, evicted.Name, f.Name)
		}
	}
}

// AddSkills adds skills to the extra capabilities, keeping the newest items and
// moving duplicate names to the newest position.
func (ecm *ExtraCapabilitiesManager) AddSkills(skills ...ExtraSkillInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, s := range skills {
		if s.Name == "" {
			continue
		}
		if idx := indexSkillByName(ecm.skills, s.Name); idx >= 0 {
			ecm.skills = moveSkillToLatest(ecm.skills, idx, s)
			continue
		}
		ecm.skills = append(ecm.skills, s)
		if ecm.MaxExtraSkills > 0 && len(ecm.skills) > ecm.MaxExtraSkills {
			evicted := ecm.skills[0]
			ecm.skills = ecm.skills[1:]
			log.Infof("extra capabilities: max skill limit (%d) reached, evict oldest skill: %s, keep newest: %s", ecm.MaxExtraSkills, evicted.Name, s.Name)
		}
	}
}

// AddFocusModes adds focus modes to the extra capabilities, keeping the newest
// items and moving duplicate names to the newest position.
func (ecm *ExtraCapabilitiesManager) AddFocusModes(modes ...ExtraFocusModeInfo) {
	ecm.mu.Lock()
	defer ecm.mu.Unlock()

	for _, m := range modes {
		if m.Name == "" {
			continue
		}
		if idx := indexFocusModeByName(ecm.focusModes, m.Name); idx >= 0 {
			ecm.focusModes = moveFocusModeToLatest(ecm.focusModes, idx, m)
			continue
		}
		ecm.focusModes = append(ecm.focusModes, m)
		if ecm.MaxExtraFocusModes > 0 && len(ecm.focusModes) > ecm.MaxExtraFocusModes {
			evicted := ecm.focusModes[0]
			ecm.focusModes = ecm.focusModes[1:]
			log.Infof("extra capabilities: max focus mode limit (%d) reached, evict oldest focus mode: %s, keep newest: %s", ecm.MaxExtraFocusModes, evicted.Name, m.Name)
		}
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

func (ecm *ExtraCapabilitiesManager) ListTools() []*aitool.Tool {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	result := make([]*aitool.Tool, len(ecm.tools))
	copy(result, ecm.tools)
	return result
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
		sb.WriteString(fmt.Sprintf("## Additional Tools / 额外工具 (%d/%d, titles marked as Core Tool / High Priority should be preferred)\n", len(ecm.tools), ecm.MaxExtraTools))
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
		sb.WriteString(fmt.Sprintf("## Blueprints / AI Forges / 蓝图 (%d/%d)\n", len(ecm.forges), ecm.MaxExtraForges))
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
		sb.WriteString(fmt.Sprintf("## Skills / 技能 (%d/%d)\n", len(ecm.skills), ecm.MaxExtraSkills))
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
		sb.WriteString(fmt.Sprintf("## Focus Modes / 专注模式 (%d/%d)\n", len(ecm.focusModes), ecm.MaxExtraFocusModes))
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
