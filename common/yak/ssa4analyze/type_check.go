package ssa4analyze

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TypeCheckTAG ssa.ErrorTag = "TypeCheck"

type TypeCheck struct {
}

func init() {
	RegisterAnalyzer(&TypeCheck{})
}

// Analyze(config, *ssa.Program)
func (t *TypeCheck) Analyze(config config, prog *ssa.Program) {

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

	if v, ok := inst.(ssa.InstructionValue); ok {
		switch v.GetType().GetTypeKind() {
		case ssa.ErrorType:
			variable := v.GetVariable()
			if len(ssa.GetUserOnly(v)) == 0 && variable != "_" && variable != "" {
				v.NewError(ssa.Error, TypeCheckTAG, ErrorUnhandled())
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
	case *ssa.Undefine:
		t.TypeCheckUndefine(inst)
	}
}

func (t *TypeCheck) TypeCheckUndefine(inst *ssa.Undefine) {
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
		str := ""
		if f, ok := c.Method.(*ssa.Function); ok {
			str = f.Name
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

	// check return number
	if objType, ok := funcTyp.ReturnType.(*ssa.ObjectType); ok && objType.Combination {
		// a, b, err = fun()
		rightLen := len(objType.FieldTypes)
		if c.IsDropError {
			rightLen -= 1
		}
		// a = func(); a = func()~
		if rightLen == 1 {
			return
		}

		leftLen := len(ssa.GetFields(c))
		// a, b = fun()~
		if leftLen != rightLen && leftLen > 1 {
			// a = fun();
			// if leftLen == 0 {
			// 	leftLen = 1
			// }
			c.NewError(
				ssa.Error, TypeCheckTAG,
				CallAssignmentMismatch(leftLen, rightLen),
			)
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
