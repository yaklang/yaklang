package bash2ssa

import (
	"maps"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// The ordinary Call value models an external command's stdout. Its exit status
// is a separate value. A shell function returns both, in that order.
const statusName = "$?"

func (ab *astBuilder) put(name string, value ssa.Value) {
	v := ab.CreateVariableForce(name)
	// Keep updates to `local x` (and our shell-local status/stream slots)
	// in the same binding, including writes in nested CFG scopes.
	if prior := ab.CurrentBlock.ScopeTable.ReadVariable(name); prior != nil {
		v.SetCaptured(prior)
	}
	closure := ab.SupportClosure
	if ab.childDepth > 0 {
		ab.SupportClosure = false
	}
	ab.AssignVariable(v, value)
	ab.SupportClosure = closure
}

func (ab *astBuilder) slot(name string, initial ssa.Value) string {
	ab.serial++
	name = "@bash." + name + "." + strconv.Itoa(ab.serial)
	ab.AssignVariable(ab.CreateLocalVariable(name), initial)
	return name
}

// An intrinsic represents a shell operation with runtime-dependent results. It
// records operands for dataflow; the frontend never executes a command.
func (ab *astBuilder) intrinsic(name string, args ...ssa.Value) ssa.Value {
	return ab.EmitCall(ab.NewCall(ab.calleeValue("@bash."+name), args))
}

func (ab *astBuilder) number(v ssa.Value) ssa.Value {
	if v.GetType().GetTypeKind() == ssa.NumberTypeKind {
		return v
	}
	return ab.EmitTypeCast(v, ssa.CreateNumberType())
}

func (ab *astBuilder) text(v ssa.Value) ssa.Value {
	if v.GetType().GetTypeKind() == ssa.StringTypeKind {
		return v
	}
	return ab.EmitTypeCast(v, ssa.CreateStringType())
}

func (ab *astBuilder) success() ssa.Value {
	return ab.EmitBinOp(ssa.OpEq, ab.ReadValue(statusName), ab.EmitConstInst(0))
}

func (ab *astBuilder) boolStatus(v ssa.Value) ssa.Value {
	if c, ok := ssa.ToConstInst(v); ok && c.IsBoolean() {
		if c.Boolean() {
			return ab.EmitConstInst(0)
		}
		return ab.EmitConstInst(1)
	}
	return ab.number(ab.EmitUnOp(ssa.OpNot, v))
}

func (ab *astBuilder) concat(a, b ssa.Value) ssa.Value {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if c, ok := ssa.ToConstInst(a); ok && c.IsString() && c.String() == "" {
		return b
	}
	if c, ok := ssa.ToConstInst(b); ok && c.IsString() && c.String() == "" {
		return a
	}
	return ab.EmitBinOp(ssa.OpAdd, ab.text(a), ab.text(b))
}

func (ab *astBuilder) join(values []ssa.Value, separator string) ssa.Value {
	var out ssa.Value
	for i, v := range values {
		if i != 0 {
			out = ab.concat(out, ab.EmitConstInst(separator))
		}
		out = ab.concat(out, v)
	}
	if out == nil {
		return ab.EmitConstInst("")
	}
	return out
}

// Variables inside a child shell are copies. A brace group, unlike a
// parenthesized group, must not use this scope. Keep the result in the same
// function: a substitution can feed an enclosing command directly.
func (ab *astBuilder) childShell(build func() ssa.Value) (ssa.Value, ssa.Value) {
	parent := ab.CurrentBlock.ScopeTable
	enter := ab.NewBasicBlock("bash.subshell")
	ab.EmitJump(enter)
	ab.CurrentBlock = enter
	enter.SetScope(parent.CreateSubScope())
	funcs, depth, stdin, stream, assoc := ab.funcs, ab.loopDepth, ab.pendingStdin, ab.stdoutSlot, ab.assoc
	ab.stdoutSlot = ""
	ab.assoc = maps.Clone(assoc)
	ab.funcs = make(map[string]*ssa.Function, len(funcs))
	for k, v := range funcs {
		ab.funcs[k] = v
	}
	ab.loopDepth = 0
	ab.childDepth++
	out := build()
	status := ab.ReadValue(statusName)
	ab.childDepth--
	ab.loopDepth, ab.funcs, ab.pendingStdin, ab.stdoutSlot, ab.assoc = depth, funcs, stdin, stream, assoc
	end := ab.NewBasicBlock("bash.subshell.done")
	ab.EmitJump(end)
	ab.CurrentBlock = end
	end.SetScope(parent.CreateShadowScope())
	return out, status
}

func isPlainName(s string) bool { return !strings.ContainsAny(s, `/\\`) }
