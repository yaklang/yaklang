package bash2ssa

import (
	"path"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"mvdan.cc/sh/v3/syntax"
)

func (ab *astBuilder) buildCallExpr(cmd *syntax.CallExpr, redirs []*syntax.Redirect, stdin ssa.Value) ssa.Value {
	if cmd == nil {
		return nil
	}
	defer ab.setRange(cmd)()

	if utils.IsNil(stdin) {
		stdin = ab.pendingStdin
	}
	ab.pendingStdin = nil
	if len(cmd.Args) == 0 {
		ab.lastSubst = nil
		for _, a := range cmd.Assigns {
			ab.buildAssign(a)
		}
		status := ab.lastSubst
		if status == nil {
			status = ab.EmitConstInst(0)
		}
		ab.put(statusName, status)
		return nil
	}
	word := ab.buildWord(cmd.Args[0])
	fields, spread := ab.words(cmd.Args[1:])
	// Command identity uses the raw word text; value expansion stays decoded.
	name, isStatic := rawStaticWord(cmd.Args[0])
	if isStatic {
		word = ab.EmitConstInst(name)
	}
	var fn *ssa.Function
	if isStatic && isPlainName(name) {
		fn = ab.funcs[name]
		if fn == nil {
			// A call may precede the FuncDecl that defines it. The pre-indexed
			// function already owns the name, so bind this call to it now; its
			// $@ parameter and return slots are filled when the body is built.
			fn = ab.forward[name]
		}
	}
	// Expansion precedes temporary command-environment assignments.
	for _, a := range cmd.Assigns {
		if a.Name != nil {
			n := a.Name.Value
			prior := ab.readOrCreate(n)
			defer func() {
				if !ab.IsBlockFinish() {
					ab.put(n, prior)
				}
			}()
		}
		ab.buildAssign(a)
	}
	if fn == nil && isStatic && name == "return" && ab.inFunction && ab.childDepth == 0 {
		status := ab.ReadValue(statusName)
		if len(fields) > 0 {
			if c, ok := ssa.ToConstInst(fields[0]); ok {
				n, err := strconv.ParseInt(c.String(), 10, 64)
				if err == nil {
					status = ab.EmitConstInst(int(n & 255))
				} else {
					status = ab.EmitConstInst(2)
				}
			} else {
				status = ab.EmitTypeCast(ab.number(fields[0]), ssa.CreateByteType())
			}
		}
		ab.shellReturn(status)
		return nil
	}
	var callee ssa.Value
	args := fields
	if fn != nil {
		callee = fn
	} else {
		args = append([]ssa.Value{word}, fields...)
		if isStatic {
			callee = ab.calleeValue(commandBase(name))
		} else {
			callee = ab.calleeValue(dynamicCommandName)
		}
	}
	call := ab.NewCall(callee, args)
	if fn != nil && fn.Type != nil {
		for _, fv := range fn.Type.FreeValue {
			if v := ab.PeekValue(fv.GetName()); v != nil {
				call.Binding[fv.GetName()] = v.GetId()
			}
		}
	}
	if fn != nil && ab.needsArgv(fn) {
		// A shell function has no fixed arity. Its single $@ parameter owns the
		// positional vector, and $1/$#/$* read members of that vector. Attach
		// the vector to the call so the callee and top-def analysis see the
		// same value; omitted positions expand to the empty string.
		call.IsEllipsis = spread
		call.Binding[ssa.VariadicArgsBinding] = ab.packFields(fields, spread).GetId()
		call.Binding[ssa.VariadicDefaultBinding] = ab.EmitConstInst("").GetId()
	}
	for fd, id := range ab.redirects {
		call.Binding[fd] = id
	}
	if !utils.IsNil(stdin) {
		call.Binding["stdin"] = stdin.GetId()
	}
	ab.applyRedirs(call, redirs)

	ret := ab.EmitCall(call)
	if ret == nil {
		return nil
	}

	var output ssa.Value = ret
	var status ssa.Value
	if fn != nil {
		output = ab.ReadMemberCallValue(ret, ab.EmitConstInst(0))
		status = ab.ReadMemberCallValue(ret, ab.EmitConstInst(1))
		if v := ab.knownReturn(fn, 0); v != nil {
			output = v
		}
		if v := ab.knownReturn(fn, 1); v != nil {
			status = v
		}
	} else if isStatic {
		// TODO(ssa-bash): trap handlers are not registered; their commands never join the CFG.
		switch name {
		case "true", ":", "false":
			n := 0
			if name == "false" {
				n = 1
			}
			status = ab.EmitConstInst(n)
			output = nil
		case "echo", "printf":
			args := fields
			for len(args) > 0 {
				c, ok := ssa.ToConstInst(args[0])
				if !ok || !strings.HasPrefix(c.String(), "-") || c.String() == "-" {
					break
				}
				args = args[1:]
			}
			sep := " "
			if name == "printf" {
				sep = ""
				if len(args) > 0 {
					if c, ok := ssa.ToConstInst(args[0]); !ok || strings.ContainsRune(c.String(), '%') {
						break
					}
				}
			}
			output = ab.join(args, sep)
			status = ab.EmitConstInst(0)
		case "set":
			// TODO(ssa-bash): only set -- is modeled; set -e/-u/-o options do not change later control flow.
			if len(cmd.Args) >= 2 && cmd.Args[1].Lit() == "--" {
				ab.put("$@", ab.packFields(fields[1:], spread))
				status = ab.EmitConstInst(0)
				output = nil
			}
		case "shift":
			status = ab.shift(fields)
			output = nil
		case "[", "test":
			output = nil
		case "read":
			ab.applyRead(cmd.Args, ret)
			output = nil
		case "break", "continue":
			depth := 1
			if len(fields) == 1 {
				if c, ok := ssa.ToConstInst(fields[0]); ok {
					depth, _ = strconv.Atoi(c.String())
				} else {
					depth = 0
				}
			}
			if ab.loopDepth > 0 && len(fields) <= 1 && depth > 0 {
				ab.put(statusName, ab.EmitConstInst(0))
				if name == "continue" {
					ab.ContinueLoop(min(depth, ab.loopDepth))
				} else {
					ab.BreakLoop(min(depth, ab.loopDepth))
				}
				return nil
			}
		case "source", ".":
			if len(cmd.Args) > 1 {
				if result, ok := ab.source(cmd.Args[1], fields, spread); ok {
					return result
				}
			}
		}
	}
	if status == nil {
		// TODO(ssa-bash): a non-constant exit status stays an intrinsic; static value matching sees no number.
		status = ab.intrinsic("exit-status", ret)
	}
	ab.put(statusName, status)
	if fn == nil && isStatic {
		ab.emitWrappedCommands(cmd.Args, ret, stdin)
	}
	if ab.stdoutSlot != "" && output != nil && !ab.IsBlockFinish() {
		ab.put(ab.stdoutSlot, ab.concat(ab.ReadValue(ab.stdoutSlot), output))
	}
	return output
}

// TODO(ssa-bash): read assigns one value to every target; field-by-field splitting of `read a b` is not modeled.
func (ab *astBuilder) applyRead(args []*syntax.Word, value ssa.Value) {
	for _, name := range readTargetNames(args) {
		ab.put(name, value)
	}
}

func (ab *astBuilder) shift(fields []ssa.Value) ssa.Value {
	n := int64(1)
	vec := ab.positional("@")
	if len(fields) > 0 {
		c, ok := ssa.ToConstInst(fields[0])
		if !ok {
			ab.put("$@", ab.intrinsic("shift", vec, fields[0]))
			return ab.intrinsic("shift-status", vec, fields[0])
		}
		var err error
		n, err = strconv.ParseInt(c.String(), 10, 64)
		if err != nil || n < 0 {
			return ab.EmitConstInst(1)
		}
	}
	if args, ok := ab.argvFrames[vec.GetId()]; ok {
		if n > int64(len(args)) {
			return ab.EmitConstInst(1)
		}
		ab.put("$@", ab.argv(args[n:]))
		return ab.EmitConstInst(0)
	}
	count := ab.EmitConstInst(n)
	ab.put("$@", ab.intrinsic("shift", vec, count))
	return ab.intrinsic("shift-status", vec, count)
}

// commandBase resolves the command identity. Paths are reduced to their base
// name so /bin/rm and rm both match `rm` rules; the raw path is still available
// in the call's source range.
func commandBase(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return dynamicCommandName
	}
	if strings.ContainsAny(name, `/\`) {
		return path.Base(strings.ReplaceAll(name, `\`, "/"))
	}
	return name
}

func (ab *astBuilder) applyRedirs(call *ssa.Call, redirs []*syntax.Redirect) {
	ab.bindRedirs(call.Binding, redirs)
}
func (ab *astBuilder) bindRedirs(binding map[string]int64, redirs []*syntax.Redirect) {
	for _, redir := range redirs {
		if redir == nil {
			continue
		}
		var value ssa.Value
		switch redir.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			// TODO(ssa-bash): heredoc newline and tab normalization (<<, <<-) is not modeled.
			value = ab.buildWord(redir.Hdoc)
		default:
			value = ab.buildWord(redir.Word)
		}
		if utils.IsNil(value) {
			continue
		}
		id := value.GetId()
		switch redir.Op {
		case syntax.RdrIn, syntax.DplIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			binding["stdin"] = id
		case syntax.RdrAll, syntax.AppAll:
			binding["stdout"] = id
			binding["stderr"] = id
		case syntax.RdrOut, syntax.AppOut, syntax.ClbOut, syntax.DplOut:
			switch {
			case fdIsStdin(redir.N):
				binding["stdin"] = id
			case fdIsStderr(redir.N):
				binding["stderr"] = id
			default:
				binding["stdout"] = id
			}
		case syntax.RdrInOut:
			binding["stdin"] = id
			binding["stdout"] = id
		}
	}
}

func fdIsStdin(fd *syntax.Lit) bool {
	return fd != nil && strings.TrimSpace(fd.Value) == "0"
}

func fdIsStderr(fd *syntax.Lit) bool {
	return fd != nil && strings.TrimSpace(fd.Value) == "2"
}

func (ab *astBuilder) emitWrappedCommands(args []*syntax.Word, outer *ssa.Call, stdin ssa.Value) {
	if len(args) == 0 || ab.wrapperDepth >= maxWrapperDepth {
		return
	}
	name, ok := staticWord(args[0])
	if !ok {
		return
	}
	base := commandBase(name)

	switch base {
	case "eval":
		if len(args) < 2 {
			return
		}
		if src, ok := staticWord(args[1]); ok {
			ab.buildInlineScript(src, outer)
		}
		// TODO(ssa-bash): eval with a non-constant payload is not parsed or inlined.
	case "sh", "bash", "zsh", "dash", "ksh", "ash":
		if idx := commandFlagIndex(args, "-c"); idx >= 0 && idx+1 < len(args) {
			// TODO(ssa-bash): a non-constant sh -c payload is not parsed.
			if src, ok := staticWord(args[idx+1]); ok {
				ab.childShell(func() ssa.Value { ab.buildInlineScript(src, outer); return nil })
			}
		}
	case "sudo", "env", "timeout", "nice", "ionice", "nohup", "exec", "command", "builtin", "stdbuf":
		start := wrapperCommandIndex(base, args)
		if start > 0 && start < len(args) {
			ab.buildNestedCall(args[start:], outer, stdin)
		}
	case "xargs":
		start := xargsCommandIndex(args)
		if start > 0 && start < len(args) {
			ab.buildNestedCall(args[start:], outer, stdin)
		}
	case "find":
		start, end := findExecCommand(args)
		if start > 0 && start < end {
			ab.buildNestedCall(args[start:end], outer, stdin)
		}
	}
}

func (ab *astBuilder) buildNestedCall(words []*syntax.Word, outer *ssa.Call, stdin ssa.Value) {
	if len(words) == 0 || ab.wrapperDepth >= maxWrapperDepth {
		return
	}
	ab.wrapperDepth++
	defer func() { ab.wrapperDepth-- }()

	ret := ab.buildCallExpr(&syntax.CallExpr{Args: words}, nil, stdin)
	inner, ok := ret.(*ssa.Call)
	if !ok || inner == nil || outer == nil {
		return
	}
	// The wrapper passes its streams through to the effective command, so a
	// pipeline such as `curl ... | sudo bash` must keep the stdin edge.
	for _, key := range []string{"stdin", "stdout", "stderr"} {
		if id, ok := outer.Binding[key]; ok {
			inner.Binding[key] = id
		}
	}
	inner.Binding["wrapped_by"] = outer.GetId()
}

func (ab *astBuilder) buildInlineScript(src string, outer *ssa.Call) {
	src = strings.TrimSpace(src)
	if src == "" || ab.inlineDepth >= maxInlineScriptDepth {
		return
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return
	}

	ab.inlineDepth++
	ab.synthetic++
	defer func() {
		ab.synthetic--
		ab.inlineDepth--
	}()
	if outer != nil {
		ab.SetRangeByRangeIf(outer.GetRange())
	}
	ab.buildStmts(file.Stmts)
}

func commandFlagIndex(args []*syntax.Word, flag string) int {
	for i := 1; i < len(args); i++ {
		if v, ok := staticWord(args[i]); ok && v == flag {
			return i
		}
	}
	return -1
}
