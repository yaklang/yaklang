package aicommon

// LegionResultRuntime is the narrow, run-scoped capability boundary that
// Legion injects into an AI runtime (Focus release or conversation-audit
// session). It is the only channel through which a server-delivered yak
// script or AI tool may perform network side effects and submit structured
// results (assets, risks) back to Legion. Desktop and client-only runtimes
// never have one, so their risks stay in process-local SQLite.
//
// The runtime is also exposed to yak scripts as the global variable
// `focusRuntime` (see reactloops.serverFocusRuntimeGlobal); that script-facing
// name is kept stable as a cross-repository contract with Legion release
// scripts even though the Go type has been renamed.
type LegionResultRuntime interface {
	AuthorizedTarget() string
	Execute(capability string, params map[string]any) (map[string]any, error)
}

// LegionResultRuntimeProvider stays separate from AICallerConfigIf so
// downstream implementations of the broad caller interface remain
// source-compatible.
type LegionResultRuntimeProvider interface {
	GetLegionResultRuntime() LegionResultRuntime
}

func LegionResultRuntimeFromConfig(config any) LegionResultRuntime {
	provider, ok := config.(LegionResultRuntimeProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.GetLegionResultRuntime()
}

// ManagedInputRuntimePolicy is opt-in for server-managed filesystem sessions.
// It prevents built-in dynamic capabilities from escaping a finite tool set.
type ManagedInputRuntimePolicy interface{ ManagedInputRestricted() bool }

func HasManagedInputRestriction(config any) bool {
	runtime := LegionResultRuntimeFromConfig(config)
	policy, ok := runtime.(ManagedInputRuntimePolicy)
	return ok && policy.ManagedInputRestricted()
}
