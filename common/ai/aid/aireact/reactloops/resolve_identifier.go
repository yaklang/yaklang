package reactloops

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// ResolveIdentifier resolves an identifier name against ALL registries:
// tools, focused mode loops (via the loop registry), forges, and skills (via Config).
//
// Resolution order: tools -> focused mode loops -> forges -> skills.
//
// Focused mode loops take priority over forges, ensuring that if an identifier
// happens to match both a loop and a forge, it is always dispatched via
// handleLoadFocusMode rather than handleLoadForge. In practice, loop names and
// forge names should be kept distinct to avoid ambiguity.
//
// This method extends Config.ResolveIdentifier by additionally checking the
// reactloops loop registry (RegisterLoopFactory). This is necessary because
// the loop registry lives in the reactloops package and is not accessible from
// aicommon.Config directly.
//
// This method is the primary entry point for action handlers that need to resolve
// an identifier and provide helpful AI feedback.
func (r *ReActLoop) ResolveIdentifier(name string) *aicommon.ResolvedIdentifier {
	// 1. Check tools first (highest priority – direct executable capability).
	if cfg, ok := r.config.(*aicommon.Config); ok {
		if toolResult := cfg.ResolveIdentifierToolOnly(name); toolResult != nil && !toolResult.IsUnknown() {
			return toolResult
		}
	}

	// 2. Check focused mode loops BEFORE forges.
	// Loops take priority to ensure focus mode is always entered directly.
	if _, ok := GetLoopFactory(name); ok {
		meta, _ := GetLoopMetadata(name)
		description := ""
		if meta != nil && meta.Description != "" {
			description = " Description: " + meta.Description
		}
		return &aicommon.ResolvedIdentifier{
			Name:         name,
			IdentityType: aicommon.ResolvedAs_FocusedMode,
			ActionType:   schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY,
			Suggestion: fmt.Sprintf(
				"'%s' is a Focused Mode Loop (专注模式).%s "+
					"Use load_capability to enter this mode directly.",
				name, description,
			),
		}
	}

	// 3. Fall back to Config for forge/skill resolution.
	if cfg, ok := r.config.(*aicommon.Config); ok {
		result := cfg.ResolveIdentifier(name)
		if !result.IsUnknown() {
			return result
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
