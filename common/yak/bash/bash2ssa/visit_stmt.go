package bash2ssa

import (
	"fmt"
	"strings"

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
		return ab.buildShortCircuit(cmd)
	default:
		ab.buildStmt(cmd.X)
		return ab.buildStmt(cmd.Y)
	}
}

// buildShortCircuit lowers `left && right` / `left || right` to a conditional
// block. Both sides are still compiled so a dangerous command in either side
// stays visible, but assignment on the right merges with the left path through
// the if-done Phi instead of overwriting the entering value.
func (ab *astBuilder) buildShortCircuit(cmd *syntax.BinaryCmd) ssa.Value {
	if ab.IsBlockFinish() {
		return nil
	}
	ab.declareMergeTargets(cmd)

	branches := ab.CreateIfBuilder()
	var left, right ssa.Value
	condition := func() ssa.Value {
		left = ab.buildStmt(cmd.X)
		if cmd.Op == syntax.OrStmt && !utils.IsNil(left) {
			// `||` runs the right side when the left side is false.
			if flipped := ab.EmitUnOp(ssa.OpNeg, left); !utils.IsNil(flipped) {
				return flipped
			}
		}
		return left
	}
	body := func() {
		right = ab.buildStmt(cmd.Y)
	}
	branches.SetCondition(condition, body)
	branches.Build()

	return ab.mergeBranchValues("binary$value", ssa.Values{left, right})
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

// buildIfClause lowers if/elif/else into a real conditional CFG. The else
// branch is represented by an IfClause without a condition, which is how
// mvdan/sh models a bare `else`.
func (ab *astBuilder) buildIfClause(clause *syntax.IfClause) ssa.Value {
	if clause == nil || ab.IsBlockFinish() {
		return nil
	}
	ab.declareMergeTargets(clause)

	branches := ab.CreateIfBuilder()
	var thenValues, elseValues ssa.Values

	var elseStmts []*syntax.Stmt
	hasElse := false
	for current := clause; current != nil; current = current.Else {
		if current != clause && len(current.Cond) == 0 {
			elseStmts = current.Then
			hasElse = true
			break
		}
		item := current
		branches.AppendItem(
			func() ssa.Value { return ab.buildStmts(item.Cond) },
			func() {
				if v := ab.buildStmts(item.Then); !utils.IsNil(v) {
					thenValues = append(thenValues, v)
				}
			},
		)
	}
	if hasElse {
		branches.SetElse(func() {
			if v := ab.buildStmts(elseStmts); !utils.IsNil(v) {
				elseValues = append(elseValues, v)
			}
		})
	}
	branches.Build()

	return ab.mergeBranchValues("if$value", append(thenValues, elseValues...))
}

func (ab *astBuilder) buildWhileClause(clause *syntax.WhileClause) ssa.Value {
	if clause == nil || ab.IsBlockFinish() {
		return nil
	}
	ab.declareMergeTargets(clause)

	loop := ab.CreateLoopBuilder()
	until := clause.Until
	var out ssa.Value
	loop.SetCondition(func() ssa.Value {
		cond := ab.buildStmts(clause.Cond)
		if !until {
			return cond
		}
		if utils.IsNil(cond) {
			cond = ab.EmitConstInst(false)
		}
		if flipped := ab.EmitUnOp(ssa.OpNeg, cond); !utils.IsNil(flipped) {
			return flipped
		}
		return cond
	})
	loop.SetBody(func() {
		if v := ab.buildStmts(clause.Do); !utils.IsNil(v) {
			out = v
		}
	})
	loop.Finish()
	return out
}

func (ab *astBuilder) buildForClause(clause *syntax.ForClause) ssa.Value {
	if clause == nil {
		return nil
	}
	switch iter := clause.Loop.(type) {
	case *syntax.WordIter:
		// Every item can be bound to the loop variable across iterations.
		var values ssa.Values
		allStatic := true
		for _, item := range iter.Items {
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
		// values see every iteration, not just a merged phi. A body that
		// assigns variables keeps the real loop so loop-carried values get a
		// Phi at the loop header. The depth and budget caps keep nested loops
		// from multiplying body compilation.
		if allStatic && len(values) > 1 && len(values) <= maxLiteralLoopUnroll &&
			ab.loopUnrollDepth < maxLoopUnrollDepth && ab.loopUnrollBudget >= len(values) &&
			len(collectAssignedNamesInStmts(clause.Do)) == 0 && !ab.IsBlockFinish() {
			ab.loopUnrollDepth++
			ab.loopUnrollBudget -= len(values)
			var out ssa.Value
			for _, v := range values {
				ab.assignLoopVar(iter, v)
				if body := ab.buildStmts(clause.Do); !utils.IsNil(body) {
					out = body
				}
			}
			ab.loopUnrollDepth--
			// Reads after the loop may observe any iteration's value.
			ab.assignLoopVar(iter, ab.mergeLoopValues(iter, values))
			return out
		}

		ab.declareMergeTargets(clause)
		loop := ab.CreateLoopBuilder()
		loop.SetFirst(func() []ssa.Value {
			// The merged item Phi lives in the loop header, where the variable
			// is bound before the first iteration.
			ab.assignLoopVar(iter, ab.mergeLoopValues(iter, values))
			return nil
		})
		var out ssa.Value
		loop.SetBody(func() {
			if v := ab.buildStmts(clause.Do); !utils.IsNil(v) {
				out = v
			}
		})
		loop.Finish()
		return out
	case *syntax.CStyleLoop:
		ab.declareMergeTargets(clause)
		loop := ab.CreateLoopBuilder()
		loop.SetFirst(func() []ssa.Value {
			ab.buildArithm(iter.Init)
			return nil
		})
		loop.SetCondition(func() ssa.Value {
			return ab.buildArithm(iter.Cond)
		})
		loop.SetThird(func() []ssa.Value {
			ab.buildArithm(iter.Post)
			return nil
		})
		var out ssa.Value
		loop.SetBody(func() {
			if v := ab.buildStmts(clause.Do); !utils.IsNil(v) {
				out = v
			}
		})
		loop.Finish()
		return out
	}
	return ab.buildStmts(clause.Do)
}

// mergeBranchValues joins the values produced by sibling branches. The Phi is
// emitted in whatever block the branch builder left current (the join block).
func (ab *astBuilder) mergeBranchValues(name string, values ssa.Values) ssa.Value {
	filtered := make(ssa.Values, 0, len(values))
	for _, value := range values {
		if !utils.IsNil(value) {
			filtered = append(filtered, value)
		}
	}
	values = filtered
	switch len(values) {
	case 0:
		return nil
	case 1:
		return values[0]
	}
	if ab.IsBlockFinish() {
		return values[len(values)-1]
	}
	if merged := ab.EmitPhi(name, values); !utils.IsNil(merged) {
		return merged
	}
	return values[len(values)-1]
}

// declareMergeTargets pre-creates the variables assigned inside a compound
// statement in the current block. Bash variables do not need a declaration
// before use, so without this the scope merge cannot find the name and skips
// the Phi, losing values first assigned inside a branch or loop body.
func (ab *astBuilder) declareMergeTargets(node syntax.Node) {
	for _, name := range collectAssignedNames(node) {
		ab.readOrCreate(name)
	}
}

// assignedNameCollector gathers assigned variable names without duplicates.
type assignedNameCollector struct {
	seen  map[string]struct{}
	names []string
}

func newAssignedNameCollector() *assignedNameCollector {
	return &assignedNameCollector{seen: make(map[string]struct{})}
}

func (c *assignedNameCollector) add(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if _, ok := c.seen[name]; ok {
		return
	}
	c.seen[name] = struct{}{}
	c.names = append(c.names, name)
}

func (c *assignedNameCollector) walk(node syntax.Node) {
	if node == nil {
		return
	}
	syntax.Walk(node, func(current syntax.Node) bool {
		switch n := current.(type) {
		case *syntax.Assign:
			if n.Name != nil {
				c.add(n.Name.Value)
			}
		case *syntax.DeclClause:
			for _, arg := range n.Args {
				if arg != nil && arg.Name != nil && !arg.Naked {
					c.add(arg.Name.Value)
				}
			}
		case *syntax.LetClause:
			for _, expr := range n.Exprs {
				if name, ok := arithmeticVariableName(expr); ok {
					c.add(name)
				}
			}
		case *syntax.BinaryArithm:
			if _, ok := arithmAssignOp(n.Op); ok {
				if name, ok := arithmeticVariableName(n.X); ok {
					c.add(name)
				}
			}
		case *syntax.UnaryArithm:
			if n.Op == syntax.Inc || n.Op == syntax.Dec {
				if name, ok := arithmeticVariableName(n.X); ok {
					c.add(name)
				}
			}
		case *syntax.CallExpr:
			for _, name := range readTargetNames(n.Args) {
				c.add(name)
			}
		}
		return true
	})
}

// collectAssignedNames returns the variable names a syntax node assigns:
// plain assignments, declare/local/export arguments, let/arithmetic targets,
// and the target arguments of `read`. Names are returned once, in first-seen
// order.
func collectAssignedNames(node syntax.Node) []string {
	collector := newAssignedNameCollector()
	collector.walk(node)
	return collector.names
}

func collectAssignedNamesInStmts(stmts []*syntax.Stmt) []string {
	collector := newAssignedNameCollector()
	for _, stmt := range stmts {
		collector.walk(stmt)
	}
	return collector.names
}

// readTargetNames lists the variables `read [-opts] var...` assigns.
func readTargetNames(args []*syntax.Word) []string {
	if len(args) == 0 {
		return nil
	}
	if name, ok := staticWord(args[0]); !ok || commandBase(name) != "read" {
		return nil
	}
	var names []string
	for i := 1; i < len(args); i++ {
		token, ok := staticWord(args[i])
		if !ok {
			break
		}
		if token == "--" {
			for _, word := range args[i+1:] {
				if name, ok := staticWord(word); ok {
					names = append(names, name)
				}
			}
			break
		}
		if strings.HasPrefix(token, "-") && token != "-" {
			if readOptionTakesValue(token) {
				i++
			}
			continue
		}
		names = append(names, token)
	}
	return names
}

func readOptionTakesValue(token string) bool {
	switch token {
	case "-a", "-d", "-i", "-n", "-N", "-p", "-t", "-u":
		return true
	}
	return false
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
