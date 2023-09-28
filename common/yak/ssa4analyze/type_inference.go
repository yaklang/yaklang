package ssa4analyze

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TypeInferenceTAG ssa.ErrorTag = "typeInference"

type TypeInference struct {
	Finish    map[ssa.InstructionValue]struct{}
	CheckList []ssa.InstructionValue
}

func init() {
	RegisterAnalyzer(&TypeInference{})
}

// Analyze(config, *ssa.Program)
func (t *TypeInference) Analyze(config config, prog *ssa.Program) {
	t.Finish = make(map[ssa.InstructionValue]struct{})

	var inference func(ssa.InstructionValue)
	inference = func(inst ssa.InstructionValue) {
		if t.InferenceOnInstruction(inst) {
			t.CheckList = append(t.CheckList, inst)
		}
	}

	// dfs: up-down; check and set type (from user to value)
	var check func(ssa.InstructionValue)
	check = func(inst ssa.InstructionValue) {
		t.CheckOnInstruction(inst)
	}

	analyzeOnFunction := func(f *ssa.Function) {
		t.CheckList = make([]ssa.InstructionValue, 0)
		for _, b := range f.Blocks {
			for _, phi := range b.Phis {
				inference(phi)
			}
			for _, inst := range b.Instrs {
				i, ok := inst.(ssa.InstructionValue)
				if !ok {
					continue
				}
				inference(i)
			}
		}
		for _, i := range t.CheckList {
			check(i)
		}
	}

	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {
			analyzeOnFunction(f)
		}
	}
}

func (t *TypeInference) CheckOnInstruction(inst ssa.InstructionValue) bool {
	// if _, ok := t.Finish[inst]; ok {
	// 	return true
	// }

	switch inst := inst.(type) {
	case *ssa.Make:
		// pass; this is top instruction
		return true
	case *ssa.Field:
		return t.TypeCheckField(inst)
	case *ssa.Update:
		return t.TypeCheckUpdate(inst)
	// case *ssa.ConstInst:
	// case *ssa.BinOp:
	case *ssa.Call:
		return t.TypeCheckCall(inst)
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
	//TODO:type kind check should handler interfaceTypeKind
	if t := v.GetType(); t.GetTypeKind() != typ.GetTypeKind() && t.GetTypeKind() != ssa.Any {
		if inst, ok := v.(ssa.Instruction); ok {
			inst.NewError(ssa.Error, TypeInferenceTAG, "type check failed, this shoud be %s", typ)
		}
	}
	v.SetType(typ)
	return true
}

func (t *TypeInference) TypeCheckCall(c *ssa.Call) bool {
	functyp, ok := c.Method.GetType().(*ssa.FunctionType)
	if !ok {
		return false
	}
	if c.GetVariable() == "" {
		return false
	}

	if objType, ok := functyp.ReturnType.(*ssa.ObjectType); ok && objType.Combination {
		// a, b, err = fun()
		rightLen := len(objType.FieldTypes)
		if c.IsDropError {
			rightLen -= 1
		}
		// a = func(); a = func()~
		if rightLen == 1 {
			return false
		}

		leftLen := len(ssa.GetFields(c))
		// a, b = fun()~
		if leftLen != rightLen {
			// a = fun();
			if leftLen == 0 {
				leftLen = 1
			}
			c.NewError(
				ssa.Error, TypeInferenceTAG,
				"assignmemt mismatch: %d variable but return %d values",
				leftLen, rightLen,
			)
		}
	}
	return false
}

func (t *TypeInference) TypeCheckField(f *ssa.Field) bool {
	// use interface
	// if _, ok := t.Finish[f.I]; ok {
	typ := f.GetType()
	// if typ.GetTypeKind() == ssa.ErrorType {
	// }
	switch typ.GetTypeKind() {
	case ssa.ErrorType:
		if len(f.GetUserOnly()) == 0 && f.GetVariable() != "_" {
			f.NewError(ssa.Error, TypeInferenceTAG, "this error not handler")
		}
		return false
	default:
		return false
	}
}

func (t *TypeInference) TypeCheckUpdate(u *ssa.Update) bool {
	if checkType(u.Value, u.Address.GetType()) {
		u.NewError(ssa.Error, TypeInferenceTAG, "type check failed, this shoud be %s", u.Address.GetType())
		return true
	} else {
		return false
	}
}

func (t *TypeInference) InferenceOnInstruction(inst ssa.InstructionValue) bool {
	// if _, ok := t.Finish[inst]; ok {
	// 	return true
	// }
	// set type in ast-builder
	// if typ := inst.GetType(); typ != nil {
	// 	// if typ.GetTypeKind() != ssa.Any {
	// 	// 	t.CheckList = append(t.CheckList, inst)
	// 	// 	return true
	// 	// }
	// } else {
	// 	inst.SetType(ssa.BasicTypes[ssa.Any])
	// }

	switch inst := inst.(type) {
	case *ssa.Phi:
		// return t.TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		// return t.TypeInferenceBinOp(inst)
	case *ssa.Call:
		return t.TypeInferenceCall(inst)
	// case *ssa.Return:
	// 	return t.TypeInferenceReturn(inst)
	// case *ssa.Switch:
	// case *ssa.If:
	case *ssa.Make:
		return t.TypeInferenceInterface(inst)
	case *ssa.Field:
		return t.TypeInferenceField(inst)
	case *ssa.Update:
		// return t.TypeInferenceUpdate(inst)
	case *ssa.Undefine:
		return t.TypeInferenceUndefine(inst)
	}
	return false
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
		inst, ok := v.(ssa.InstructionValue)
		if !ok {
			continue
		}
		if _, ok := t.Finish[inst]; !ok {
			return true
		}
	}
	return false
}

func (t *TypeInference) TypeInferenceUndefine(un *ssa.Undefine) bool {
	un.NewError(ssa.Error, TypeInferenceTAG, "this value undefine:%s", un.GetVariable())
	return false
}

func (t *TypeInference) TypeInferencePhi(phi *ssa.Phi) bool {

	// check
	// TODO: handler Acyclic graph
	if t.checkValuesNotFinish(phi.Edge) {
		return false
	}

	// set type
	typs := collectTypeFromValues(
		phi.Edge,
		// // skip unreachable block
		func(index int, value ssa.Value) bool {
			block := phi.Block.Preds[index]
			return block.Reachable() == -1
		},
	)

	// only first set type, phi will change
	phi.SetType(typs[0])
	return true
}

func (t *TypeInference) TypeInferenceBinOp(bin *ssa.BinOp) bool {
	XTyps := bin.X.GetType()
	YTyps := bin.Y.GetType()
	if t.checkValuesNotFinish([]ssa.Value{bin.X, bin.Y}) {
		return false
	}

	handlerBinOpType := func(x, y ssa.Type) ssa.Type {
		if x == nil {
			return y
		}
		if x.GetTypeKind() == y.GetTypeKind() {
			return x
		}
		if y.GetTypeKind() == ssa.Null {
			if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
				return ssa.BasicTypes[ssa.Boolean]
			}
		}
		return nil
	}
	retTyp := handlerBinOpType(XTyps, YTyps)
	if retTyp == nil {
		bin.NewError(ssa.Error, TypeInferenceTAG, "this expression type error: x[%s] %s y[%s]", XTyps, ssa.BinaryOpcodeName[bin.Op], YTyps)
		return false
	}

	// typ := handler
	if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
		bin.SetType(ssa.BasicTypes[ssa.Boolean])
		return true
	} else {
		bin.SetType(retTyp)
		return true
	}
}

func (t *TypeInference) TypeInferenceInterface(i *ssa.Make) bool {
	// check error type

	// check field finish
	// if t.checkValuesNotFinish(
	// 	lo.MapToSlice(i.Field,
	// 		func(key ssa.Value, v *ssa.Field) ssa.Value {
	// 			return v
	// 		},
	// 	),
	// ) {
	// 	return false
	// }

	// type pair struct {
	// 	key   ssa.Value
	// 	field *ssa.Field
	// }
	// // inference type
	// typ := ssa.NewInterfaceType()
	// // sort by key
	// vs := lo.MapToSlice(i.Field, func(key ssa.Value, v *ssa.Field) pair {
	// 	return pair{key: key, field: v}
	// })
	// // if number, sort
	// sort.Slice(vs, func(i, j int) bool {
	// 	return vs[i].key.String() < vs[j].key.String()
	// })
	// for _, pair := range vs {
	// 	typ.AddField(pair.key, pair.field.GetType())
	// }
	// typ.Finish()
	// i.SetType(ssa.Types{typ)

	return true
}

func (t *TypeInference) TypeInferenceField(f *ssa.Field) bool {
	if t := f.Obj.GetType(); t.GetTypeKind() != ssa.Any {
		if t.GetTypeKind() == ssa.ObjectTypeKind {
			interfaceTyp := f.Obj.GetType().(*ssa.ObjectType)
			fTyp, kTyp := interfaceTyp.GetField(f.Key)
			if fTyp == nil || kTyp == nil {
				if methodTyp := interfaceTyp.GetMethod(f.Key.String()); methodTyp != nil {
					f.SetType(methodTyp)
					f.IsMethod = true
					return true
				} else {
					f.NewError(ssa.Error, TypeInferenceTAG, "type check failed, this field not in interface")
					return false
				}
			}
			// if one change, will next check
			fcheck := checkType(f, fTyp)
			kcheck := checkType(f.Key, kTyp)
			return fcheck || kcheck
		}
	}
	// use update
	vs := lo.Map(f.Update, func(v ssa.Value, i int) ssa.Value {
		switch v := v.(type) {
		case *ssa.Update:
			return v.Value
		default:
			return v
		}
	})

	// check value finish
	// TODO: handler Acyclic Graph
	if t.checkValuesNotFinish(vs) {
		return false
	}

	ts := collectTypeFromValues(
		// f.Update,
		vs,
		func(i int, v ssa.Value) bool { return false },
	)
	if len(ts) != 0 {
		f.SetType(ts[0])
	}
	return true
}
func (t *TypeInference) TypeInferenceCall(c *ssa.Call) bool {
	// TODO: type inference call
	// get function type
	functyp, ok := c.Method.GetType().(*ssa.FunctionType)
	if !ok {
		return false
	}

	if c.GetType() == nil || c.GetType().GetTypeKind() == ssa.Any {
		c.SetType(functyp.ReturnType)
		return true
	} else {
		return false
	}
}

func (t *TypeInference) TypeInferenceReturn(r *ssa.Return) bool {
	// TODO: type inference return
	return false
}

func (t *TypeInference) TypeInferenceUpdate(u *ssa.Update) bool {
	// TODO: type inference update
	return false
}
