package bash2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"maps"
	"mvdan.cc/sh/v3/syntax"
	"strings"
)

func (ab *astBuilder) buildFuncDecl(d *syntax.FuncDecl) {
	name := d.Name.Value
	locals := map[string]bool{}
	syntax.Walk(d.Body, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.FuncDecl:
			return false
		case *syntax.DeclClause:
			if localDecl(n) {
				for _, a := range n.Args {
					if a.Name != nil {
						locals[a.Name.Value] = true
					}
				}
			}
		}
		return true
	})
	// Materialize the outer slots before PushFunction. The SSA closure builder
	// can then bind reads at the actual call site and publish non-local writes as
	// SideEffects. Do not import local declarations into the parent.
	// TODO(ssa-bash): non-local writes are published through SSA closure bindings; bash's dynamic scope is not reproduced at the call site.
	for _, s := range assignedNames(d.Body) {
		if !locals[s] {
			ab.readOrCreate(s)
		}
	}
	syntax.Walk(d.Body, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.FuncDecl:
			return false
		case *syntax.ParamExp:
			if syntax.ValidName(n.Param.Value) && !locals[n.Param.Value] {
				ab.readOrCreate(n.Param.Value)
			}
		}
		return true
	})
	fn := ab.declared[d]
	if fn == nil {
		fn = ab.NewFunc(name)
	}
	ab.funcs[name] = fn
	ab.GetProgram().SetInstructionWithName(name, fn)
	ab.GetProgram().SetExportValue(name, fn)
	sub := *ab
	sub.FunctionBuilder = ab.PushFunction(fn)
	sub.inFunction = true
	sub.childDepth = 0
	sub.pendingStdin = nil
	sub.loopDepth = 0
	sub.assoc = maps.Clone(ab.assoc)
	sub.funcs = maps.Clone(ab.forward)
	maps.Copy(sub.funcs, ab.funcs)
	argv := sub.NewParam("$@")
	sub.AssignVariable(sub.CreateLocalVariable("$@"), argv)
	sub.HandlerEllipsis()
	fn.ParamLength = len(fn.Params)
	sub.AssignVariable(sub.CreateLocalVariable(statusName), sub.EmitConstInst(0))
	sub.stdoutSlot = sub.slot("function.stdout", sub.EmitConstInst(""))
	sub.buildStmt(d.Body)
	if !sub.IsBlockFinish() {
		sub.shellReturn(sub.ReadValue(statusName))
	}
	sub.Finish()
}

func (ab *astBuilder) shellReturn(status ssa.Value) {
	ab.EmitReturn([]ssa.Value{ab.ReadValue(ab.stdoutSlot), status})
}

// knownReturn returns the shared value of one return slot when every exit of
// the function returns the same constant. Static checks can then see, for
// example, that `f(){ return 7; }; f; x=$?` sets x to 7.
func (ab *astBuilder) knownReturn(fn *ssa.Function, index int) ssa.Value {
	if fn == nil || len(fn.Return) == 0 {
		return nil
	}
	var only ssa.Value
	for _, id := range fn.Return {
		inst, ok := fn.GetInstructionById(id)
		if !ok {
			return nil
		}
		r, ok := ssa.ToReturn(inst)
		if !ok || len(r.Results) <= index {
			return nil
		}
		v, ok := fn.GetValueById(r.Results[index])
		if !ok {
			return nil
		}
		c, ok := ssa.ToConstInst(v)
		if !ok {
			return nil
		}
		if only == nil {
			only = c
			continue
		}
		if only.GetType().GetTypeKind() != c.GetType().GetTypeKind() || only.String() != c.String() {
			return nil
		}
	}
	return only
}

func localDecl(d *syntax.DeclClause) bool {
	switch d.Variant.Value {
	case "local":
		return true
	case "declare", "typeset":
		for _, a := range d.Args {
			if a.Value != nil && strings.HasPrefix(a.Value.Lit(), "-") && strings.Contains(a.Value.Lit(), "g") {
				return false
			}
		}
		return true
	}
	return false
}
func (ab *astBuilder) buildDeclClause(d *syntax.DeclClause) {
	assoc := false
	local := ab.inFunction && localDecl(d)
	for _, a := range d.Args {
		if a.Name == nil {
			if a.Value != nil && strings.HasPrefix(a.Value.Lit(), "-") && strings.Contains(a.Value.Lit(), "A") {
				assoc = true
			}
			continue
		}
		name := a.Name.Value
		if assoc {
			ab.assoc[name] = true
		}
		if !a.Naked || local {
			ab.assign(a, local)
		}
	}
}
