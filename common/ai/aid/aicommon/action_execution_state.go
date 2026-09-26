package aicommon

// SetExecutionValue binds verifier output to this invocation, not the shared
// loop. These values never enter GetParams, replay JSON or streaming parameter
// barriers, and cannot be supplied by the model. A nil value clears the key.
func (a *Action) SetExecutionValue(key string, value any) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if value == nil {
		delete(a.executionValues, key)
		return
	}
	if a.executionValues == nil {
		a.executionValues = make(map[string]any)
	}
	a.executionValues[key] = value
}

// GetExecutionValue does not wait for streamed model parameters. Mutable
// values remain owned by this invocation and must not be shared across actions.
func (a *Action) GetExecutionValue(key string) any {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.executionValues[key]
}
