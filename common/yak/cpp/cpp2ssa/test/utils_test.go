package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage(ssaconfig.CPP, cpp2ssa.CreateBuilder)
}

// compileCppProject lowers an in-memory multi-file C++ project. Headers enter
// the SSA only through #include, which matches the production frontend.
func compileCppProject(t *testing.T, files map[string]string) []*ssaapi.Program {
	t.Helper()
	vf := filesys.NewVirtualFs()
	for name, code := range files {
		vf.AddFile(name, code)
	}
	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.CPP),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	return progs
}

// funcBySymbol returns a lowered function by its exported name.
func funcBySymbol(t *testing.T, prog *ssaapi.Program, name string) *ssa.Function {
	t.Helper()
	fn := prog.Program.GetFunction(name, "")
	require.NotNilf(t, fn, "function %q was not lowered", name)
	return fn
}

// blockValue resolves a basic-block id owned by fn.
func blockValue(t *testing.T, fn *ssa.Function, id int64) *ssa.BasicBlock {
	t.Helper()
	value, ok := fn.GetValueById(id)
	require.Truef(t, ok, "missing value %d", id)
	block, ok := value.(*ssa.BasicBlock)
	require.Truef(t, ok, "value %d is %T, not a basic block", id, value)
	return block
}

// findPhi returns the first phi that carries the named variable in fn.
func findPhi(t *testing.T, fn *ssa.Function, name string) *ssa.Phi {
	t.Helper()
	for _, blockID := range fn.Blocks {
		block := blockValue(t, fn, blockID)
		for _, phiID := range block.Phis {
			value, ok := fn.GetValueById(phiID)
			require.True(t, ok)
			phi, ok := ssa.ToPhi(value)
			if ok && phi.GetName() == name {
				return phi
			}
		}
	}
	require.Failf(t, "phi not found", "no phi for %q in %s", name, fn.GetName())
	return nil
}

// phiEdgeFrom returns the incoming value whose predecessor block name starts
// with prefix. The phi edge list is parallel to the predecessor list.
func phiEdgeFrom(t *testing.T, fn *ssa.Function, phi *ssa.Phi, prefix string) ssa.Value {
	t.Helper()
	block := phi.GetBlock()
	require.NotNil(t, block)
	require.Len(t, phi.Edge, len(block.Preds), "phi edges must match predecessor blocks")
	for index, predID := range block.Preds {
		pred := blockValue(t, fn, predID)
		if !strings.HasPrefix(pred.GetName(), prefix) {
			continue
		}
		value, ok := fn.GetValueById(phi.Edge[index])
		require.True(t, ok)
		return value
	}
	require.Failf(t, "predecessor not found", "no %q predecessor for phi %s (preds %v)",
		prefix, phi.GetName(), block.Preds)
	return nil
}

// constInt reads the integer payload of a constant value.
func constInt(t *testing.T, value ssa.Value) int64 {
	t.Helper()
	constInst, ok := ssa.ToConstInst(value)
	require.Truef(t, ok, "value %v is not a constant", value)
	switch raw := constInst.GetRawValue().(type) {
	case int64:
		return raw
	case int:
		return int64(raw)
	default:
		require.Failf(t, "not an integer constant", "%T = %v", raw, raw)
		return 0
	}
}

// callsTo returns the calls in caller whose target is callee.
func callsTo(t *testing.T, caller, callee *ssa.Function) []*ssa.Call {
	t.Helper()
	var calls []*ssa.Call
	for _, blockID := range caller.Blocks {
		block := blockValue(t, caller, blockID)
		for _, instID := range block.Insts {
			inst, ok := caller.GetInstructionById(instID)
			if !ok {
				continue
			}
			call, ok := ssa.ToCall(inst)
			if !ok {
				continue
			}
			target, ok := caller.GetValueById(call.Method)
			if !ok {
				continue
			}
			if target.GetId() == callee.GetId() {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

// callArgStrings renders the argument values of a call.
func callArgStrings(t *testing.T, fn *ssa.Function, call *ssa.Call) []string {
	t.Helper()
	args := make([]string, 0, len(call.Args))
	for _, id := range call.Args {
		value, ok := fn.GetValueById(id)
		require.True(t, ok)
		args = append(args, ssa.LineDisASM(value))
	}
	return args
}
// callsIn returns every call instruction in fn.
func callsIn(t *testing.T, fn *ssa.Function) []*ssa.Call {
	t.Helper()
	var calls []*ssa.Call
	for _, blockID := range fn.Blocks {
		block := blockValue(t, fn, blockID)
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok {
				continue
			}
			if call, ok := ssa.ToCall(inst); ok {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

// hasCallToName reports whether one call in fn names target.
func hasCallToName(t *testing.T, fn *ssa.Function, target string) bool {
	t.Helper()
	for _, call := range callsIn(t, fn) {
		callee, ok := fn.GetValueById(call.Method)
		if !ok || utils.IsNil(callee) {
			continue
		}
		if callee.GetName() == target {
			return true
		}
		if function, ok := ssa.ToFunction(callee); ok && function.GetName() == target {
			return true
		}
	}
	return false
}

// typeCastsIn returns the dedicated typecast instructions emitted by fn.
func typeCastsIn(t *testing.T, fn *ssa.Function) []*ssa.TypeCast {
	t.Helper()
	var casts []*ssa.TypeCast
	for _, blockID := range fn.Blocks {
		block := blockValue(t, fn, blockID)
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok {
				continue
			}
			if cast, ok := ssa.ToTypeCast(inst); ok && cast != nil {
				casts = append(casts, cast)
			}
		}
	}
	return casts
}

// freeValueNames lists the free-value parameters of a closure function.
func freeValueNames(t *testing.T, fn *ssa.Function) []string {
	t.Helper()
	var names []string
	for variable, id := range fn.FreeValues {
		value, ok := fn.GetValueById(id)
		if !ok {
			continue
		}
		param, ok := ssa.ToParameter(value)
		if ok && param != nil && param.IsFreeValue {
			name := param.GetName()
			if variable != nil && variable.GetName() != "" {
				name = variable.GetName()
			}
			names = append(names, name)
		}
	}
	return names
}

// findFunctionByNameInPrograms returns the lowered function whose table key or
// emitted name matches name.
func findFunctionByNameInPrograms(t *testing.T, progs []*ssaapi.Program, name string) *ssa.Function {
	t.Helper()
	for _, prog := range progs {
		var found *ssa.Function
		prog.Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
			if key == name || fn.GetName() == name {
				found = fn
				return false
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}
