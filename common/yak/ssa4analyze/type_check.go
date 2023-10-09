package ssa4analyze

import (
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

/*
if v.Type !match typ return true
if v.Type match  typ return false
*/
func checkType(v ssa.Value, typ ssa.Type) bool {
	if v.GetType() == nil {
		v.SetType(typ)
		return false
	}
	//TODO:type kind check should handler interfaceTypeKind
	t := v.GetType()
	if t.GetTypeKind() != typ.GetTypeKind() && t.GetTypeKind() != ssa.Any && typ.GetTypeKind() != ssa.Any {
		if inst, ok := v.(ssa.Instruction); ok {
			inst.NewError(ssa.Error, TypeCheckTAG, "type check failed, this should be %s", typ)
		}
	}
	v.SetType(typ)
	return true
}

func (t *TypeCheck) TypeCheckUndefine(inst *ssa.Undefine) {
	inst.NewError(ssa.Error, TypeCheckTAG, "this value undefine:%s", inst.GetVariable())
}

func (t *TypeCheck) TypeCheckCall(c *ssa.Call) {
	funcTyp, ok := c.Method.GetType().(*ssa.FunctionType)
	if !ok {
		return
	}
	if c.GetVariable() == "" {
		return
	}

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
		if leftLen != rightLen {
			// a = fun();
			if leftLen == 0 {
				leftLen = 1
			}
			c.NewError(
				ssa.Error, TypeCheckTAG,
				"assignment mismatch: %d variable but return %d values",
				leftLen, rightLen,
			)
		}
	}
}

func (t *TypeCheck) TypeCheckField(f *ssa.Field) {
	// use interface
	typ := f.GetType()
	// if typ.GetTypeKind() == ssa.ErrorType {
	// }
	switch typ.GetTypeKind() {
	case ssa.ErrorType:
		if len(f.GetUserOnly()) == 0 && f.GetVariable() != "_" {
			f.NewError(ssa.Error, TypeCheckTAG, "this error not handler")
		}
		return
	default:
		return
	}
}

func (t *TypeCheck) TypeCheckUpdate(u *ssa.Update) {
}
