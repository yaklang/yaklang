package reactloops

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// ResolveIdentifier resolves an identifier name against ALL registries:
// tools, forges, skills (via Config), and focused mode loops (via the loop registry).
//
// Resolution order: tools -> forges -> skills -> focused mode loops.
//
// This method extends Config.ResolveIdentifier by additionally checking the
// reactloops loop registry (RegisterLoopFactory). This is necessary because
// the loop registry lives in the reactloops package and is not accessible from
// aicommon.Config directly.
//
// This method is the primary entry point for action handlers that need to resolve
// an identifier and provide helpful AI feedback.
func (r *ReActLoop) ResolveIdentifier(name string) *aicommon.ResolvedIdentifier {
	// First, delegate to Config for tool/forge/skill resolution
	if cfg, ok := r.config.(*aicommon.Config); ok {
		result := cfg.ResolveIdentifier(name)
		if !result.IsUnknown() {
			return result
		}
	}

	// Config returned Unknown, or config type assertion failed.
	// Check the focused mode loop registry as a final fallback.
	if _, ok := GetLoopFactory(name); ok {
		meta, _ := GetLoopMetadata(name)
		description := ""
		if meta != nil && meta.Description != "" {
			description = " Description: " + meta.Description
		}
		return &aicommon.ResolvedIdentifier{
			Name:         name,
			IdentityType: aicommon.ResolvedAs_FocusedMode,
			ActionType:   "", // focused mode is not entered via AI actions
			Suggestion: fmt.Sprintf(
				"IMPORTANT: '%s' is a Focused Mode Loop (专注模式), NOT a skill, tool, or blueprint.%s "+
					"Focused modes are entered automatically when assigned to a task. "+
					"You CANNOT enter this mode via loading_skills, require_tool, or require_ai_blueprint. "+
					"Do NOT retry the current action with this name. "+
					"If you need capabilities from this focused mode, describe your needs directly instead.",
				name, description,
			),
		}
	}

	// Truly unknown
	return &aicommon.ResolvedIdentifier{
		Name:         name,
		IdentityType: aicommon.ResolvedAs_Unknown,
		Suggestion: fmt.Sprintf(
			"'%s' does not exist as a tool, AI blueprint, skill, or focused mode. "+
				"Please verify the name is correct. Do NOT retry with the same name. "+
				"Try a different approach or ask for clarification.",
			name,
		),
	}
}
