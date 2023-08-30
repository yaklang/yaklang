package ssa4analyze

import (
	"sort"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "typeInference"

type TypeInference struct {
	Finish map[ssa.Instruction]struct{}
}

func init() {
	RegisterAnalyzer(&TypeInference{})
}

// Analyze(config, *ssa.Program)
func (t *TypeInference) Analyze(config config, prog *ssa.Program) {
	t.Finish = make(map[ssa.Instruction]struct{})

	// dfs with finish-flag
	var analyzeInst func(ssa.Instruction)
	analyzeInst = func(inst ssa.Instruction) {
		if _, ok := t.Finish[inst]; ok {
			// finished, just return
			return
		}
		if !t.AnalyzeOnInstruction(inst) {
			// not finish, just next
			return
		}
		// finish
		t.Finish[inst] = struct{}{}

		// if this instruction is value; update all user type
		value, ok := inst.(ssa.Value)
		if !ok {
			return
		}
		// dfs from value to user
		for _, user := range value.GetUsers() {
			uInst, ok := user.(ssa.Instruction)
			if !ok {
				continue
			}
			analyzeInst(uInst)
		}
	}

	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {
			for _, b := range f.Blocks {
				for _, phi := range b.Phis {
					analyzeInst(phi)
				}
				for _, inst := range b.Instrs {
					analyzeInst(inst)
				}
			}
		}
	}
}

func (t *TypeInference) AnalyzeOnInstruction(inst ssa.Instruction) bool {
	if _, ok := t.Finish[inst]; ok {
		return true
	}
	switch inst := inst.(type) {
	case *ssa.Phi:
		return t.TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		return t.TypeInferenceBinOp(inst)
	case *ssa.Call:
		return t.TypeInferenceCall(inst)
	case *ssa.Return:
		return t.TypeInferenceReturn(inst)
	// case *ssa.Switch:
	// case *ssa.If:
	case *ssa.Interface:
		return t.TypeInferenceInterface(inst)
	case *ssa.Field:
		return t.TypeInferenceField(inst)
		// case *ssa.Update:
		// return TypeInferenceUpdate(inst)
	}
	return false
}

func collectTypeFromValues(values []ssa.Value, skip func(int, ssa.Value) bool) []ssa.Type {
	typMap := make(map[ssa.Type]struct{})
	typs := make([]ssa.Type, 0, len(values))
	for index, value := range values {
		// skip function
		if skip(index, value) {
			continue
		}
		// uniq typ
		for _, typ := range value.GetType() {
			if _, ok := typMap[typ]; !ok {
				typMap[typ] = struct{}{}
				typs = append(typs, typ)
			}
		}
	}
	return typs
}

// if all finish, return false
func (t *TypeInference) checkValuesNotFinish(vs []ssa.Value) bool {
	for _, v := range vs {
		inst, ok := v.(ssa.Instruction)

		if !ok {
			continue
		}
		if _, ok := t.Finish[inst]; !ok {
			return true
		}
	}
	return false
}

func (t *TypeInference) TypeInferencePhi(phi *ssa.Phi) bool {

	// check
	// if t.checkValuesNotFinish(phi.Edge) {
	// 	return false
	// }

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
	phi.SetType(typs)
	return true
}

func (t *TypeInference) TypeInferenceBinOp(bin *ssa.BinOp) bool {
	XTyps := bin.X.GetType()
	YTyps := bin.Y.GetType()
	if t.checkValuesNotFinish([]ssa.Value{bin.X, bin.Y}) {
		return false
	}

	handlerBinOpType := func(x, y ssa.Types) ssa.Types {
		xmap := make(map[ssa.Type]struct{})
		for _, typ := range x {
			xmap[typ] = struct{}{}
		}
		for _, typ := range y {
			if _, ok := xmap[typ]; ok {
				return ssa.Types{typ}
			}
		}
		return nil
	}
	retTyp := handlerBinOpType(XTyps, YTyps)
	if retTyp == nil {
		bin.NewError(ssa.Error, TAG, "this expression type error: x[%s] %s y[%s]", XTyps, ssa.BinaryOpcodeName[bin.Op], YTyps)
		return false
	}

	// typ := handler
	if bin.Op >= ssa.OpGt && bin.Op <= ssa.OpNotEq {
		bin.SetType(ssa.Types{ssa.BasicTypesKind[ssa.Boolean]})
		return true
	} else {
		bin.SetType(retTyp)
		return true
	}
}

func (t *TypeInference) TypeInferenceInterface(i *ssa.Interface) bool {
	// set type in yak-code
	// TODO: just check
	if len(i.GetType()) != 0 {
		return true
	}

	// check field finish
	if t.checkValuesNotFinish(
		lo.MapToSlice(i.Field,
			func(key ssa.Value, v *ssa.Field) ssa.Value {
				return v
			},
		),
	) {
		return false
	}

	type pair struct {
		key   ssa.Value
		field *ssa.Field
	}
	// inference type
	typ := ssa.NewInterfaceType()
	// sort by key
	vs := lo.MapToSlice(i.Field, func(key ssa.Value, v *ssa.Field) pair {
		return pair{key: key, field: v}
	})
	// if number, sort
	sort.Slice(vs, func(i, j int) bool {
		return vs[i].key.String() < vs[j].key.String()
	})
	for _, pair := range vs {
		typ.AddField(pair.key, pair.field.GetType())
	}
	typ.Finish()
	i.SetType(ssa.Types{typ})

	return true
}

func (t *TypeInference) TypeInferenceField(f *ssa.Field) bool {
	// use interface
	if _, ok := t.Finish[f.I]; ok {
		interfaceTyp := f.I.GetType()[0].(*ssa.InterfaceType)
		f.SetType(interfaceTyp.GetField(f.Key))
		// TODO: check all update type

		return true
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
	if t.checkValuesNotFinish(vs) {
		return false
	}

	f.SetType(
		collectTypeFromValues(
			// f.Update,
			vs,
			func(i int, v ssa.Value) bool { return false }),
	)
	return true
}
func (t *TypeInference) TypeInferenceCall(c *ssa.Call) bool {
	// TODO: type inference call
	return false
}

func (t *TypeInference) TypeInferenceReturn(r *ssa.Return) bool {
	// TODO: type inference return
	return false
}
