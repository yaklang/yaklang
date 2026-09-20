package ssa

// EnterExecutionBuilder enters a private lexical view while sharing the latest
// control-flow position with nested tasks of this function. The owning unit
// must serialize these scopes and close them in stack order.
func (s *StoredFunctionBuilder) EnterExecutionBuilder() (*FunctionBuilder, func()) {
	b := s.NewExecutionBuilder()
	if b == nil {
		return nil, func() {}
	}
	function := b.Function
	parent := function.executionBuilder
	flow := parent
	if flow == nil {
		// A stored lexical scope may refer to a private view whose execution
		// already ended. The function's owner holds the committed CFG state.
		flow = function.builder
	}
	if flow != nil {
		b.CurrentBlock = flow.CurrentBlock
		b.IsReturn = flow.IsReturn
		b.executionInitialBlock = b.CurrentBlock
		b.executionInitialReturn = b.IsReturn
	}
	function.executionBuilder = b
	return b, func() {
		s.CommitExecutionBuilder(b)
		if owner := function.builder; owner != nil {
			owner.CurrentBlock = b.CurrentBlock
			owner.IsReturn = b.IsReturn
		}
		if parent != nil {
			parent.CurrentBlock = b.CurrentBlock
			parent.IsReturn = b.IsReturn
		}
		function.executionBuilder = parent
	}
}
