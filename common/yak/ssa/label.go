package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"strings"
)

// use in for/switch
type target struct {
	tail         *target // the stack
	LabelTarget  ssautil.LabelTarget[Value]
	_break       *BasicBlock
	_continue    *BasicBlock
	_fallthrough *BasicBlock
}

// target stack
func (b *FunctionBuilder) PushTarget(scope ssautil.LabelTarget[Value], _break, _continue, _fallthrough *BasicBlock) {
	b.target = &target{
		tail:         b.target,
		LabelTarget:  scope,
		_break:       _break,
		_continue:    _continue,
		_fallthrough: _fallthrough,
	}
}

func (b *FunctionBuilder) PopTarget() bool {
	b.target = b.target.tail
	if b.target == nil {
		// b.NewError(Error, SSATAG, "error target struct this position when build")
		return false
	} else {
		return true
	}
}

func (b *FunctionBuilder) Break() bool {
	for target := b.target; target != nil; target = target.tail {
		if target._break != nil {
			target.LabelTarget.Break(b.CurrentBlock.ScopeTable)
			b.EmitJump(target._break)
			return true
		}
	}
	return false
}

// BreakLoop jumps to the break target of the depth-th enclosing loop.
// Depth one is the innermost loop. Switch and label targets are skipped.
func (b *FunctionBuilder) BreakLoop(depth int) bool {
	return b.branchToLoop(depth, false)
}

// ContinueLoop jumps to the continue target of the depth-th enclosing loop.
// Shells need depth here because `break N`/`continue N` count loops only,
// while Break/Continue stop at the nearest target, which may be a switch.
func (b *FunctionBuilder) ContinueLoop(depth int) bool {
	return b.branchToLoop(depth, true)
}

func (b *FunctionBuilder) branchToLoop(depth int, continueLoop bool) bool {
	if depth <= 0 {
		return false
	}
	for t := b.target; t != nil; t = t.tail {
		if t._continue == nil || t._break == nil {
			continue
		}
		depth--
		if depth != 0 {
			continue
		}
		if continueLoop {
			t.LabelTarget.Continue(b.CurrentBlock.ScopeTable)
			b.EmitJump(t._continue)
		} else {
			t.LabelTarget.Break(b.CurrentBlock.ScopeTable)
			b.EmitJump(t._break)
		}
		return true
	}
	return false
}

func (b *FunctionBuilder) BreakWithLabelName(labelName string) bool {
	for target := b.target; target != nil; target = target.tail {
		if target._break != nil && strings.Contains(target._break.name, labelName) {
			target.LabelTarget.Break(b.CurrentBlock.ScopeTable)
			b.EmitJump(target._break)
			return true
		}
	}
	return false
}

func (b *FunctionBuilder) Continue() bool {
	for target := b.target; target != nil; target = target.tail {
		if target._continue != nil {
			target.LabelTarget.Continue(b.CurrentBlock.ScopeTable)
			b.EmitJump(target._continue)
			return true
		}
	}
	return false
}

func (b *FunctionBuilder) ContinueWithLabelName(labelName string) bool {
	for target := b.target; target != nil; target = target.tail {
		if target._continue != nil && strings.Contains(target._continue.name, labelName) {
			target.LabelTarget.Continue(b.CurrentBlock.ScopeTable)
			b.EmitJump(target._continue)
			return true
		}
	}
	return false
}

func (b *FunctionBuilder) Fallthrough() bool {
	for target := b.target; target != nil; target = target.tail {
		if target._fallthrough != nil {
			target.LabelTarget.FallThough(b.CurrentBlock.ScopeTable)
			b.EmitJump(target._fallthrough)
			return true
		}
	}
	return false
}

// for goto and label
func (b *FunctionBuilder) AddLabel(name string, block *BasicBlock) {
	b.labels[name] = block
}

func (b *FunctionBuilder) GetLabel(name string) *BasicBlock {
	if b, ok := b.labels[name]; ok {
		return b
	} else {
		return nil
	}
}

func (b *FunctionBuilder) DeleteLabel(name string) {
	delete(b.labels, name)
}
