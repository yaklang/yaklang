package ssa4analyze

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"golang.org/x/exp/slices"
)

const TypeCheckTAG ssa.ErrorTag = "TypeCheck"

type TypeCheck struct {
}

func NewTypeCheck(config) Analyzer {
	return &TypeCheck{}
}

// Analyze(config, *ssa.Program)
func (t *TypeCheck) Run(prog *ssa.Program) {
	check := func(inst ssa.Instruction) {
		t.CheckOnInstruction(inst)
	}

	analyzeOnFunction := func(f *ssa.Function) {
		for _, b := range f.Blocks {
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
	if v, ok := inst.(ssa.Value); ok {
		switch v.GetType().GetTypeKind() {
		case ssa.ErrorTypeKind:
			if len(v.GetUsers()) == 0 {
				vs := v.GetAllVariables()
				if len(vs) == 0 && v.GetOpcode() != ssa.OpCall {
					// if `a()//return err` just ignore,
					// but `a()[1] //return int,err` add handler
					if *v.GetRange().SourceCode != "_" {
						v.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
					}
				}
				if slices.Contains(lo.Keys(vs), "_") {
					break
				}
				for _, variable := range vs {
					// if is `_` variable
					if variable.GetName() == "_" {
						break
					}
					variable.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
				}
			}
		case ssa.NullTypeKind:
			if len(v.GetAllVariables()) != 0 {
				inst.NewError(ssa.Warn, TypeCheckTAG, ssa.ValueIsNull())
			}
		default:
		}
	}

	switch inst := inst.(type) {
	// case *ssa.ConstInst:
	// case *ssa.BinOp:
	case *ssa.Call:
		t.TypeCheckCall(inst)
	case *ssa.Undefined:
		t.TypeCheckUndefine(inst)
	}
}

func (t *TypeCheck) TypeCheckUndefine(inst *ssa.Undefined) {
	tmp := make(map[ssa.Value]struct{})
	err := func(i ssa.Value) bool {
		for _, variable := range i.GetAllVariables() {
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

	if inst.Kind == ssa.UndefinedValue {
		markUndefinedValue(inst)
	}

	if inst.Kind == ssa.UndefinedMemberInValid {

		objTyp := inst.GetObject().GetType()
		key := inst.GetKey()
		if ssa.IsConst(key) {
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
			InvalidField(objTyp.String(), ssa.GetKeyString(inst)),
		)
	}
}

func (t *TypeCheck) TypeCheckCall(c *ssa.Call) {
	funcTyp, ok := c.Method.GetType().(*ssa.FunctionType)
	isMethod := false
	if !ok {
		return
	}
	// check argument number
	func() {
		wantParaLen := len(funcTyp.Parameter)
		var gotPara ssa.Types = lo.Map(c.Args, func(arg ssa.Value, _ int) ssa.Type { return arg.GetType() })
		gotParaLen := len(c.Args)
		funName := ""
		if f, ok := c.Method.(*ssa.Function); ok {
			funName = f.GetName()
		} else if funcTyp.Name != "" {
			funName = funcTyp.Name
		}

		lengthError := false
		switch {
		case funcTyp.IsVariadic && !c.IsEllipsis:
			//len:  gotParaLen >=  wantParaLen-1
			lengthError = gotParaLen < wantParaLen-1
		case !funcTyp.IsVariadic && c.IsEllipsis:
			// error, con't use ellipsis in this function
			lengthError = true
		case funcTyp.IsVariadic && c.IsEllipsis:
			// lengthError = gotParaLen != wantParaLen
			// TODO: warn
			lengthError = false
			return // skip type check
		case !funcTyp.IsVariadic && !c.IsEllipsis:
			lengthError = gotParaLen != wantParaLen
		}
		if lengthError {
			c.NewError(
				ssa.Error, TypeCheckTAG,
				NotEnoughArgument(funName, gotPara.String(), funcTyp.GetParamString()),
			)
			return
		}
		checkParamType := func(i int) {
			if !ssa.TypeCompare(gotPara[i], funcTyp.Parameter[i]) {
				// any just skip
				index := i + 1
				if isMethod {
					index = i
				}
				c.NewError(ssa.Error, TypeCheckTAG,
					ArgumentTypeError(index, gotPara[i].String(), funcTyp.Parameter[i].String(), funName),
				)
			}
		}

		for i := 0; i < wantParaLen; i++ {
			if i == wantParaLen-1 && funcTyp.IsVariadic {
				break // ignore
			}
			checkParamType(i)
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
				rightLen -= 1
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
					if c, ok := ssa.ToConst(key); ok {
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
					for _, variable := range vs {
						variable.NewError(ssa.Error, TypeCheckTAG,
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
