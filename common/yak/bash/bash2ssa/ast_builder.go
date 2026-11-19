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
	callees  map[*ssa.FunctionBuilder]map[string]ssa.Value
	funcs    map[string]*ssa.Function
	declared map[*syntax.FuncDecl]*ssa.Function
	forward  map[string]*ssa.Function

	redirects                     map[string]int64
	pendingStdin                  ssa.Value
	inlineDepth                   int
	synthetic                     int
	wrapperDepth                  int
	serial, loopDepth, childDepth int
	inFunction                    bool
	// argvFrames contains the known fields of immutable positional vectors.
	argvFrames map[int64][]ssa.Value
	assoc      map[string]bool
	sourcing   map[string]bool
	stdoutSlot string
	lastSubst  ssa.Value

	// Loop bodies are unrolled for small literal item lists so value-matching
	// rules can see every iteration; the budget bounds total duplication.
	loopUnrollBudget int
	loopUnrollDepth  int
}

func newASTBuilder(b *ssa.FunctionBuilder) *astBuilder {
	return &astBuilder{
		FunctionBuilder: b,
		callees:         make(map[*ssa.FunctionBuilder]map[string]ssa.Value),
		funcs:           make(map[string]*ssa.Function),
		argvFrames:      make(map[int64][]ssa.Value), assoc: make(map[string]bool), sourcing: make(map[string]bool),
		loopUnrollBudget: 64,
	}
}

func (ab *astBuilder) buildFile(file *syntax.File) {
	if file == nil {
		return
	}
	ab.SupportClosure = true
	ab.AssignVariable(ab.CreateLocalVariable(statusName), ab.EmitConstInst(0))
	ab.forward = make(map[string]*ssa.Function)
	ab.declared = make(map[*syntax.FuncDecl]*ssa.Function)
	ab.indexFunctions(file)
	ab.buildStmts(file.Stmts)
}

// Bodies may call a function declared later in the file. A direct command at
// file scope still uses only definitions executed up to that point.
func (ab *astBuilder) indexFunctions(f *syntax.File) {
	for _, s := range f.Stmts {
		if d, ok := s.Cmd.(*syntax.FuncDecl); ok && ab.declared[d] == nil {
			restore := ab.setRange(d)
			fn := ab.NewFunc(d.Name.Value)
			restore()
			ab.declared[d] = fn
			ab.forward[d.Name.Value] = fn
		}
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
		var v ssa.Value
		if stmt.Background || stmt.Coprocess {
			ab.childShell(func() ssa.Value { return ab.buildStmt(stmt) })
			ab.put(statusName, ab.EmitConstInst(0))
		} else {
			v = ab.buildStmt(stmt)
		}
		if !utils.IsNil(v) && !ab.IsBlockFinish() {
			out = ab.concat(out, v)
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
	// Callees are shared across branches. Their definitions must dominate
	// all uses, not sit in the first arm that happens to call the command.
	previous := ab.CurrentBlock
	if entry, ok := ab.GetBasicBlockByID(ab.EnterBlock); ok {
		ab.CurrentBlock = entry
	}
	v := ab.EmitUndefined(name)
	ab.CurrentBlock = previous
	if !utils.IsNil(v) {
		// EmitUndefined only allocates a virtual register; the name index used
		// by SyntaxFlow's bare-name filters (curl, rm, ...) is separate.
		ab.GetProgram().SetInstructionWithName(name, v)
	}
	table[name] = v
	return v
}

func (ab *astBuilder) readOrCreate(name string) ssa.Value {
	if v := ab.PeekValue(name); !utils.IsNil(v) && !v.IsExtern() {
		if _, hostLib := ssa.ToExternLib(v); !hostLib {
			return v
		}
	}
	// A shell name may come from the caller's environment, but never from a
	// Yak plugin's extern library of the same name.
	v := ab.EmitValueOnlyDeclare(name)
	ab.put(name, v)
	return v
}

func (ab *astBuilder) buildAssign(a *syntax.Assign) ssa.Value { return ab.assign(a, false) }
func (ab *astBuilder) assign(a *syntax.Assign, local bool) ssa.Value {
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
		value = ab.arrayLiteral(name, a.Array)
	default:
		value = ab.EmitConstInst("")
	}
	if a.Index != nil {
		return ab.assignElement(name, ab.arrayKey(name, a.Index), value, a.Append)
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
	if local {
		ab.AssignVariable(ab.CreateLocalVariable(name), value)
	} else {
		ab.put(name, value)
	}
	return value
}
