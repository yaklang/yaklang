package ssa4analyze

import "github.com/yaklang/yaklang/common/yak/ssa"

const TAG ssa.ErrorTag = "typeInference"

type TypeInference struct {
}

func init() {
	RegisterAnalyzer(&TypeInference{})
}

// Analyze(config, *ssa.Program)
func (t *TypeInference) Analyze(config config, prog *ssa.Program) {
	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {

			for _, b := range f.Blocks {
				for _, inst := range b.Instrs {
					t.AnalyzeOnInstruction(inst)
				}
				for _, phi := range b.Phis {
					t.AnalyzeOnInstruction(phi)
				}
			}
		}
	}
}

func (t *TypeInference) AnalyzeOnInstruction(inst ssa.Instruction) bool {
	// inst.TypeInference()
	switch inst := inst.(type) {
	case *ssa.Phi:
		return TypeInferencePhi(inst)
	case *ssa.UnOp:
	case *ssa.BinOp:
		return TypeInferenceBinOp(inst)
	case *ssa.Call:
	case *ssa.Return:
	// case *ssa.Switch:
	// case *ssa.If:
	case *ssa.Interface:
	case *ssa.Field:
	case *ssa.Update:
	}
	return false
}

func TypeInferencePhi(phi *ssa.Phi) bool {
	change := false
	typMap := make(map[ssa.Type]struct{})
	typs := make([]ssa.Type, 0, len(phi.Edge))
	for index, value := range phi.Edge {
		// skip unreachable block
		block := phi.Block.Preds[index]
		if block.Reachable() == -1 {
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

	// only first set type, phi will change
	if len(phi.GetType()) == 0 {
		phi.SetType(typs)
		change = true
	}
	return change
}

func TypeInferenceBinOp(bin *ssa.BinOp) bool {

	return false
}

func TypeInferenceField(f *ssa.Field) bool {
	change := false
	return change
}

func TypeInferenceUpdate(u *ssa.Update) bool {
	return false
}

func TypeInferenceCall(c *ssa.Call) bool {
	return false
}

func TypeInferenceReturn(r *ssa.Return) bool {
	return false
}
