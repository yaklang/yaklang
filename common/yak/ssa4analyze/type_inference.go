package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/log"
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
	for _, bRaw := range fun.Blocks {
		b, ok := ssa.ToBasicBlock(bRaw)
		if !ok {
			log.Errorf("TypeInference: %s is not a basic block", bRaw.GetName())
			continue
		}
		for _, inst := range b.Insts {
			t.InferenceOnInstruction(inst)
		}
	}
	for _, inst := range t.DeleteInst {
		ssa.DeleteInst(inst)
	}

	hasCall := false
	for _, user := range fun.GetUsers() {
		if _, ok := ssa.ToCall(user); ok {
			hasCall = true
			break
		}
	}
	if hasCall {
		return
	}
	for name, fv := range fun.FreeValues {
		param, ok := ssa.ToParameter(fv)
		if !ok {
			log.Warnf("free value %s is not a parameter", name)
			continue
		}
		if param.GetDefault() != nil {
			continue
		}
		fv.NewError(ssa.Warn, TITAG, FreeValueUndefine(name.GetName()))
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
	case *ssa.Call:
		t.TypeInferenceCall(inst)
	case *ssa.Phi:
		// return t.TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		t.TypeInferenceBinOp(inst)
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
			blockRaw := phi.GetBlock().Preds[index]
			block, ok := blockRaw.(*ssa.BasicBlock)
			if !ok {
				log.Warnf("BUG: block is not *ssa.BasicBlock")
				return true
			}
			return block.Reachable() == -1
		},
	)

	// only first set type, phi will change
	phi.SetType(typs[0])
}

func (t *TypeInference) TypeInferenceCall(call *ssa.Call) {
	iFuncType := call.Method.GetType()
	funcType, ok := iFuncType.(*ssa.FunctionType)
	if !ok {
		return
	}
	args := call.Args
	paramsLen := funcType.ParameterLen

	var typeInferenceFunctionType func(funcType *ssa.FunctionType)

	typeInferenceArgWithParam := func(arg ssa.Value, argTyp ssa.Type, paramTyp ssa.Type) {
		if !ssa.TypeCompare(argTyp, paramTyp) {
			return
		}
		if argFuncType, ok := ssa.ToFunctionType(argTyp); ok {
			paramFuncType, _ := ssa.ToFunctionType(paramTyp)
			// should not override to any function type
			if paramFuncType == nil || paramFuncType.IsAnyFunctionType() {
				return
			}
			arg.SetType(paramTyp)

			argFunc, ok := ssa.ToFunction(arg)
			if !ok {
				return
			}
			argFuncParams := len(argFunc.Params)
			for i := range argFuncType.Parameter {
				if i >= paramFuncType.ParameterLen {
					break
				}
				if i >= argFuncParams {
					break
				}
				argFunc.Params[i].SetType(paramFuncType.Parameter[i])
				argFuncType.Parameter[i] = paramFuncType.Parameter[i]
			}
		} else if argTyp == ssa.GetAnyType() {
			arg.SetType(paramTyp)
		}
	}

	typeInferenceFunctionType = func(funcType *ssa.FunctionType) {
		for i, paramTyp := range funcType.Parameter {
			if i >= len(args) {
				break
			}
			if i == paramsLen-1 && funcType.IsVariadic {
				for j := i; j < len(args); j++ {
					arg := args[j]
					paramTyp, ok := ssa.ToObjectType(paramTyp)
					if ok && paramTyp.Kind == ssa.SliceTypeKind {
						typeInferenceArgWithParam(arg, arg.GetType(), paramTyp.FieldType)
					}
				}
				break
			}

			arg := args[i]
			typeInferenceArgWithParam(arg, arg.GetType(), paramTyp)
		}
	}
	typeInferenceFunctionType(funcType)
}

func (t *TypeInference) TypeInferenceBinOp(bin *ssa.BinOp) {
	if bin == nil || bin.X == nil || bin.Y == nil {
		return
	}
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
		if ssa.IsCompareOpcode(bin.Op) {
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
	if ssa.IsCompareOpcode(bin.Op) {
		bin.SetType(ssa.BasicTypes[ssa.BooleanTypeKind])
		return
	} else {
		bin.SetType(retTyp)
		return
	}
}
