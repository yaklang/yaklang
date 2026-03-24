package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/schema"
)

// ResolvedIdentifierType indicates what kind of resource an identifier resolves to.
type ResolvedIdentifierType string

const (
	// ResolvedAs_Tool means the identifier is a registered AI tool.
	ResolvedAs_Tool ResolvedIdentifierType = "tool"
	// ResolvedAs_Forge means the identifier is a registered AI Blueprint (forge).
	ResolvedAs_Forge ResolvedIdentifierType = "forge"
	// ResolvedAs_Skill means the identifier is a registered AI skill.
	ResolvedAs_Skill ResolvedIdentifierType = "skill"
	// ResolvedAs_FocusedMode means the identifier is a registered focused mode loop
	// (e.g. code_audit_verify, write_yaklang_code).
	// Focused modes are specialized ReAct loops that are entered via task.SetFocusMode(),
	// not through AI actions like loading_skills or require_tool.
	ResolvedAs_FocusedMode ResolvedIdentifierType = "focused_mode"
	// ResolvedAs_Unknown means the identifier was not found in any registry.
	ResolvedAs_Unknown ResolvedIdentifierType = ""
)

// ResolvedIdentifier holds the result of resolving an identifier name
// against the tool, forge, skill, and focused mode loop registries.
type ResolvedIdentifier struct {
	// Name is the original identifier that was resolved.
	Name string
	// IdentityType indicates what the identifier resolves to (tool/forge/skill/focused_mode/unknown).
	IdentityType ResolvedIdentifierType
	// ActionType is the @action string the AI should use for this identifier.
	// e.g. "require_tool", "require_ai_blueprint", "loading_skills", or "".
	// For focused_mode, this is empty since focus mode is entered via task assignment, not AI actions.
	ActionType string
	// Suggestion is a human-readable message for the AI explaining the resolution result
	// and what action to take instead. Designed to be strongly worded to prevent loops.
	Suggestion string
	// Alternatives holds other registries where this identifier also exists.
	// For example, if "recon" is both a forge and a skill, the primary match is forge
	// and the skill entry appears in Alternatives.
	Alternatives []*ResolvedIdentifier
}

// IsUnknown returns true if the identifier was not found in any registry.
func (r *ResolvedIdentifier) IsUnknown() bool {
	return r.IdentityType == ResolvedAs_Unknown
}

// HasAlternative returns true if an alternative of the given type exists.
func (r *ResolvedIdentifier) HasAlternative(t ResolvedIdentifierType) bool {
	for _, alt := range r.Alternatives {
		if alt.IdentityType == t {
			return true
		}
	}
	return false
}

// GetAlternative returns the alternative with the given type, or nil.
func (r *ResolvedIdentifier) GetAlternative(t ResolvedIdentifierType) *ResolvedIdentifier {
	for _, alt := range r.Alternatives {
		if alt.IdentityType == t {
			return alt
		}
	}
	return nil
}

// ResolveIdentifier checks whether the given name exists as a tool, forge, or skill
// and returns a ResolvedIdentifier with the correct action type and a suggestion message.
// When the same name exists in multiple registries (e.g. both forge and skill),
// the primary result follows the priority order (tools -> forges -> skills) and
// other matches are stored in the Alternatives field.
//
// Note: Focused mode loops are NOT checked here because the loop registry lives in
// the reactloops package. Use ReActLoop.ResolveIdentifier() for full resolution
// including focused mode loops.
// Returns ResolvedAs_Unknown if the name is not found in any registry.
func (c *Config) ResolveIdentifier(name string) *ResolvedIdentifier {
	if name == "" {
		return &ResolvedIdentifier{
			Name:         name,
			IdentityType: ResolvedAs_Unknown,
			Suggestion:   "The identifier name is empty. Please provide a valid name.",
		}
	}

	var matches []*ResolvedIdentifier

	// 1. Check tools
	if c.AiToolManager != nil {
		if _, err := c.AiToolManager.GetToolByName(name); err == nil {
			matches = append(matches, &ResolvedIdentifier{
				Name:         name,
				IdentityType: ResolvedAs_Tool,
				ActionType:   schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			})
		}
	}

	// 2. Check forges (AI Blueprints)
	if c.AiForgeManager != nil {
		if _, err := c.AiForgeManager.GetAIForge(name); err == nil {
			matches = append(matches, &ResolvedIdentifier{
				Name:         name,
				IdentityType: ResolvedAs_Forge,
				ActionType:   schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT,
			})
		}
	}

	// 3. Check skills
	if sl := c.GetSkillLoader(); sl != nil {
		if _, err := aiskillloader.LookupSkillMeta(sl, name); err == nil {
			matches = append(matches, &ResolvedIdentifier{
				Name:         name,
				IdentityType: ResolvedAs_Skill,
				ActionType:   schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
			})
		}
	}

	if len(matches) == 0 {
		return &ResolvedIdentifier{
			Name:         name,
			IdentityType: ResolvedAs_Unknown,
			Suggestion: fmt.Sprintf(
				"'%s' does not exist as a tool, AI blueprint, skill, or focused mode. "+
					"Please verify the name is correct. Do NOT retry with the same name. "+
					"Try a different approach or ask for clarification.",
				name,
			),
		}
	}

	primary := matches[0]
	if len(matches) > 1 {
		primary.Alternatives = matches[1:]
	}

	primary.Suggestion = buildResolveSuggestion(name, primary, matches)
	return primary
}

func buildResolveSuggestion(name string, primary *ResolvedIdentifier, allMatches []*ResolvedIdentifier) string {
	if len(allMatches) == 1 {
		return buildSingleSuggestion(name, primary)
	}

	var typeNames []string
	for _, m := range allMatches {
		typeNames = append(typeNames, describeType(m.IdentityType))
	}

	var actionHints []string
	for _, m := range allMatches {
		if m.ActionType != "" {
			actionHints = append(actionHints, fmt.Sprintf("  - %s: use @action '%s'", describeType(m.IdentityType), m.ActionType))
		}
	}

	return fmt.Sprintf(
		"'%s' exists as MULTIPLE types: %s. "+
			"The system resolved it as '%s' by default. "+
			"If this is not the intended type, use the correct action:\n%s",
		name, strings.Join(typeNames, ", "),
		describeType(primary.IdentityType),
		strings.Join(actionHints, "\n"),
	)
}

func buildSingleSuggestion(name string, resolved *ResolvedIdentifier) string {
	switch resolved.IdentityType {
	case ResolvedAs_Tool:
		return fmt.Sprintf(
			"IMPORTANT: '%s' is NOT a skill or blueprint. It is a TOOL. "+
				"Use @action '%s' with tool_require_payload '%s' instead. "+
				"Do NOT retry the current action.",
			name, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, name,
		)
	case ResolvedAs_Forge:
		return fmt.Sprintf(
			"IMPORTANT: '%s' is NOT a tool or skill. It is an AI Blueprint (forge). "+
				"Use @action '%s' with blueprint_payload '%s' instead. "+
				"Do NOT retry the current action.",
			name, schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT, name,
		)
	case ResolvedAs_Skill:
		return fmt.Sprintf(
			"IMPORTANT: '%s' is NOT a tool or blueprint. It is a SKILL. "+
				"Use @action '%s' with skill_name '%s' instead. "+
				"Do NOT retry the current action.",
			name, schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS, name,
		)
	default:
		return ""
	}
}

func describeType(t ResolvedIdentifierType) string {
	switch t {
	case ResolvedAs_Tool:
		return "Tool"
	case ResolvedAs_Forge:
		return "AI Blueprint (forge)"
	case ResolvedAs_Skill:
		return "Skill"
	case ResolvedAs_FocusedMode:
		return "Focus Mode"
	default:
		return "Unknown"
	}
}
