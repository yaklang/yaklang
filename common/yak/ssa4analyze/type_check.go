package ssa4analyze

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"golang.org/x/exp/slices"
)

const TypeCheckTAG ssa.ErrorTag = "TypeCheck"

type TypeCheck struct{}

func NewTypeCheck(config) Analyzer {
	return &TypeCheck{}
}

// Analyze(config, *ssa.Program)
func (t *TypeCheck) Run(prog *ssa.Program) {

	analyzeOnFunction := func(f *ssa.Function) {
		check := func(instId int64) {
			inst, ok := f.GetInstructionById(instId)
			if !ok {
				return
			}
			t.CheckOnInstruction(inst)
		}
		for _, bRaw := range f.Blocks {
			b, ok := f.GetBasicBlockByID(bRaw)
			if !ok || b == nil {
				log.Errorf("TypeCheck: %d is not a basic block", bRaw)
				continue
			}

			for _, phi := range b.Phis {
				check(phi)
			}
			for _, inst := range b.Insts {
				check(inst)
			}
		}
	}

	prog.EachFunction(func(f *ssa.Function) {
		analyzeOnFunction(f)
	})
}

func (t *TypeCheck) CheckOnInstruction(inst ssa.Instruction) {
	var checkError func(value ssa.Value, top ...ssa.Value)
	errorIds := make(map[int64]struct{})
	addError := func(value ssa.Value) {
		if value.IsSideEffect() {
			// skip side effect error
			// if this error not handled, function inner will report error
			return
		}
		_, ok := errorIds[value.GetId()]
		if !ok {
			value.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
			errorIds[value.GetId()] = struct{}{}
		}
	}
	checkError = func(v ssa.Value, top ...ssa.Value) {
		userCount := 0
		for _, user := range v.GetUsers() {
			if len(top) == 0 {
				userCount++
				continue
			}
			for _, value := range top {
				if user.GetId() != value.GetId() {
					userCount++
				}
			}
		}
		phi, isPhi := ssa.ToPhi(v)
		if isPhi {
			//说明phi被处理，不应该去检查上层
			if userCount != 0 {
				return
			}
			//phi没有被处理，检查phi edge里面的每一层
			for _, edge := range phi.Edge {
				//有一个不处理就报错
				edge, ok := phi.GetValueById(edge)
				if !ok {
					continue
				}
				checkError(edge, append(top, phi)...)
			}
			return
		}
		if userCount != 0 {
			return
		}
		vs := v.GetAllVariables()
		if len(vs) == 0 && v.GetOpcode() != ssa.SSAOpcodeCall {
			// if `a()//return err` just ignore,
			// but `a()[1] //return int,err` add handler
			if v.GetRange().GetText() != "_" {
				addError(v)
			}
		}
		if slices.Contains(lo.Keys(vs), "_") {
			return
		}
		// Sort variable names to ensure deterministic iteration order
		// This fixes non-deterministic error reporting when iterating over map
		varNames := lo.Keys(vs)
		slices.Sort(varNames)
		for _, varName := range varNames {
			variable := vs[varName]
			// if is `_` variable
			if variable.GetName() == "_" {
				break
			}
			ret := variable.GetValue()
			addError(ret)
			//variable.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
		}
		return
	}
	if v, ok := inst.(ssa.Value); ok {
		switch v.GetType().GetTypeKind() {
		case ssa.ErrorTypeKind:
			checkError(v)
		case ssa.NullTypeKind:
			// if len(v.GetAllVariables()) != 0 {
			// 	inst.NewError(ssa.Warn, TypeCheckTAG, ssa.ValueIsNull())
			// }
		default:
		}
	}

	switch inst := inst.(type) {
	// case *ssa.ConstInst:
	// case *ssa.BinOp:
	case *ssa.Phi:
		t.TypeCheckPhi(inst)
	case *ssa.Call:
		t.TypeCheckCall(inst)
	case *ssa.Undefined:
		t.TypeCheckUndefine(inst)
	}
}

func (t *TypeCheck) TypeCheckPhi(phi *ssa.Phi) {
	// Type narrowing: if a phi edge comes from a block that returns or is unreachable,
	// we can exclude that edge's type from the phi's type in subsequent blocks.
	// This handles cases like "if config == nil { return }" where config cannot be null after the check.

	phiBlock := phi.GetBlock()
	if phiBlock == nil {
		return
	}

	// Check if any edge comes from a block that returns or is unreachable
	// If so, we can narrow the type by excluding null types from those edges
	orType, ok := ssa.ToOrType(phi.GetType())
	if !ok {
		// Not an OrType, no narrowing needed
		return
	}

	// Get all types in the OrType
	allTypes := orType.GetTypes()
	nullType := ssa.CreateNullType()
	hasNullType := false
	for _, typ := range allTypes {
		if typ.GetTypeKind() == ssa.NullTypeKind {
			hasNullType = true
			break
		}
	}

	if !hasNullType {
		// No null type to narrow, nothing to do
		return
	}

	// Check each edge to see if it comes from a block that returns or is unreachable
	// If a null-typed edge comes from such a block, we can exclude null from the phi's type
	for i, edgeID := range phi.Edge {
		edgeValue, ok := phi.GetValueById(edgeID)
		if !ok || edgeValue == nil {
			continue
		}

		// Check if this edge has null type
		if edgeValue.GetType().GetTypeKind() != ssa.NullTypeKind {
			continue
		}

		// Get the predecessor block for this edge
		if i >= len(phiBlock.Preds) {
			continue
		}
		predBlockID := phiBlock.Preds[i]
		predBlock, ok := phiBlock.GetBasicBlockByID(predBlockID)
		if !ok || predBlock == nil {
			continue
		}

		// Check if the predecessor block has a return statement or is unreachable
		if t.blockHasReturnOrUnreachable(predBlock) {
			// This edge comes from a block that returns, so null cannot flow to subsequent blocks
			// Narrow the phi's type by removing null
			t.narrowPhiType(phi, nullType)
			return
		}
	}
}

func (t *TypeCheck) blockHasReturnOrUnreachable(block *ssa.BasicBlock) bool {
	if block == nil {
		return false
	}

	// Check if block is unreachable
	if block.Reachable() == ssa.BasicBlockUnReachable {
		return true
	}

	// Check if block has a return statement
	// A block has return if it has no successors (finish) or if it contains a Return instruction
	if len(block.Succs) == 0 {
		return true
	}

	// Check if block contains a Return instruction
	for _, instID := range block.Insts {
		inst, ok := block.GetInstructionById(instID)
		if !ok {
			continue
		}
		if _, ok := ssa.ToReturn(inst); ok {
			// Found return statement
			return true
		}
	}

	return false
}

func (t *TypeCheck) narrowPhiType(phi *ssa.Phi, excludeType ssa.Type) {
	orType, ok := ssa.ToOrType(phi.GetType())
	if !ok {
		return
	}

	allTypes := orType.GetTypes()
	var newTypes []ssa.Type
	for _, typ := range allTypes {
		if typ.GetTypeKind() != excludeType.GetTypeKind() {
			newTypes = append(newTypes, typ)
		}
	}

	if len(newTypes) == 0 {
		// Should not happen, but fallback to any type
		phi.SetType(ssa.CreateAnyType())
		return
	}

	if len(newTypes) == 1 {
		phi.SetType(newTypes[0])
	} else {
		phi.SetType(ssa.NewOrType(newTypes...))
	}
}

func (t *TypeCheck) TypeCheckUndefine(inst *ssa.Undefined) {
	tmp := make(map[ssa.Value]struct{})
	err := func(i ssa.Value) bool {
		vs := i.GetAllVariables()
		// Sort variable names to ensure deterministic iteration order
		varNames := lo.Keys(vs)
		slices.Sort(varNames)
		for _, varName := range varNames {
			variable := vs[varName]
			variable.NewError(ssa.Error, TypeCheckTAG, ssa.ValueUndefined(inst.GetName()))
		}
		return true
		// if variable := i.GetVariable(inst.GetName()); variable != nil {
		// 	variable.NewError(ssa.Error, TypeCheckTAG, ssa.ValueUndefined(inst.GetName()))
		// 	return true
		// } else {
		// 	return false
		// }
	}
	var markUndefinedValue func(i ssa.Value)
	markUndefinedValue = func(i ssa.Value) {
		if _, ok := tmp[i]; ok {
			return
		}
		tmp[i] = struct{}{}
		if err(i) {
			return
		}
		for _, user := range i.GetUsers() {
			if phi, ok := ssa.ToPhi(user); ok {
				markUndefinedValue(phi)
			}
		}
	}

	if inst.Kind == ssa.UndefinedValueInValid {
		markUndefinedValue(inst)
	}

	if inst.Kind == ssa.UndefinedMemberInValid {
		obj := inst.GetObject()
		if utils.IsNil(obj) {
			return
		}
		objTyp := obj.GetType()
		if utils.IsNil(objTyp) || objTyp.GetTypeKind() == ssa.AnyTypeKind {
			return
		}
		key := inst.GetKey()
		keyName := ssa.GetKeyString(key)
		if keyName == "" {
			return
		}
		if ssa.IsConstInst(key) {
			want := ssa.TryGetSimilarityKey(ssa.GetAllKey(objTyp), key.String())
			if want != "" {
				inst.NewError(
					ssa.Error, TypeCheckTAG,
					ssa.ExternFieldError("Type", objTyp.String(), key.String(), want),
				)
				return
			}
		}

		inst.NewError(ssa.Error, TypeCheckTAG,
			ssa.InvalidField(objTyp.String(), keyName),
		)
	}
}

func (t *TypeCheck) TypeCheckCall(c *ssa.Call) {
	method, ok := c.GetValueById(c.Method)
	if !ok {
		return
	}
	funcTyp, ok := method.GetType().(*ssa.FunctionType)
	isMethod := false
	if !ok {
		return
	}
	// check argument number
	func() {
		fixedLen := len(funcTyp.Parameter)
		if funcTyp.IsVariadic {
			fixedLen = len(funcTyp.Parameter) - 1
		}
		var gotPara ssa.Types = lo.FilterMap(c.Args, func(argId int64, _ int) (ssa.Type, bool) {
			arg, ok := c.GetValueById(argId)
			if !ok || utils.IsNil(arg) {
				return nil, false
			}
			return arg.GetType(), true
		})
		gotParaLen := len(c.Args)
		funName := ""
		if f, ok := ssa.ToFunction(method); ok {
			funName = f.GetName()
		} else if funcTyp.Name != "" {
			funName = funcTyp.Name
		}

		lengthError := false
		switch {
		case funcTyp.IsVariadic && !c.IsEllipsis:
			// len:  gotParaLen >=  wantParaLen-1
			lengthError = gotParaLen < fixedLen
		case !funcTyp.IsVariadic && c.IsEllipsis:
			if len(gotPara) > 0 && fixedLen > 0 {
				firstArgType := gotPara[0]
				expectedType := funcTyp.Parameter[0]
				sliceType := ssa.NewSliceType(firstArgType)

				if !ssa.TypeCompare(sliceType, expectedType) {
					c.NewError(ssa.Error, TypeCheckTAG,
						ArgumentTypeError(1, sliceType.String(), expectedType.String(), funName),
					)
					return
				}
			}
			lengthError = gotParaLen != fixedLen
		case funcTyp.IsVariadic && c.IsEllipsis:
			// lengthError = gotParaLen != wantParaLen
			// TODO: warn
			lengthError = false
			return // skip type check
		case !funcTyp.IsVariadic && !c.IsEllipsis:
			lengthError = gotParaLen != fixedLen
		}
		if lengthError {
			if gotParaLen != funcTyp.ParameterLen {
				c.NewError(
					ssa.Error, TypeCheckTAG,
					NotEnoughArgument(funName, gotPara.String(), funcTyp.GetParamString()),
				)
				return
			}
			log.Errorf("TypeCheckCall: %s, %s", method.GetVerboseName(),
				"gotParaLen == funcTyp.ParameterLen but no enough argument")
			return
		}
		checkParamType := func(i int, got, want ssa.Type) {
			if !ssa.TypeCompare(got, want) {
				// any just skip
				index := i + 1
				if isMethod {
					index = i
				}
				c.NewError(ssa.Error, TypeCheckTAG,
					ArgumentTypeError(index, got.String(), want.String(), funName),
				)
			}
		}

		// checkFixedParams
		var got, want ssa.Type
		if len(gotPara) < fixedLen {
			// Safety check: avoid index out of range
			return
		}
		for i := 0; i < fixedLen; i++ {
			got = gotPara[i]
			want = funcTyp.Parameter[i]
			checkParamType(i, got, want)
		}

		// checkVariadicParams
		if funcTyp.IsVariadic {
			variadicType := funcTyp.Parameter[len(funcTyp.Parameter)-1]
			objType, ok := ssa.ToObjectType(variadicType)
			if ok {
				if objType.GetTypeKind() == ssa.SliceTypeKind {
					variadicType = objType.FieldType
				}
			}
			for i := fixedLen; i < gotParaLen; i++ {
				got = gotPara[i]
				checkParamType(i, got, variadicType)
			}
		}
	}()
	if len(c.GetAllVariables()) == 0 && len(c.GetUsers()) == 0 {
		return
	}

	// check return number
	objType, ok := funcTyp.ReturnType.(*ssa.ObjectType)
	if !ok {
		return
	}
	if objType.Combination {
		// a, b, err = fun()
		hasError := false
		rightLen := len(objType.FieldTypes)
		if objType.FieldTypes[len(objType.FieldTypes)-1].GetTypeKind() == ssa.ErrorTypeKind {
			if c.IsDropError {
				rightLen--
				hasError = false
			} else {
				hasError = true
			}
		}

		// 如果是 没有拆包 则检查后续错误是否处理
		if !c.Unpack {
			if hasError {
				hasError := true
				for key := range c.GetAllMember() {
					if c, ok := ssa.ToConstInst(key); ok {
						if c.IsNumber() {
							if int(c.Number()) == len(objType.FieldTypes)-1 {
								hasError = false
								break
							}
						}
					}
				}

				if hasError {
					vs := c.GetAllVariables()
					// Sort variable names to ensure deterministic iteration order
					varNames := lo.Keys(vs)
					slices.Sort(varNames)
					for _, varName := range varNames {
						variable := vs[varName]
						variable.NewError(ssa.Error, TypeCheckTAG,
							ErrorUnhandledWithType(c.GetType().String()),
						)
					}
					if c.HasUsers() {
						c.NewError(ssa.Error, TypeCheckTAG,
							ErrorUnhandledWithType(c.GetType().String()),
						)
					}
				}
			}
			// 如果未拆包 不需要后续检查
			return
		}
	}
}
