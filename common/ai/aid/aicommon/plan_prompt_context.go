package aicommon

import "fmt"

// WrapPlanReferenceForPrompt distinguishes framework plan reference data from
// actual USER_INTERACT records. Its HMAC is session/content/kind-bound and uses
// the persisted Timeline key, independently of projection control authority.
func (m *Timeline) WrapPlanReferenceForPrompt(kind, content string) string {
	if content == "" {
		return ""
	}
	switch kind {
	case "PLAN_DEFINITION", "PLAN_RUNTIME_STATE", "PLAN_FACTS", "PLAN_DOCUMENT":
	default:
		panic("invalid plan reference kind") // caller-owned type, never user text
	}
	key := ""
	if m != nil {
		m.mu.RLock()
		key = m.userInputBoundaryKey
		m.mu.RUnlock()
	}
	nonce := userInputBoundaryNonce(key, "plan-reference-v1\x00"+kind+"\x00"+content)
	return fmt.Sprintf("<|REFERENCE_DATA_%s_%s|>\n%s\n<|REFERENCE_DATA_END_%s_%s|>", kind, nonce, content, kind, nonce)
}

// PlanPromptContext separates immutable plan data from live execution state.
// Definition/RuntimeState are framed reference data, never control-tag authority.
// Version is derived from persisted plan structure, not status, current task or time.
type PlanPromptContext struct {
	Version        string
	UserQuery      string
	Definition     string
	RuntimeState   string
	ExecutionRules string
}

// PlanPromptContextProvider is preferred over the legacy mixed user-input split.
type PlanPromptContextProvider interface {
	GetPlanPromptContext() PlanPromptContext
}

// ApplyPlanPromptContext keeps a single definition partition per request. A new
// version replaces its bytes; changing the current node only changes the open tail.
func ApplyPlanPromptContext(m *PromptMaterials, plan PlanPromptContext) {
	if m == nil || plan.Version == "" {
		return
	}
	m.FrozenPartitions = append(m.FrozenPartitions, FrozenBlockPartition{
		ID: "plan_definition", Title: "Plan Definition", Order: 90,
		Content: plan.Definition,
	})
	m.PlanRuntimeState = plan.RuntimeState
	m.PlanExecutionRules = plan.ExecutionRules
}
