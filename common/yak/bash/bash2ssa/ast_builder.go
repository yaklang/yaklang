package bash2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"mvdan.cc/sh/v3/syntax"
)

const (
	// dynamicCommandName is the callee used when the command word is not a
	// compile-time constant, e.g. "$CMD -rf /".
	dynamicCommandName = "Command"

	// maxInlineScriptDepth bounds recursive parsing of eval / sh -c payloads.
	maxInlineScriptDepth = 3
	maxWrapperDepth      = 3

	// maxLiteralLoopUnroll bounds per-loop body duplication when a for list is
	// made of literal words; maxLoopUnrollDepth bounds nesting.
	maxLiteralLoopUnroll = 16
	maxLoopUnrollDepth   = 2
)

// astBuilder lowers a mvdan/sh syntax tree into SSA.
//
// Call layout used by this frontend:
//   - Call.Method is a callee named after the resolved command (curl, rm, ...)
//   - Call.Args[0] is the command word, Call.Args[1:] are the arguments
//   - pipe / heredoc / redirection edges live in Call.Binding, which also
//     participates in SSA def-use through Call.GetValues()
type astBuilder struct {
	*ssa.FunctionBuilder

	// callees is keyed by builder because a function body gets its own
	// FunctionBuilder; values must be emitted in the function that uses them.
	callees map[*ssa.FunctionBuilder]map[string]ssa.Value
	funcs   map[string]*ssa.Function

	pendingStdin ssa.Value
	inlineDepth  int
	synthetic    int
	wrapperDepth int

	// Loop bodies are unrolled for small literal item lists so value-matching
	// rules can see every iteration; the budget bounds total duplication.
	loopUnrollBudget int
	loopUnrollDepth  int
}

func newASTBuilder(b *ssa.FunctionBuilder) *astBuilder {
	return &astBuilder{
		FunctionBuilder:  b,
		callees:          make(map[*ssa.FunctionBuilder]map[string]ssa.Value),
		funcs:            make(map[string]*ssa.Function),
		loopUnrollBudget: 64,
	}
}

func (ab *astBuilder) buildFile(file *syntax.File) {
	if file == nil {
		return
	}
	ab.registerTopLevelFunctions(file)
	ab.buildStmts(file.Stmts)
}

// registerTopLevelFunctions makes shell function declarations callable even
// when the call appears earlier in the file. Shell resolves functions at
// runtime, so a conservative pre-pass is preferable for safety analysis.
func (ab *astBuilder) registerTopLevelFunctions(file *syntax.File) {
	for _, stmt := range file.Stmts {
		fd, ok := stmt.Cmd.(*syntax.FuncDecl)
		if !ok || fd.Name == nil || fd.Name.Value == "" {
			continue
		}
		if _, exists := ab.funcs[fd.Name.Value]; exists {
			continue
		}
		ab.funcs[fd.Name.Value] = ab.NewFunc(fd.Name.Value)
	}
}

func (ab *astBuilder) buildStmts(stmts []*syntax.Stmt) ssa.Value {
	var out ssa.Value
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		// break/continue finish the block; later statements are unreachable.
		// Building them would still create values in a finished block and
		// pollute the scope merged after the loop.
		if ab.IsBlockFinish() {
			break
		}
		if v := ab.buildStmt(stmt); !utils.IsNil(v) {
			out = v
		}
	}
	return out
}

// setRange points the builder at node's source range and returns a restore
// function. Nodes parsed from an inline eval/sh -c payload carry offsets into
// that synthetic string, so they must not replace CurrentRange.
func (ab *astBuilder) setRange(node syntax.Node) func() {
	if node == nil || ab.synthetic > 0 {
		return func() {}
	}
	editor := ab.GetEditor()
	if editor == nil {
		return func() {}
	}
	pos, end, ok := safeNodeRange(node)
	if !ok {
		return func() {}
	}
	rng := memedit.NewRangeFromOffsets(editor, int(pos.Offset()), int(end.Offset()))
	if rng == nil {
		return func() {}
	}
	return ab.SetRangePure(rng)
}

// safeNodeRange guards mvdan's Pos/End helpers, which index into children:
// an empty *Word (for example `""` lowered through a synthetic word) panics in
// Word.Pos, and malformed/empty nodes should simply lack a source range.
func safeNodeRange(node syntax.Node) (pos, end syntax.Pos, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	pos, end = node.Pos(), node.End()
	return pos, end, pos.IsValid() && end.IsValid()
}

func (ab *astBuilder) calleeValue(name string) ssa.Value {
	name = strings.TrimSpace(name)
	if name == "" {
		name = dynamicCommandName
	}
	table := ab.callees[ab.FunctionBuilder]
	if table == nil {
		table = make(map[string]ssa.Value)
		ab.callees[ab.FunctionBuilder] = table
	}
	if v, ok := table[name]; ok {
		return v
	}
	v := ab.EmitUndefined(name)
	if !utils.IsNil(v) {
		// EmitUndefined only allocates a virtual register; the name index used
		// by SyntaxFlow's bare-name filters (curl, rm, ...) is separate.
		ab.GetProgram().SetInstructionWithName(name, v)
	}
	table[name] = v
	return v
}

func (ab *astBuilder) resolveCallee(name string) ssa.Value {
	if fn, ok := ab.funcs[name]; ok && fn != nil {
		return fn
	}
	return ab.calleeValue(name)
}

func (ab *astBuilder) readOrCreate(name string) ssa.Value {
	if v := ab.ReadValue(name); !utils.IsNil(v) {
		return v
	}
	v := ab.EmitUndefined(name)
	if utils.IsNil(v) {
		return nil
	}
	if variable := ab.CreateVariable(name); variable != nil {
		ab.AssignVariable(variable, v)
	}
	return v
}

func (ab *astBuilder) buildAssign(a *syntax.Assign) ssa.Value {
	if a == nil {
		return nil
	}
	defer ab.setRange(a)()

	if a.Name == nil {
		if a.Value != nil {
			return ab.buildWord(a.Value)
		}
		return nil
	}

	name := a.Name.Value
	var value ssa.Value
	switch {
	case a.Value != nil:
		value = ab.buildWord(a.Value)
	case a.Array != nil:
		value = ab.buildArray(a.Array)
	default:
		value = ab.readOrCreate(name)
	}
	if a.Index != nil {
		ab.buildArithm(a.Index)
	}
	if utils.IsNil(value) {
		value = ab.EmitUndefined(name)
	}
	if a.Append {
		if old := ab.ReadValue(name); !utils.IsNil(old) {
			if merged := ab.EmitBinOp(ssa.OpAdd, old, value); !utils.IsNil(merged) {
				value = merged
			}
		}
	}
	if variable := ab.CreateVariable(name); variable != nil {
		ab.AssignVariable(variable, value)
	}
	return value
}

func (ab *astBuilder) buildArray(a *syntax.ArrayExpr) ssa.Value {
	if a == nil {
		return nil
	}
	var first ssa.Value
	for _, elem := range a.Elems {
		if elem == nil {
			continue
		}
		if elem.Index != nil {
			ab.buildArithm(elem.Index)
		}
		if elem.Value != nil {
			if v := ab.buildWord(elem.Value); !utils.IsNil(v) && utils.IsNil(first) {
				first = v
			}
		}
	}
	if utils.IsNil(first) {
		first = ab.EmitUndefined("array")
	}
	return first
}
