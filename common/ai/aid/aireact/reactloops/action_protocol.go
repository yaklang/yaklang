package reactloops

// WithFunctionCallActionVariants selects static native variants for this loop.
// It does not register names or bypass visibility/policy. Selection happens
// after options, so an explicit text-mode override works in either option order.
func WithFunctionCallActionVariants() ReActLoopOption {
	return func(r *ReActLoop) {
		r.useFunctionCallActionVariants = true
	}
}

func (r *ReActLoop) actionForProtocol(action *LoopAction) *LoopAction {
	if action != nil && r.functionCallMode && r.useFunctionCallActionVariants {
		if native := action.FunctionCallAction; native != nil && native.ActionType == action.ActionType {
			return native
		}
	}
	return action
}
