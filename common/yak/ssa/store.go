package ssa

type StoredFunctionBuilder struct {
	Current *FunctionBuilder
	Store   *FunctionBuilder
}

func (b *FunctionBuilder) LoadFunctionBuilder(s *FunctionBuilder) {
	b._editor = s._editor
	b.IncludeStack = s.IncludeStack
	b.target = s.target
	b.CurrentBlock = s.CurrentBlock
	b.CurrentRange = s.CurrentRange
	b.parentScope = s.parentScope
	b.parentBuilder = s.parentBuilder

	b.Function.Type = s.Function.Type
	b.Function.FreeValues = s.Function.FreeValues
	b.Function.ParameterMembers = s.Function.ParameterMembers
	b.Function.SideEffects = s.Function.SideEffects
	b.Function.Return = s.Function.Return
	b.Function.Blocks = s.Function.Blocks
	b.Function.FunctionSign = s.FunctionSign

	b.SetProgram(s.GetProgram())
}

func (b *FunctionBuilder) StoreFunctionBuilder() *StoredFunctionBuilder {
	fb := &FunctionBuilder{
		Function: &Function{
			anValue: anValue{
				anInstruction: anInstruction{
					prog: b.anInstruction.prog,
				},
			},
			FunctionSign:     b.FunctionSign,
			Type:             b.Function.Type,
			ParameterMembers: b.Function.ParameterMembers,
			SideEffects:      b.Function.SideEffects,
			Return:           b.Function.Return,
			Blocks:           b.Function.Blocks,
		},
		_editor:       b._editor,
		IncludeStack:  b.IncludeStack,
		target:        b.target,
		labels:        b.labels,
		CurrentBlock:  b.CurrentBlock,
		CurrentRange:  b.CurrentRange,
		CurrentFile:   b.CurrentFile,
		parentScope:   b.parentScope,
		parentBuilder: b.parentBuilder,
	}
	return &StoredFunctionBuilder{
		Current: b,
		Store:   fb,
	}
}
