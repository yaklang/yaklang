package pass

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const (
	fsWeakReturn uint8 = 1 << iota
	fsHasValue
	fsReturnsPtr
	fsAllInt
	fsIsVoid
	fsRetKindShift = 5
	fsRetKindMask  = 7 << fsRetKindShift
)

// FunctionSummary stores compact inter-procedural facts (4 bytes per entry)
// extracted during streaming Pass 1 so Pass 2 and chunked CompileUnit Save can
// run full inter-procedural passes without keeping every function's SSA body resident.
type FunctionSummary struct {
	Arity        uint8
	Flags        uint8
	StringParams uint16 // bit i set when formal param i has string/char* type
}

func (fs FunctionSummary) returnKind() returnKind {
	return returnKind((fs.Flags & fsRetKindMask) >> fsRetKindShift)
}

func (fs *FunctionSummary) setReturnKind(k returnKind) {
	fs.Flags = (fs.Flags &^ fsRetKindMask) | ((uint8(k) & 7) << fsRetKindShift)
}

type returnKind uint8

const (
	retKindUnknown returnKind = iota
	retKindNull
	retKindPtr64
	retKindI64
	retKindAny
)

// ProgramSummary aggregates lightweight whole-program SSA facts across batches.
type ProgramSummary struct {
	Funcs       map[string]FunctionSummary
	Externs     map[string]bool
	UsedResults map[string]bool     // temporary Pass-1 state, released in Finalize
	PtrCallers  map[string][]string // temporary Pass-1 state, released in Finalize
	finalized   bool
}

// NewProgramSummary allocates an empty inter-procedural summary table.
func NewProgramSummary() *ProgramSummary {
	return &ProgramSummary{
		Funcs:       make(map[string]FunctionSummary),
		UsedResults: make(map[string]bool),
		PtrCallers:  make(map[string][]string),
		Externs:     make(map[string]bool),
	}
}

// CollectProgram records summary facts from every function in program.
func (s *ProgramSummary) CollectProgram(program *ssa.Program) {
	if s == nil || program == nil {
		return
	}
	var externs []string
	var funcs []*ssa.Function
	program.Funcs.ForEach(func(name string, fn *ssa.Function) bool {
		if fn == nil {
			return true
		}
		if fn.IsExtern() {
			externs = append(externs, name)
			return true
		}
		funcs = append(funcs, fn)
		return true
	})
	for _, name := range externs {
		s.Externs[name] = true
	}
	s.CollectFunctions(funcs)
}

// CollectFunctions runs local C-definition + local pointer propagation on
// functions (one batch of a fresh program) and records their inter-procedural
// summary facts.
func (s *ProgramSummary) CollectFunctions(functions []*ssa.Function) {
	if s == nil || len(functions) == 0 {
		return
	}
	applyFunctionDefineOn(functions)
	inferTypesOn(functions, nil)

	for _, fn := range functions {
		if fn == nil || fn.IsExtern() {
			continue
		}
		if fn.GetName() == "" {
			continue
		}
		name := fn.GetName()
		fs := FunctionSummary{
			Arity: uint8(usedArity(fn)),
			Flags: fsWeakReturn | fsAllInt,
		}
		var paramMask uint16
		for _, id := range fn.Params {
			val, ok := fn.GetValueById(id)
			if !ok || val == nil {
				continue
			}
			idx := -1
			if p, ok := ssa.ToParameter(val); ok && p != nil && p.FormalParameterIndex >= 0 {
				idx = p.FormalParameterIndex
			}
			if idx >= 0 && idx < 16 && isStringType(val.GetType()) {
				paramMask |= 1 << uint(idx)
			}
		}
		fs.StringParams = paramMask

		returns := functionReturns(fn)
		if len(returns) == 0 {
			fs.Flags &^= fsHasValue | fsAllInt
		} else {
			for _, ret := range returns {
				vals := ret.GetValues()
				if len(vals) == 0 {
					continue
				}
				fs.Flags |= fsHasValue
				for _, v := range vals {
					if !weakReturnValue(v) {
						fs.Flags &^= fsWeakReturn
					}
					typ := v.GetType()
					if isPointerType(typ) || isPointerSSAType(typ) {
						fs.Flags |= fsReturnsPtr
					}
					if typ == nil || !isIntegerSSAType(typ) {
						fs.Flags &^= fsAllInt
					}
				}
			}
		}
		if fs.Flags&fsHasValue == 0 {
			fs.Flags &^= fsAllInt
		}
		s.Funcs[name] = fs

		// Record call usage, extern targets, and calls whose return value can
		// flow into a Return value of fn (directly or via Add/Sub/Phi/TypeCast).
		retPtrDepCalls := returnDependentCallIDs(returns)
		eachCallInFunctions([]*ssa.Function{fn}, func(_ *ssa.Function, call *ssa.Call) {
			calleeName := internalCallName(call)
			if calleeName == "" {
				return
			}
			if callVal, ok := call.GetValueById(call.Method); ok && callVal != nil {
				if calleeFn, ok := ssa.ToFunction(callVal); ok && calleeFn != nil && calleeFn.IsExtern() {
					s.Externs[calleeFn.GetName()] = true
				}
			}
			if call.HasUsers() {
				s.UsedResults[calleeName] = true
			}
			if retPtrDepCalls[call.GetId()] {
				s.PtrCallers[calleeName] = appendUniqueString(s.PtrCallers[calleeName], name)
			}
		})
	}
}

// Merge folds another worker-local ProgramSummary into s before Finalize.
func (s *ProgramSummary) Merge(other *ProgramSummary) {
	if s == nil || other == nil {
		return
	}
	for k, v := range other.Funcs {
		s.Funcs[k] = v
	}
	for k, v := range other.UsedResults {
		if v {
			s.UsedResults[k] = true
		}
	}
	for k, v := range other.Externs {
		if v {
			s.Externs[k] = true
		}
	}
	for callee, callers := range other.PtrCallers {
		for _, c := range callers {
			s.PtrCallers[callee] = appendUniqueString(s.PtrCallers[callee], c)
		}
	}
}

// ReturnsPointer reports whether summary recorded a pointer return for name.
func (s *ProgramSummary) ReturnsPointer(name string) bool {
	if s == nil {
		return false
	}
	fs, ok := s.Funcs[name]
	return ok && fs.Flags&fsReturnsPtr != 0
}

// IsVoid reports whether summary classified name as a void-returning function.
func (s *ProgramSummary) IsVoid(name string) bool {
	if s == nil {
		return false
	}
	fs, ok := s.Funcs[name]
	return ok && fs.Flags&fsIsVoid != 0
}

// ReturnKind returns the finalized return type category for name.
func (s *ProgramSummary) ReturnKind(name string) returnKind {
	if s == nil {
		return retKindUnknown
	}
	return s.Funcs[name].returnKind()
}

// Finalize solves cross-function pointer return propagation and classifies void
// and return types across the whole program summary.
func (s *ProgramSummary) Finalize() {
	if s == nil || s.finalized {
		return
	}
	s.finalized = true

	// Cross-batch internal calls create placeholder extern stubs in isolated Pass-1
	// batches. Strip any symbol whose body was recovered from Externs.
	for name := range s.Funcs {
		delete(s.Externs, name)
	}

	// Seed pointer returners and propagate across PtrCallers via BFS.
	var queue []string
	for name, fs := range s.Funcs {
		if fs.Flags&fsReturnsPtr != 0 {
			queue = append(queue, name)
		}
	}
	for head := 0; head < len(queue); head++ {
		callee := queue[head]
		for _, caller := range s.PtrCallers[callee] {
			fs, ok := s.Funcs[caller]
			if ok && fs.Flags&fsReturnsPtr == 0 {
				fs.Flags |= fsReturnsPtr
				s.Funcs[caller] = fs
				queue = append(queue, caller)
			}
		}
	}

	for name, fs := range s.Funcs {
		if _, known := symbol.Lookup(name); known {
			continue
		}
		hasValue := fs.Flags&fsHasValue != 0
		weakReturn := fs.Flags&fsWeakReturn != 0
		if !hasValue || (weakReturn && !s.UsedResults[name]) {
			fs.Flags |= fsIsVoid
			fs.setReturnKind(retKindNull)
		} else if fs.Flags&fsReturnsPtr != 0 {
			fs.setReturnKind(retKindPtr64)
		} else if fs.Flags&fsAllInt != 0 {
			fs.setReturnKind(retKindI64)
		} else {
			fs.setReturnKind(retKindAny)
		}
		s.Funcs[name] = fs
	}
	// Release raw Pass-1 state once finalized.
	s.PtrCallers = nil
	s.UsedResults = nil
}

// ApplyFunctions runs the requested (or default) SSA preparation passes on
// functions strictly within the current batch, consulting s for cross-function facts.
func (s *ProgramSummary) ApplyFunctions(functions []*ssa.Function, stringAt func(uint64) (string, bool), names ...string) error {
	if len(functions) == 0 {
		return nil
	}
	if len(names) == 0 {
		names = DefaultNames()
	}
	if s != nil && !s.finalized {
		s.Finalize()
	}
	for _, name := range names {
		switch name {
		case "apply-function-define":
			applyFunctionDefineOn(functions)
		case "infer-types":
			var sharedReturnsPtr func(string) bool
			if s != nil {
				sharedReturnsPtr = s.ReturnsPointer
			}
			inferTypesOn(functions, sharedReturnsPtr)
		case "trim-internal-calls":
			s.trimInternalCallsOn(functions)
		case "mark-void-returns":
			s.markVoidReturnsOn(functions)
		case "refine-return-types":
			s.refineReturnTypesOn(functions)
		case "simplify-partial-writes":
			simplifyPartialWritesOn(functions)
		case "redefine-type-value":
			s.redefineTypeValueOn(functions, stringAt)
		default:
			return fmt.Errorf("unknown SSA pass %q", name)
		}
	}
	return nil
}

func (s *ProgramSummary) trimInternalCallsOn(functions []*ssa.Function) {
	localArities := make(map[string]int)
	if s == nil {
		for _, fn := range functions {
			if fn != nil && !fn.IsExtern() {
				localArities[fn.GetName()] = usedArity(fn)
			}
		}
	}
	eachCallInFunctions(functions, func(_ *ssa.Function, call *ssa.Call) {
		name := internalCallName(call)
		if name == "" {
			return
		}
		if _, known := symbol.Lookup(name); known {
			return
		}
		if call.GetVerboseName() == "variadic" {
			return
		}
		var arity int
		var ok bool
		if s != nil {
			var fs FunctionSummary
			if fs, ok = s.Funcs[name]; ok {
				arity = int(fs.Arity)
			}
		} else {
			arity, ok = localArities[name]
		}
		if !ok {
			return
		}
		if arity > 6 {
			arity = 6
		}
		if len(call.Args) > arity {
			call.Args = call.Args[:arity]
		}
	})
}

func (s *ProgramSummary) markVoidReturnsOn(functions []*ssa.Function) {
	if s == nil {
		return
	}
	for _, fn := range functions {
		if fn == nil || fn.IsExtern() || !s.IsVoid(fn.GetName()) {
			continue
		}
		for _, id := range fn.Blocks {
			block, ok := fn.GetBasicBlockByID(id)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := fn.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if ret, ok := instruction.(*ssa.Return); ok && ret != nil {
					ret.Results = nil
				}
			}
		}
		if fn.Type != nil {
			fn.Type.ReturnType = ssa.CreateNullType()
		}
	}
	eachCallInFunctions(functions, func(_ *ssa.Function, call *ssa.Call) {
		if s.IsVoid(internalCallName(call)) {
			call.SetType(ssa.CreateNullType())
		}
	})
}

func (s *ProgramSummary) refineReturnTypesOn(functions []*ssa.Function) {
	if s == nil {
		return
	}
	typeForKind := func(k returnKind) ssa.Type {
		switch k {
		case retKindNull:
			return ssa.CreateNullType()
		case retKindPtr64:
			return ssa.NewBasicType(ssa.NumberTypeKind, "ptr64")
		case retKindI64:
			return ssa.NewBasicType(ssa.NumberTypeKind, "i64")
		case retKindAny:
			return ssa.CreateAnyType()
		default:
			return nil
		}
	}
	for _, fn := range functions {
		if fn == nil || fn.IsExtern() || fn.Type == nil {
			continue
		}
		if k := s.ReturnKind(fn.GetName()); k != retKindUnknown {
			if t := typeForKind(k); t != nil {
				fn.Type.ReturnType = t
			}
		}
	}
	eachCallInFunctions(functions, func(_ *ssa.Function, call *ssa.Call) {
		if k := s.ReturnKind(internalCallName(call)); k != retKindUnknown {
			if t := typeForKind(k); t != nil {
				call.SetType(t)
			}
		}
	})
}

func (s *ProgramSummary) redefineTypeValueOn(functions []*ssa.Function, stringAt func(uint64) (string, bool)) {
	if stringAt == nil {
		return
	}
	eachCallInFunctions(functions, func(function *ssa.Function, call *ssa.Call) {
		definition, knownDefinition := definitionForCall(call)
		name := internalCallName(call)
		var mask uint16
		if s != nil {
			if fs, ok := s.Funcs[name]; ok {
				mask = fs.StringParams
			}
		}
		for index, argID := range call.Args {
			isString := false
			if knownDefinition {
				param, known := parameterAt(definition, index)
				if !known && !definition.Variadic {
					break
				}
				isString = known && param.Type.IsString()
				if !isString && !definition.Variadic {
					continue
				}
			} else if index < 16 {
				isString = mask&(1<<uint(index)) != 0
			}
			if !isString {
				continue
			}
			address, ok := constantAddress(call, argID)
			if !ok {
				continue
			}
			text, ok := stringAt(address)
			if !ok {
				continue
			}
			replacement := stringConstant(function, call.GetBlock(), text)
			if replacement == nil {
				continue
			}
			call.Args[index] = replacement.GetId()
		}
	})
}

func eachCallInFunctions(functions []*ssa.Function, visit func(*ssa.Function, *ssa.Call)) {
	for _, function := range functions {
		if function == nil || function.IsExtern() {
			continue
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if call, ok := ssa.ToCall(instruction); ok && call != nil {
					visit(function, call)
				}
			}
			for _, instructionID := range block.Phis {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if call, ok := ssa.ToCall(instruction); ok && call != nil {
					visit(function, call)
				}
			}
		}
	}
}

func applyFunctionDefineOn(functions []*ssa.Function) {
	eachCallInFunctions(functions, func(_ *ssa.Function, call *ssa.Call) {
		definition, ok := definitionForCall(call)
		if !ok {
			return
		}
		call.SetType(ssaType(definition.Return))
		if !definition.Variadic && len(call.Args) > len(definition.Params) {
			call.Args = call.Args[:len(definition.Params)]
		}
		for index, argID := range call.Args {
			if index >= len(definition.Params) {
				break
			}
			param := definition.Params[index]
			if param.Type.IsVoid() {
				continue
			}
			value, ok := call.GetValueById(argID)
			if !ok || value == nil {
				continue
			}
			value.SetType(ssaType(param.Type))
			if parameter, ok := ssa.ToParameter(value); ok && parameter != nil && param.Type.IsString() && param.Name != "" {
				parameter.SetName(param.Name)
			}
		}
	})
}

func inferTypesOn(functions []*ssa.Function, sharedReturnsPtr func(string) bool) {
	funcs := make([]inferFuncCache, 0, len(functions))
	for _, function := range functions {
		if function != nil && !function.IsExtern() {
			funcs = append(funcs, collectInferFunc(function))
		}
	}
	localPtrs := make(map[string]bool, len(funcs))
	runInferRounds(funcs, localPtrs, sharedReturnsPtr)
}

func simplifyPartialWritesOn(functions []*ssa.Function) {
	for _, function := range functions {
		if function == nil || function.IsExtern() {
			continue
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				or, ok := instruction.(*ssa.BinOp)
				if !ok || or == nil || or.Op != ssa.OpOr {
					continue
				}
				left, right := orBinOpValue(or, or.X), orBinOpValue(or, or.Y)
				if left == nil || right == nil {
					continue
				}
				var low *ssa.BinOp
				if isMaskedOr(or, left, right) {
					low = right
				} else if isMaskedOr(or, right, left) {
					low = left
				}
				if low == nil || low.X == 0 {
					continue
				}
				source, ok := or.GetValueById(low.X)
				if !ok || source == nil {
					continue
				}
				ssa.ReplaceValue(or, source, func(instruction ssa.Instruction) bool {
					return instruction == or
				})
			}
		}
	}
}

// returnDependentCallIDs traces backwards from ret.Results through Add/Sub/Phi/TypeCast
// to identify which Call instruction IDs can supply a pointer return value of the function.
func returnDependentCallIDs(returns []*ssa.Return) map[int64]bool {
	callIDs := make(map[int64]bool)
	if len(returns) == 0 {
		return callIDs
	}
	visited := make(map[int64]bool)
	var work []ssa.Value
	for _, ret := range returns {
		for _, v := range ret.GetValues() {
			if v != nil && !visited[v.GetId()] {
				visited[v.GetId()] = true
				work = append(work, v)
			}
		}
	}
	pushID := func(ref ssa.Instruction, id int64) {
		if ref == nil || id == 0 || visited[id] {
			return
		}
		if v, ok := ref.GetValueById(id); ok && v != nil {
			visited[id] = true
			work = append(work, v)
		}
	}
	for len(work) > 0 {
		v := work[len(work)-1]
		work = work[:len(work)-1]
		switch typed := v.(type) {
		case *ssa.Call:
			callIDs[typed.GetId()] = true
		case *ssa.BinOp:
			if typed.Op == ssa.OpAdd || typed.Op == ssa.OpSub {
				pushID(typed, typed.X)
				pushID(typed, typed.Y)
			}
		case *ssa.TypeCast:
			pushID(typed, typed.Value)
		case *ssa.Phi:
			for _, item := range typed.GetValues() {
				if item != nil && !visited[item.GetId()] {
					visited[item.GetId()] = true
					work = append(work, item)
				}
			}
		}
	}
	return callIDs
}

func appendUniqueString(slice []string, value string) []string {
	for _, existing := range slice {
		if existing == value {
			return slice
		}
	}
	return append(slice, value)
}

// HasInterproceduralPass reports whether any pass name requires cross-function
// summary extraction before per-batch execution.
func HasInterproceduralPass(names []string) bool {
	if len(names) == 0 {
		return true
	}
	for _, name := range names {
		switch strings.TrimSpace(name) {
		case "infer-types", "trim-internal-calls", "mark-void-returns", "refine-return-types", "redefine-type-value":
			return true
		}
	}
	return false
}
