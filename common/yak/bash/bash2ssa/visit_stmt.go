package bash2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"mvdan.cc/sh/v3/syntax"
)

func (ab *astBuilder) buildStmt(stmt *syntax.Stmt) ssa.Value {
	if stmt == nil || ab.IsBlockFinish() {
		return nil
	}
	defer ab.setRange(stmt)()
	if _, simple := stmt.Cmd.(*syntax.CallExpr); !simple && len(stmt.Redirs) > 0 {
		previous := ab.redirects
		ab.redirects = make(map[string]int64, len(previous))
		for k, id := range previous {
			ab.redirects[k] = id
		}
		ab.bindRedirs(ab.redirects, stmt.Redirs)
		defer func() { ab.redirects = previous }()
	}
	var out ssa.Value
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		out = ab.buildCallExpr(cmd, stmt.Redirs, nil)
	case *syntax.BinaryCmd:
		out = ab.buildBinaryCmd(cmd)
	case *syntax.Subshell:
		var status ssa.Value
		out, status = ab.childShell(func() ssa.Value { return ab.buildStmts(cmd.Stmts) })
		ab.put(statusName, status)
	case *syntax.Block:
		out = ab.buildStmts(cmd.Stmts)
	case *syntax.IfClause:
		out = ab.buildIfClause(cmd)
	case *syntax.WhileClause:
		out = ab.buildWhileClause(cmd)
	case *syntax.ForClause:
		out = ab.buildForClause(cmd)
	case *syntax.CaseClause:
		out = ab.buildCaseClause(cmd)
	case *syntax.FuncDecl:
		ab.buildFuncDecl(cmd)
		ab.put(statusName, ab.EmitConstInst(0))
	case *syntax.ArithmCmd:
		ab.arithmStatus(cmd.X)
	case *syntax.TestClause:
		ab.put(statusName, ab.boolStatus(ab.buildTest(cmd.X)))
	case *syntax.DeclClause:
		ab.buildDeclClause(cmd)
		ab.put(statusName, ab.EmitConstInst(0))
	case *syntax.LetClause:
		for _, e := range cmd.Exprs {
			ab.arithmStatus(e)
		}
	case *syntax.TimeClause:
		out = ab.buildStmt(cmd.Stmt)
	case *syntax.CoprocClause:
		out, _ = ab.childShell(func() ssa.Value { return ab.buildStmt(cmd.Stmt) })
		ab.put(statusName, ab.EmitConstInst(0))
	case *syntax.TestDecl:
		out = ab.buildStmt(cmd.Body)
	}
	if stmt.Negated && !ab.IsBlockFinish() {
		ab.put(statusName, ab.boolStatus(ab.EmitUnOp(ssa.OpNot, ab.success())))
	}
	return out
}

func (ab *astBuilder) arithmStatus(e syntax.ArithmExpr) {
	if v := ab.buildArithm(e); v != nil {
		ab.put(statusName, ab.boolStatus(ab.EmitBinOp(ssa.OpNotEq, v, ab.EmitConstInst(0))))
	}
}

func (ab *astBuilder) buildBinaryCmd(c *syntax.BinaryCmd) ssa.Value {
	if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
		left, _ := ab.childShell(func() ssa.Value { return ab.buildStmt(c.X) })
		right, status := ab.childShell(func() ssa.Value { ab.pendingStdin = left; return ab.buildStmt(c.Y) })
		ab.put(statusName, status)
		return right
	}
	ab.declareMergeTargets(c)
	left := ab.buildStmt(c.X)
	if ab.IsBlockFinish() {
		return left
	}
	result := ab.slot("rhs", ab.EmitConstInst(""))
	b := ab.CreateIfBuilder()
	b.SetCondition(func() ssa.Value {
		v := ab.success()
		if c.Op == syntax.OrStmt {
			return ab.EmitUnOp(ssa.OpNot, v)
		}
		return v
	}, func() {
		if right := ab.buildStmt(c.Y); right != nil && !ab.IsBlockFinish() {
			ab.put(result, right)
		}
	})
	b.Build()
	ab.foldTrivialPhis()
	return ab.concat(left, ab.ReadValue(result))
}

func (ab *astBuilder) buildIfClause(c *syntax.IfClause) ssa.Value {
	ab.declareMergeTargets(c)
	result := ab.slot("if.stdout", ab.EmitConstInst(""))
	b := ab.CreateIfBuilder()
	body := func(stmts []*syntax.Stmt) {
		if out := ab.buildStmts(stmts); out != nil && !ab.IsBlockFinish() {
			ab.put(result, out)
		}
	}
	for arm := c; arm != nil; arm = arm.Else {
		arm := arm
		if len(arm.Cond) == 0 {
			b.SetElse(func() { body(arm.Then) })
			break
		}
		b.AppendItem(func() ssa.Value { ab.buildStmts(arm.Cond); return ab.success() }, func() { body(arm.Then) })
		if arm.Else == nil {
			b.SetElse(func() { ab.put(statusName, ab.EmitConstInst(0)) })
		}
	}
	b.Build()
	ab.foldTrivialPhis()
	if ab.IsBlockFinish() {
		return nil
	}
	return ab.ReadValue(result)
}

// Shell assignments need no declaration. Seed names in the entering scope so
// the shared CFG builder can merge first assignments, including loop targets.
func (ab *astBuilder) foldTrivialPhis() {
	block := ab.CurrentBlock
	if block == nil {
		return
	}
	for _, id := range block.Phis {
		value, ok := ab.GetValueById(id)
		if !ok {
			continue
		}
		phi, ok := ssa.ToPhi(value)
		if !ok || len(phi.Edge) < 2 || phi.GetName() == "" {
			continue
		}
		var same ssa.Value
		changed := false
		for _, edgeID := range phi.Edge {
			edge, ok := ab.GetValueById(edgeID)
			if !ok {
				same = nil
				break
			}
			c, ok := ssa.ToConstInst(edge)
			if !ok {
				same = nil
				break
			}
			if same == nil {
				same = c
				continue
			}
			if same.GetType().GetTypeKind() != c.GetType().GetTypeKind() || same.String() != c.String() {
				same = nil
				break
			}
			changed = true
		}
		if same != nil && changed {
			ab.put(phi.GetName(), same)
		}
	}
}

func (ab *astBuilder) declareMergeTargets(n syntax.Node) {
	for _, name := range assignedNames(n) {
		ab.readOrCreate(name)
	}
}
func assignedNames(n syntax.Node) []string {
	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			names = append(names, s)
		}
	}
	syntax.Walk(n, func(n syntax.Node) bool {
		switch a := n.(type) {
		case *syntax.FuncDecl, *syntax.Subshell, *syntax.CmdSubst, *syntax.ProcSubst, *syntax.CoprocClause:
			return false
		case *syntax.BinaryCmd:
			if a.Op == syntax.Pipe || a.Op == syntax.PipeAll {
				return false
			}
		case *syntax.CallExpr:
			if len(a.Args) > 0 {
				for _, s := range readTargetNames(a.Args) {
					add(s)
				}
				return false
			}
		case *syntax.WordIter:
			add(a.Name.Value)
		case *syntax.Assign:
			if a.Name != nil {
				add(a.Name.Value)
			}
		case *syntax.BinaryArithm:
			if _, ok := arithmAssignOp(a.Op); ok {
				if s, ok := arithmeticVariableName(a.X); ok {
					add(s)
				}
			}
		case *syntax.UnaryArithm:
			if a.Op == syntax.Inc || a.Op == syntax.Dec {
				if s, ok := arithmeticVariableName(a.X); ok {
					add(s)
				}
			}
		case *syntax.ParamExp:
			if a.Exp != nil && (a.Exp.Op == syntax.AssignUnset || a.Exp.Op == syntax.AssignUnsetOrNull) {
				add(a.Param.Value)
			}
		}
		return true
	})
	return names
}
