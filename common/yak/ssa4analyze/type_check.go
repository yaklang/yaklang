package ssa4analyze

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
		case ssa.ErrorType:
			if len(v.GetUsers()) == 0 {
				vs := v.GetAllVariables()
				if len(vs) == 0 && v.GetOpcode() != ssa.OpCall {
					// if `a()//return err` just ignore,
					// but `a()[1] //return int,err` add handler
					if *v.GetRange().SourceCode != "_" {
						v.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
					}
				}
				for _, variable := range vs {
					if variable.Name == "_" {
						continue
					}
					variable.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
				}
			}
		default:
		}
	}

	switch inst := inst.(type) {
	case *ssa.Make:
		// pass; this is top instruction
	case *ssa.Field:
		t.TypeCheckField(inst)
	case *ssa.Update:
		t.TypeCheckUpdate(inst)
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
		if variable := i.GetVariable(inst.GetName()); variable != nil {
			variable.NewError(ssa.Error, TypeCheckTAG, ssa.ValueUndefined(inst.GetName()))
			return true
		} else {
			return false
		}
	}
	var mark func(i ssa.Value)
	mark = func(i ssa.Value) {
		if _, ok := tmp[i]; ok {
			return
		}
		tmp[i] = struct{}{}
		if err(i) {
			return
		}
		for _, user := range i.GetUsers() {
			if phi, ok := ssa.ToPhi(user); ok {
				mark(phi)
			}
		}
	}

	mark(inst)
}

func (t *TypeCheck) TypeCheckCall(c *ssa.Call) {
	funcTyp, ok := c.Method.GetType().(*ssa.FunctionType)
	isMethod := false
	if f, ok := ssa.ToField(c.Method); ok {
		if f.IsMethod {
			isMethod = true
		}
	}
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

	leftLen := len(ssa.GetFields(c))
	// check return number
	objType, ok := funcTyp.ReturnType.(*ssa.ObjectType)
	if !ok {
		// not object type
		if c.Unpack && leftLen != 1 {
			c.NewError(ssa.Error, TypeCheckTAG, CallAssignmentMismatch(leftLen, c.GetType().String()))
		}
		return
	}
	if objType.Combination {
		// a, b, err = fun()
		hasError := false
		rightLen := len(objType.FieldTypes)
		if objType.FieldTypes[len(objType.FieldTypes)-1].GetTypeKind() == ssa.ErrorType {
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
				// a = func() (m * any, error)
				f := ssa.GetField(c, ssa.NewConst(len(objType.FieldTypes)-1))
				if f == nil {
					vs := c.GetAllVariables()
					for _, variable := range vs {
						variable.NewError(ssa.Error, TypeCheckTAG,
							ErrorUnhandledWithType(c.GetType().String()),
						)
					}
				} else {
					if f.GetName() == "" {
						f.SetName(c.GetName() + ".error")
					}
				}
			}
			// 如果未拆包 不需要后续检查
			return
		}

		if leftLen != rightLen {
			if c.IsDropError {
				c.NewError(ssa.Warn, TypeCheckTAG,
					CallAssignmentMismatchDropError(leftLen, c.GetType().String()),
				)

			} else {
				c.NewError(
					ssa.Error, TypeCheckTAG,
					CallAssignmentMismatch(leftLen, c.GetType().String()),
				)
			}
		}
	}
}

func (t *TypeCheck) TypeCheckField(f *ssa.Field) {
	// use interface
	// typ := f.GetType()
	// if typ.GetTypeKind() == ssa.ErrorType {
	// }
}

func (t *TypeCheck) TypeCheckUpdate(u *ssa.Update) {
}
