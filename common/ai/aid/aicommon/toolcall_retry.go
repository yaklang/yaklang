package aicommon

import "fmt"

// ToolCallRetryError rejects a direct proposal before any tool callback ran.
// The owning loop loads the schema and makes a new explicit proposal.
type ToolCallRetryError struct {
	ToolName string
	Reason   string
}

func (e *ToolCallRetryError) Error() string {
	return fmt.Sprintf("reason: %s; retry: load the schema for %q and correct explicit arguments for directly_call_tool; no tool was executed", e.Reason, e.ToolName)
}
