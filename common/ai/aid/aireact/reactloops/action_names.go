package reactloops

import "fmt"

// ValidateActionToolNames checks one loop's namespace against the business tool
// catalog. Different loops may intentionally reuse a local action (e.g. finish).
// A tool projected as itself (read_file -> read_file) is not a collision.
func ValidateActionToolNames(actions []*LoopAction, toolNames []string) error {
	tools := make(map[string]bool, len(toolNames))
	for _, name := range toolNames {
		tools[name] = true
	}
	seen := make(map[string]bool, len(actions))
	for _, action := range actions {
		if action == nil {
			continue
		}
		name := action.ActionType
		if !validActionToolName(name) {
			return fmt.Errorf("invalid action tool name %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate action tool name %q", name)
		}
		seen[name] = true
		if tools[name] && action.SourceToolName != name {
			return fmt.Errorf("action %q conflicts with a business tool; register a projection of that tool or choose a distinct action name", name)
		}
	}
	return nil
}
