package aicommon

import (
	"fmt"

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
	// (e.g. vuln_verify, python_poc, write_yaklang_code).
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
}

// IsUnknown returns true if the identifier was not found in any registry.
func (r *ResolvedIdentifier) IsUnknown() bool {
	return r.IdentityType == ResolvedAs_Unknown
}

// ResolveIdentifier checks whether the given name exists as a tool, forge, or skill
// and returns a ResolvedIdentifier with the correct action type and a suggestion message.
// This is used as a fallback mechanism when an action fails because the AI used the wrong
// action type for a given identifier (e.g. trying to load a tool as a skill).
//
// Check order: tools -> forges -> skills.
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

	// 1. Check tools
	if c.AiToolManager != nil {
		if _, err := c.AiToolManager.GetToolByName(name); err == nil {
			return &ResolvedIdentifier{
				Name:         name,
				IdentityType: ResolvedAs_Tool,
				ActionType:   schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
				Suggestion: fmt.Sprintf(
					"IMPORTANT: '%s' is NOT a skill or blueprint. It is a TOOL. "+
						"Use @action '%s' with tool_require_payload '%s' instead. "+
						"Do NOT retry the current action.",
					name, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, name,
				),
			}
		}
	}

	// 2. Check forges (AI Blueprints)
	if c.AiForgeManager != nil {
		if _, err := c.AiForgeManager.GetAIForge(name); err == nil {
			return &ResolvedIdentifier{
				Name:         name,
				IdentityType: ResolvedAs_Forge,
				ActionType:   schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT,
				Suggestion: fmt.Sprintf(
					"IMPORTANT: '%s' is NOT a tool or skill. It is an AI Blueprint (forge). "+
						"Use @action '%s' with blueprint_payload '%s' instead. "+
						"Do NOT retry the current action.",
					name, schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT, name,
				),
			}
		}
	}

	// 3. Check skills
	if sl := c.GetSkillLoader(); sl != nil {
		for _, meta := range sl.AllSkillMetas() {
			if meta.Name == name {
				return &ResolvedIdentifier{
					Name:         name,
					IdentityType: ResolvedAs_Skill,
					ActionType:   schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
					Suggestion: fmt.Sprintf(
						"IMPORTANT: '%s' is NOT a tool or blueprint. It is a SKILL. "+
							"Use @action '%s' with skill_name '%s' instead. "+
							"Do NOT retry the current action.",
						name, schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS, name,
					),
				}
			}
		}
	}

	// 4. Unknown - not found in tool/forge/skill registries
	// Note: focused mode loops are checked at the ReActLoop level.
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
