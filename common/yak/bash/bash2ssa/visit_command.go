package bash2ssa

import (
	"path"
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

	for _, assign := range cmd.Assigns {
		ab.buildAssign(assign)
	}

	if utils.IsNil(stdin) {
		stdin = ab.pendingStdin
	}
	ab.pendingStdin = nil

	if len(cmd.Args) == 0 {
		return nil
	}

	args := make([]ssa.Value, 0, len(cmd.Args))
	for _, word := range cmd.Args {
		value := ab.buildWord(word)
		if utils.IsNil(value) {
			value = ab.EmitUndefined("")
		}
		args = append(args, value)
	}

	name, isStatic := staticWord(cmd.Args[0])
	var callee ssa.Value
	if isStatic && strings.TrimSpace(name) != "" {
		callee = ab.resolveCallee(commandBase(name))
	} else {
		callee = ab.calleeValue(dynamicCommandName)
	}
	if utils.IsNil(callee) {
		return nil
	}

	call := ab.NewCall(callee, args)
	if call == nil {
		return nil
	}
	if !utils.IsNil(stdin) {
		call.Binding["stdin"] = stdin.GetId()
	}
	ab.applyRedirs(call, redirs)

	ret := ab.EmitCall(call)
	if ret == nil {
		return nil
	}

	if isStatic {
		ab.emitWrappedCommands(cmd.Args, ret, stdin)
		ab.applyBuiltinEffect(commandBase(name), cmd.Args, ret)
	}
	return ret
}

// applyBuiltinEffect models shell builtins that mutate variables or control
// flow. `read` binds its arguments to the command output, and break/continue
// jump to the enclosing loop target so the loop Phi sees the exit value.
func (ab *astBuilder) applyBuiltinEffect(name string, args []*syntax.Word, value ssa.Value) {
	switch name {
	case "read":
		if utils.IsNil(value) {
			return
		}
		for _, target := range readTargetNames(args) {
			if variable := ab.CreateVariable(target); variable != nil {
				ab.AssignVariable(variable, value)
			}
		}
	case "break":
		ab.Break()
	case "continue":
		ab.Continue()
	}
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
	if call == nil {
		return
	}
	for _, redir := range redirs {
		if redir == nil {
			continue
		}
		var value ssa.Value
		switch redir.Op {
		case syntax.Hdoc, syntax.DashHdoc:
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
			call.Binding["stdin"] = id
		case syntax.RdrAll, syntax.AppAll:
			call.Binding["stdout"] = id
			call.Binding["stderr"] = id
		case syntax.RdrOut, syntax.AppOut, syntax.ClbOut, syntax.DplOut:
			switch {
			case fdIsStdin(redir.N):
				call.Binding["stdin"] = id
			case fdIsStderr(redir.N):
				call.Binding["stderr"] = id
			default:
				call.Binding["stdout"] = id
			}
		case syntax.RdrInOut:
			call.Binding["stdin"] = id
			call.Binding["stdout"] = id
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
	case "sh", "bash", "zsh", "dash", "ksh", "ash":
		if idx := commandFlagIndex(args, "-c"); idx >= 0 && idx+1 < len(args) {
			if src, ok := staticWord(args[idx+1]); ok {
				ab.buildInlineScript(src, outer)
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
