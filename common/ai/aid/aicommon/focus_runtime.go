package aicommon

// FocusRuntime is the narrow, run-scoped capability boundary exposed to a
// server-released Yak Focus. The release owns orchestration while this runtime
// owns enforcement and side effects.
type FocusRuntime interface {
	AuthorizedTarget() string
	Execute(capability string, params map[string]any) (map[string]any, error)
}

// FocusRuntimeProvider stays separate from AICallerConfigIf so downstream
// implementations of the broad caller interface remain source-compatible.
type FocusRuntimeProvider interface {
	GetFocusRuntime() FocusRuntime
}

func FocusRuntimeFromConfig(config any) FocusRuntime {
	provider, ok := config.(FocusRuntimeProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.GetFocusRuntime()
}
