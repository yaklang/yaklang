package ssa

type StoredFunctionBuilder struct {
	Current *FunctionBuilder
	Store   *FunctionBuilder
}

func (b *FunctionBuilder) LoadFunctionBuilder(s *FunctionBuilder) {
	b._editor = s._editor
	// b.IncludeStack = s.IncludeStack
	// b.target = s.target
	// b.CurrentBlock = s.CurrentBlock
	// b.CurrentRange = s.CurrentRange
	// b.parentScope = s.parentScope
	b.parentBuilder = s.parentBuilder
	b.MarkedThisClassBlueprint = s.MarkedThisClassBlueprint

	b.Function.Type = s.Function.Type
	// b.Function.FreeValues = s.Function.FreeValues
	// b.Function.ParameterMembers = s.Function.ParameterMembers
	// b.Function.SideEffects = s.Function.SideEffects
	b.Function.Return = s.Function.Return
	// b.Function.Blocks = s.Function.Blocks

	b.SetProgram(s.GetProgram())
}

func (b *FunctionBuilder) StoreFunctionBuilder() *StoredFunctionBuilder {
	fb := &FunctionBuilder{
		Function: &Function{
			anValue: &anValue{
				anInstruction: &anInstruction{
					// fun:          b.anInstruction.fun,
					prog: b.anInstruction.prog,
					// block:        b.anInstruction.block,
					// R:            b.anInstruction.R,
					// name:         b.anInstruction.name,
					// verboseName:  b.anInstruction.verboseName,
					// id:           b.anInstruction.id,
					// isAnnotation: b.anInstruction.isAnnotation,
					// isExtern:     b.anInstruction.isExtern,
					// isFromDB:     b.anInstruction.isFromDB,
				},
				// typ:         b.anValue.typ,
				// userList:    b.anValue.userList,
				// object:      b.anValue.object,
				// key:         b.anValue.key,
				// member:      b.anValue.member,
				// variables:   b.anValue.variables,
				// mask:        b.anValue.mask,
				// pointer:     b.anValue.pointer,
				// reference:   b.anValue.reference,
				// occultation: b.anValue.occultation,
			},
			// lazyBuilder:       b.Function.lazyBuilder,
			// isMethod:          b.Function.isMethod,
			// methodName:        b.Function.methodName,
			Type: b.Function.Type,
			// Params:            b.Function.Params,
			// ParamLength:       b.Function.ParamLength,
			// FreeValues:       b.Function.FreeValues,
			// ParameterMembers: b.Function.ParameterMembers,
			// SideEffects:      b.Function.SideEffects,
			// parent:            b.Function.parent,
			// ChildFuncs:        b.Function.ChildFuncs,
			Return: b.Function.Return,
			// Blocks: b.Function.Blocks,
			// EnterBlock:        b.Function.EnterBlock,
			// ExitBlock:         b.Function.ExitBlock,
			// DeferBlock:        b.Function.DeferBlock,
			// errComment:        b.Function.errComment,
			// scopeId:           b.Function.scopeId,
			// builder:           b.Function.builder,
			// hasEllipsis:       b.Function.hasEllipsis,
			// isGeneric:         b.Function.isGeneric,
			// currentReturnType: b.Function.currentReturnType,
		},
		// ctx:                        b.ctx,
		_editor: b._editor,
		// SupportClosure:             b.SupportClosure,
		// SupportClassStaticModifier: b.SupportClassStaticModifier,
		// SupportClass:               b.SupportClass,
		// IncludeStack: b.IncludeStack,
		// Included:                   b.Included,
		// IsReturn:                   b.IsReturn,
		// RefParameter:               b.RefParameter,
		// target:       b.target,
		// labels:       b.labels,
		CurrentBlock: b.CurrentBlock,
		CurrentRange: b.CurrentRange,
		// CurrentFile:  b.CurrentFile,
		// parentScope:  b.parentScope,
		// DefineFunc:                 b.DefineFunc,
		MarkedFuncName:  b.MarkedFuncName,
		MarkedFuncType:  b.MarkedFuncType,
		MarkedFunctions: b.MarkedFunctions,
		// MarkedVariable:             b.MarkedVariable,
		MarkedThisObject:           b.MarkedThisObject,
		MarkedThisClassBlueprint:   b.MarkedThisClassBlueprint,
		MarkedMemberCallWantMethod: b.MarkedMemberCallWantMethod,
		parentBuilder:              b.parentBuilder,
	}
	return &StoredFunctionBuilder{
		Current: b,
		Store:   fb,
	}
}
