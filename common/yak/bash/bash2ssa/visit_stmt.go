package bash2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"mvdan.cc/sh/v3/syntax"
)

func (ab *astBuilder) buildStmt(stmt *syntax.Stmt) ssa.Value {
	if stmt == nil {
		return nil
	}
	defer ab.setRange(stmt)()

	var out ssa.Value
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		out = ab.buildCallExpr(cmd, stmt.Redirs, nil)
	case *syntax.BinaryCmd:
		out = ab.buildBinaryCmd(cmd)
	case *syntax.Subshell:
		out = ab.buildStmts(cmd.Stmts)
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
		out = ab.buildFuncDecl(cmd)
	case *syntax.ArithmCmd:
		out = ab.buildArithm(cmd.X)
	case *syntax.TestClause:
		ab.buildTest(cmd.X)
	case *syntax.DeclClause:
		out = ab.buildDeclClause(cmd)
	case *syntax.LetClause:
		for _, expr := range cmd.Exprs {
			ab.buildArithm(expr)
		}
	case *syntax.TimeClause:
		out = ab.buildStmt(cmd.Stmt)
	case *syntax.CoprocClause:
		out = ab.buildStmt(cmd.Stmt)
	case *syntax.TestDecl:
		out = ab.buildStmt(cmd.Body)
	}

	// Statement-level redirections on a non-simple command (for example
	// `{ ... } > /tmp/x`) still describe an I/O edge. Attach them to the last
	// call produced by the compound command.
	if _, isCallExpr := stmt.Cmd.(*syntax.CallExpr); !isCallExpr {
		if call, ok := out.(*ssa.Call); ok && call != nil {
			ab.applyRedirs(call, stmt.Redirs)
		}
	}
	return out
}

func (ab *astBuilder) buildBinaryCmd(cmd *syntax.BinaryCmd) ssa.Value {
	if cmd == nil {
		return nil
	}
	switch cmd.Op {
	case syntax.Pipe, syntax.PipeAll:
		left := ab.buildStmt(cmd.X)
		return ab.buildStmtWithStdin(cmd.Y, left)
	case syntax.AndStmt, syntax.OrStmt:
		// Both branches are reachable at runtime. Compile both so a dangerous
		// command in either branch is not lost.
		ab.buildStmt(cmd.X)
		return ab.buildStmt(cmd.Y)
	default:
		ab.buildStmt(cmd.X)
		return ab.buildStmt(cmd.Y)
	}
}

// buildStmtWithStdin feeds the previous pipeline stage into the first command
// of stmt. Compound commands receive the value through pendingStdin.
func (ab *astBuilder) buildStmtWithStdin(stmt *syntax.Stmt, stdin ssa.Value) ssa.Value {
	if stmt == nil {
		return nil
	}
	if callExpr, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		return ab.buildCallExpr(callExpr, stmt.Redirs, stdin)
	}
	if bc, ok := stmt.Cmd.(*syntax.BinaryCmd); ok && (bc.Op == syntax.Pipe || bc.Op == syntax.PipeAll) {
		left := ab.buildStmtWithStdin(bc.X, stdin)
		return ab.buildStmtWithStdin(bc.Y, left)
	}

	previous := ab.pendingStdin
	ab.pendingStdin = stdin
	out := ab.buildStmt(stmt)
	ab.pendingStdin = previous
	return out
}

func (ab *astBuilder) buildIfClause(clause *syntax.IfClause) ssa.Value {
	if clause == nil {
		return nil
	}
	var out ssa.Value
	if v := ab.buildStmts(clause.Cond); !utils.IsNil(v) {
		out = v
	}
	if v := ab.buildStmts(clause.Then); !utils.IsNil(v) {
		out = v
	}
	if clause.Else != nil {
		if v := ab.buildIfClause(clause.Else); !utils.IsNil(v) {
			out = v
		}
	}
	return out
}

func (ab *astBuilder) buildWhileClause(clause *syntax.WhileClause) ssa.Value {
	if clause == nil {
		return nil
	}
	var out ssa.Value
	if v := ab.buildStmts(clause.Cond); !utils.IsNil(v) {
		out = v
	}
	if v := ab.buildStmts(clause.Do); !utils.IsNil(v) {
		out = v
	}
	return out
}

func (ab *astBuilder) buildForClause(clause *syntax.ForClause) ssa.Value {
	if clause == nil {
		return nil
	}
	switch loop := clause.Loop.(type) {
	case *syntax.WordIter:
		// Every item can be bound to the loop variable across iterations.
		var values ssa.Values
		allStatic := true
		for _, item := range loop.Items {
			if v := ab.buildWord(item); !utils.IsNil(v) {
				values = append(values, v)
			}
			if _, ok := staticWord(item); !ok {
				allStatic = false
			}
		}
		if len(values) == 0 {
			values = append(values, ab.readOrCreate("$@"))
			allStatic = false
		}

		// Unroll small literal lists so rules that match concrete argument
		// values see every iteration, not just a merged phi. The depth and
		// budget caps keep nested loops from multiplying body compilation.
		if allStatic && len(values) > 1 && len(values) <= maxLiteralLoopUnroll &&
			ab.loopUnrollDepth < maxLoopUnrollDepth && ab.loopUnrollBudget >= len(values) {
			ab.loopUnrollDepth++
			ab.loopUnrollBudget -= len(values)
			var out ssa.Value
			for _, v := range values {
				ab.assignLoopVar(loop, v)
				if body := ab.buildStmts(clause.Do); !utils.IsNil(body) {
					out = body
				}
			}
			ab.loopUnrollDepth--
			// Reads after the loop may observe any iteration's value.
			ab.assignLoopVar(loop, ab.mergeLoopValues(loop, values))
			return out
		}

		ab.assignLoopVar(loop, ab.mergeLoopValues(loop, values))
	case *syntax.CStyleLoop:
		ab.buildArithm(loop.Init)
		ab.buildArithm(loop.Cond)
		ab.buildArithm(loop.Post)
	}
	return ab.buildStmts(clause.Do)
}

func (ab *astBuilder) assignLoopVar(loop *syntax.WordIter, value ssa.Value) {
	if loop == nil || loop.Name == nil || loop.Name.Value == "" || utils.IsNil(value) {
		return
	}
	if variable := ab.CreateVariable(loop.Name.Value); variable != nil {
		ab.AssignVariable(variable, value)
	}
}

func (ab *astBuilder) mergeLoopValues(loop *syntax.WordIter, values ssa.Values) ssa.Value {
	switch len(values) {
	case 0:
		return ab.readOrCreate("$@")
	case 1:
		return values[0]
	}
	name := "$for"
	if loop != nil && loop.Name != nil && loop.Name.Value != "" {
		name = loop.Name.Value
	}
	merged := ab.EmitPhi(name+"$loop", values)
	if utils.IsNil(merged) {
		return values[len(values)-1]
	}
	return merged
}

func (ab *astBuilder) buildCaseClause(clause *syntax.CaseClause) ssa.Value {
	if clause == nil {
		return nil
	}
	if clause.Word != nil {
		ab.buildWord(clause.Word)
	}
	var out ssa.Value
	for _, item := range clause.Items {
		if item == nil {
			continue
		}
		for _, pattern := range item.Patterns {
			ab.buildWord(pattern)
		}
		if v := ab.buildStmts(item.Stmts); !utils.IsNil(v) {
			out = v
		}
	}
	return out
}

func (ab *astBuilder) buildFuncDecl(decl *syntax.FuncDecl) ssa.Value {
	if decl == nil || decl.Name == nil || decl.Name.Value == "" {
		return nil
	}
	name := decl.Name.Value

	fn := ab.funcs[name]
	if fn == nil {
		fn = ab.NewFunc(name)
		ab.funcs[name] = fn
	}
	if fn == nil {
		return nil
	}

	parent := ab.FunctionBuilder
	sub := parent.PushFunction(fn)
	ab.FunctionBuilder = sub

	for i := 1; i <= 9; i++ {
		paramName := fmt.Sprintf("$%d", i)
		param := sub.NewParam(paramName)
		if variable := sub.CreateVariable(paramName); variable != nil {
			sub.AssignVariable(variable, param)
		}
	}
	for _, paramName := range []string{"$@", "$*"} {
		param := sub.NewParam(paramName)
		if variable := sub.CreateVariable(paramName); variable != nil {
			sub.AssignVariable(variable, param)
		}
	}

	ab.buildStmt(decl.Body)
	sub.Finish()
	ab.FunctionBuilder = parent

	if variable := parent.CreateVariable(name); variable != nil {
		parent.AssignVariable(variable, fn)
	}
	parent.GetProgram().SetExportValue(name, fn)
	return fn
}

func (ab *astBuilder) buildDeclClause(clause *syntax.DeclClause) ssa.Value {
	if clause == nil {
		return nil
	}
	var out ssa.Value
	for _, arg := range clause.Args {
		if arg == nil {
			continue
		}
		if arg.Naked || arg.Name == nil {
			if arg.Value != nil {
				if v := ab.buildWord(arg.Value); !utils.IsNil(v) {
					out = v
				}
			}
			continue
		}
		if v := ab.buildAssign(arg); !utils.IsNil(v) {
			out = v
		}
	}
	return out
}

func (ab *astBuilder) buildTest(expr syntax.TestExpr) {
	switch e := expr.(type) {
	case *syntax.Word:
		ab.buildWord(e)
	case *syntax.BinaryTest:
		ab.buildTest(e.X)
		ab.buildTest(e.Y)
	case *syntax.UnaryTest:
		ab.buildTest(e.X)
	case *syntax.ParenTest:
		ab.buildTest(e.X)
	}
}
