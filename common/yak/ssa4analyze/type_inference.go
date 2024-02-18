package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TITAG = "TypeInference"

type TypeInference struct {
	Finish     map[ssa.Value]struct{}
	DeleteInst []ssa.Instruction
}

func NewTypeInference(config) Analyzer {
	return &TypeInference{
		Finish: make(map[ssa.Value]struct{}),
	}
}

func (t *TypeInference) Run(prog *ssa.Program) {
	prog.EachFunction(func(f *ssa.Function) {
		t.RunOnFunction(f)
	})
}

func (t *TypeInference) RunOnFunction(fun *ssa.Function) {
	t.DeleteInst = make([]ssa.Instruction, 0)
	for _, b := range fun.Blocks {
		for _, inst := range b.Insts {
			t.InferenceOnInstruction(inst)
		}
	}
	for _, inst := range t.DeleteInst {
		ssa.DeleteInst(inst)
	}
}

func (t *TypeInference) InferenceOnInstruction(inst ssa.Instruction) {
	if iv, ok := inst.(ssa.Value); ok {
		t := iv.GetType()
		if utils.IsNil(t) {
			iv.SetType(ssa.BasicTypes[ssa.NullTypeKind])
		}
	}

	switch inst := inst.(type) {
	case *ssa.Phi:
		// return t.TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		t.TypeInferenceBinOp(inst)
	case *ssa.Call:
		t.TypeInferenceCall(inst)
	}
}

func collectTypeFromValues(values []ssa.Value, skip func(int, ssa.Value) bool) []ssa.Type {
	typMap := make(map[ssa.Type]struct{})
	typs := make([]ssa.Type, 0, len(values))
	for index, v := range values {
		// skip function
		if skip(index, v) {
			continue
		}
		// uniq typ
		typ := v.GetType()
		if _, ok := typMap[typ]; !ok {
			typMap[typ] = struct{}{}
			typs = append(typs, typ)
		}
	}
	return typs
}

// if all finish, return false
func (t *TypeInference) checkValuesNotFinish(vs []ssa.Value) bool {
	for _, v := range vs {
		if _, ok := t.Finish[v]; !ok {
			return true
		}
	}
	return false
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
	v.SetType(typ)
	return true
}

func (t *TypeInference) TypeInferencePhi(phi *ssa.Phi) {
	// check
	// TODO: handler Acyclic graph
	if t.checkValuesNotFinish(phi.Edge) {
		return
	}

	// set type
	typs := collectTypeFromValues(
		phi.Edge,
		// // skip unreachable block
		func(index int, value ssa.Value) bool {
			block := phi.GetBlock().Preds[index]
			return block.Reachable() == -1
		},
	)

	// only first set type, phi will change
	phi.SetType(typs[0])
}

func (t *TypeInference) TypeInferenceBinOp(bin *ssa.BinOp) {
	XTyps := bin.X.GetType()
	YTyps := bin.Y.GetType()

	handlerBinOpType := func(x, y ssa.Type) ssa.Type {
		if x == nil {
			return y
		}
		if x.GetTypeKind() == y.GetTypeKind() {
			return x
		}

		if x.GetTypeKind() == ssa.AnyTypeKind {
			return y
		}
		if y.GetTypeKind() == ssa.AnyTypeKind {
			return x
		}

		// if y.GetTypeKind() == ssa.Null {
		if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
			return ssa.BasicTypes[ssa.BooleanTypeKind]
		}
		// }
		return nil
	}
	retTyp := handlerBinOpType(XTyps, YTyps)
	if retTyp == nil {
		// bin.NewError(ssa.Error, TITAG, "this expression type error: x[%s] %s y[%s]", XTyps, ssa.BinaryOpcodeName[bin.Op], YTyps)
		return
	}

	// typ := handler
	if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
		bin.SetType(ssa.BasicTypes[ssa.BooleanTypeKind])
		return
	} else {
		bin.SetType(retTyp)
		return
	}
}

func (t *TypeInference) TypeInferenceCall(c *ssa.Call) {

	// get function type
	funcTyp, ok := ssa.ToFunctionType(c.Method.GetType())
	if !ok {
		return
	}

	sideEffect := funcTyp.SideEffects
	if funcTyp.IsMethod && funcTyp.IsModifySelf {
		sideEffect = append(sideEffect, c.Args[0].GetName())
	}

	// handle FreeValue
	if len(funcTyp.FreeValue) != 0 || len(sideEffect) != 0 {
		c.HandleFreeValue(funcTyp.FreeValue, sideEffect)
	}

	// handle ellipsis, unpack argument
	if c.IsEllipsis {
		// getField := func(object ssa.User, key ssa.Value) *ssa.Field {
		// 	var f *ssa.Field
		// 	if f = ssa.GetField(object, key); f == nil {
		// 		f = ssa.NewFieldOnly(key, object, c.Block)
		// 		ssa.EmitBefore(c, f)
		// 	}
		// 	return f
		// }
		// obj := c.Args[len(c.Args)-1].(ssa.User)
		// num := len(ssa.GetFields(obj))
		// if t, ok := obj.GetType().(*ssa.ObjectType); ok {
		// 	if t.Kind == ssa.Slice {
		// 		num = len(t.Key)
		// 	}
		// }

		// // fields := ssa.GetFields(obj)
		// c.Args[len(c.Args)-1] = getField(obj, ssa.NewConst(0))
		// for i := 1; i < num; i++ {
		// 	c.Args = append(c.Args, getField(obj, ssa.NewConst(i)))
		// }
	}
}
