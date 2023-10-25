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

	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {
			analyzeOnFunction(f)
		}
	}
}

func (t *TypeCheck) CheckOnInstruction(inst ssa.Instruction) {
	if v, ok := inst.(ssa.Value); ok {
		switch v.GetType().GetTypeKind() {
		case ssa.ErrorType:
			variable := v.GetVariable()
			if len(v.GetUsers()) == 0 && variable != "_" && variable != "" {
				if pos := v.GetLeftPosition(); pos != nil {
					v.GetFunc().NewErrorWithPos(ssa.Error, TypeCheckTAG, v.GetLeftPosition(), ErrorUnhandled())
				} else {
					v.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
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
	inst.NewError(ssa.Error, TypeCheckTAG, ValueUndefined(inst.GetVariable()))
}

func (t *TypeCheck) TypeCheckCall(c *ssa.Call) {
	funcTyp, ok := c.Method.GetType().(*ssa.FunctionType)
	if !ok {
		return
	}
	// check argument number
	func() {
		wantParaLen := len(funcTyp.Parameter)
		var gotPara ssa.Types = lo.Map(c.Args, func(arg ssa.Value, _ int) ssa.Type { return arg.GetType() })
		gotParaLen := len(c.Args)
		// not match
		if wantParaLen == gotParaLen {
			return
		}
		if funcTyp.IsVariadic {
			// not match minimum length
			if gotParaLen >= (wantParaLen - 1) {
				return
			}
		}
		// call f (a ... )
		if c.IsEllipsis {
			return
		}
		str := ""
		if f, ok := c.Method.(*ssa.Function); ok {
			str = f.GetVariable()
		} else if funcTyp.Name != "" {
			str = funcTyp.Name
		}
		c.NewError(
			ssa.Error, TypeCheckTAG,
			NotEnoughArgument(str, gotPara.String(), funcTyp.Parameter.String()),
		)
	}()

	if c.GetVariable() == "" {
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
					c.GetFunc().NewErrorWithPos(ssa.Error, TypeCheckTAG, c.GetLeftPosition(),
						ErrorUnhandledWithType(c.GetType().String()),
					)
				} else {
					if f.GetVariable() == "" {
						f.SetVariable(c.GetVariable() + ".error")
					}
				}
			}
			// 如果未拆包 不需要后续检查
			return
		}

		if leftLen != rightLen {
			if c.IsDropError {
				c.NewError(ssa.Error, TypeCheckTAG,
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
