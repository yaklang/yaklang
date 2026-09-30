package aicommon

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
