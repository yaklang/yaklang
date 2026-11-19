package bash2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/pattern"
	"mvdan.cc/sh/v3/syntax"
	"regexp"
)

// choose evaluates each alternative in its own block. Only live predecessors
// assign the result. In particular a skipped expansion cannot write a variable.
func (ab *astBuilder) choose(cond ssa.Value, yes, no func() ssa.Value) ssa.Value {
	if c, ok := ssa.ToConstInst(cond); ok && c.IsBoolean() {
		if c.Boolean() {
			return yes()
		}
		return no()
	}
	result := ab.slot("choice", ab.EmitConstInst(""))
	b := ab.CreateIfBuilder()
	arm := func(f func() ssa.Value) func() {
		return func() {
			if v := f(); v != nil && !ab.IsBlockFinish() {
				ab.put(result, v)
			}
		}
	}
	b.SetCondition(func() ssa.Value { return cond }, arm(yes))
	b.SetElse(arm(no))
	b.Build()
	ab.foldTrivialPhis()
	return ab.ReadValue(result)
}

func (ab *astBuilder) parameterOperator(name string, v ssa.Value, e *syntax.Expansion) ssa.Value {
	word := func() ssa.Value {
		if e.Word == nil {
			return ab.EmitConstInst("")
		}
		return ab.buildWord(e.Word)
	}
	switch e.Op {
	case syntax.AlternateUnset, syntax.AlternateUnsetOrNull, syntax.DefaultUnset, syntax.DefaultUnsetOrNull, syntax.AssignUnset, syntax.AssignUnsetOrNull:
		var set ssa.Value = ab.EmitConstInst(true)
		if _, ok := ssa.ToConstInst(v); !ok {
			set = ab.intrinsic("is-set", v)
		}
		switch e.Op {
		case syntax.AlternateUnsetOrNull, syntax.DefaultUnsetOrNull, syntax.AssignUnsetOrNull:
			set = ab.EmitBinOp(ssa.OpLogicAnd, set, ab.EmitBinOp(ssa.OpNotEq, v, ab.EmitConstInst("")))
		}
		if e.Op == syntax.AlternateUnset || e.Op == syntax.AlternateUnsetOrNull {
			return ab.choose(set, word, func() ssa.Value { return ab.EmitConstInst("") })
		}
		return ab.choose(set, func() ssa.Value { return v }, func() ssa.Value {
			out := word()
			if e.Op == syntax.AssignUnset || e.Op == syntax.AssignUnsetOrNull {
				ab.put(name, out)
			}
			return out
		})
	default:
		// TODO(ssa-bash): pattern trimming, replacement and case conversion stay expand.* intrinsic calls.
		return ab.intrinsic("expand."+e.Op.String(), v, word())
	}
}

func (ab *astBuilder) match(subject ssa.Value, w *syntax.Word) ssa.Value {
	if _, ok := staticWord(w); ok {
		pat, err := expand.Pattern(&expand.Config{Env: expand.ListEnviron()}, w)
		if err == nil {
			if pat == "*" {
				return ab.EmitConstInst(true)
			}
			if c, ok := ssa.ToConstInst(subject); ok {
				if rx, err := pattern.Regexp(pat, 0); err == nil {
					if m, err := regexp.MatchString("^(?:"+rx+")$", c.String()); err == nil {
						return ab.EmitConstInst(m)
					}
				}
			}
			if !pattern.HasMeta(pat, 0) {
				return ab.EmitBinOp(ssa.OpEq, subject, ab.buildWord(w))
			}
			return ab.intrinsic("match", subject, ab.EmitConstInst(pat))
		}
	}
	return ab.intrinsic("match", subject, ab.buildWord(w))
}
func (ab *astBuilder) buildTest(e syntax.TestExpr) ssa.Value {
	switch x := e.(type) {
	case *syntax.Word:
		return ab.EmitBinOp(ssa.OpNotEq, ab.buildWord(x), ab.EmitConstInst(""))
	case *syntax.ParenTest:
		return ab.buildTest(x.X)
	case *syntax.UnaryTest:
		if x.Op == syntax.TsNot {
			return ab.EmitUnOp(ssa.OpNot, ab.buildTest(x.X))
		}
		if w, ok := x.X.(*syntax.Word); ok {
			v := ab.buildWord(w)
			switch x.Op {
			case syntax.TsEmpStr:
				return ab.EmitBinOp(ssa.OpEq, v, ab.EmitConstInst(""))
			case syntax.TsNempStr:
				return ab.EmitBinOp(ssa.OpNotEq, v, ab.EmitConstInst(""))
			}
			return ab.intrinsic("test."+x.Op.String(), v)
		}
	case *syntax.BinaryTest:
		if x.Op == syntax.AndTest || x.Op == syntax.OrTest {
			left := ab.buildTest(x.X)
			right := func() ssa.Value { return ab.buildTest(x.Y) }
			if x.Op == syntax.AndTest {
				return ab.choose(left, right, func() ssa.Value { return left })
			}
			return ab.choose(left, func() ssa.Value { return left }, right)
		}
		l, lok := x.X.(*syntax.Word)
		r, rok := x.Y.(*syntax.Word)
		if !lok || !rok {
			break
		}
		a := ab.buildWord(l)
		switch x.Op {
		case syntax.TsMatchShort, syntax.TsMatch:
			return ab.match(a, r)
		case syntax.TsNoMatch:
			return ab.EmitUnOp(ssa.OpNot, ab.match(a, r))
		}
		b := ab.buildWord(r)
		if op, ok := map[syntax.BinTestOperator]ssa.BinaryOpcode{syntax.TsEql: ssa.OpEq, syntax.TsNeq: ssa.OpNotEq, syntax.TsLeq: ssa.OpLtEq, syntax.TsLss: ssa.OpLt, syntax.TsGeq: ssa.OpGtEq, syntax.TsGtr: ssa.OpGt}[x.Op]; ok {
			return ab.EmitBinOp(op, ab.number(a), ab.number(b))
		}
		return ab.intrinsic("test."+x.Op.String(), a, b)
	}
	return ab.EmitUndefined("[[ ]]")
}

func (ab *astBuilder) buildCaseClause(c *syntax.CaseClause) ssa.Value {
	ab.declareMergeTargets(c)
	subject := ab.buildWord(c.Word)
	ab.put(statusName, ab.EmitConstInst(0))
	out := ab.slot("case.stdout", ab.EmitConstInst(""))
	matches := func(it *syntax.CaseItem) ssa.Value {
		var v ssa.Value
		for _, w := range it.Patterns {
			if v == nil {
				v = ab.match(subject, w)
			} else {
				prev := v
				v = ab.choose(prev, func() ssa.Value { return prev }, func() ssa.Value { return ab.match(subject, w) })
			}
		}
		return v
	}
	body := func(it *syntax.CaseItem) {
		if v := ab.buildStmts(it.Stmts); v != nil && !ab.IsBlockFinish() {
			ab.put(out, ab.concat(ab.ReadValue(out), v))
		}
	}
	// Ordinary ;; cases form one if/elif chain, with the first bare * as else.
	extended := false
	for _, it := range c.Items {
		if it.Op == syntax.Fallthrough || it.Op == syntax.Resume {
			extended = true
		}
	}
	if !extended {
		b := ab.CreateIfBuilder()
		count := 0
		for _, it := range c.Items {
			it := it
			if len(it.Patterns) == 1 && it.Patterns[0].Lit() == "*" {
				if count == 0 {
					body(it)
					return ab.ReadValue(out)
				}
				b.SetElse(func() { body(it) })
				break
			}
			b.AppendItem(func() ssa.Value { return matches(it) }, func() { body(it) })
			count++
		}
		if count > 0 {
			b.Build()
			ab.foldTrivialPhis()
		}
		return ab.ReadValue(out)
	}
	// test, force, done: ;& executes the next body without testing its pattern;
	// ;;& resumes pattern testing. Each arm is compiled once, even in long chains.
	state := ab.slot("case.state", ab.EmitConstInst("test"))
	for _, it := range c.Items {
		it := it
		v := ab.ReadValue(state)
		cond := ab.choose(ab.EmitBinOp(ssa.OpEq, v, ab.EmitConstInst("test")), func() ssa.Value { return matches(it) }, func() ssa.Value { return ab.EmitBinOp(ssa.OpEq, v, ab.EmitConstInst("force")) })
		b := ab.CreateIfBuilder()
		b.SetCondition(func() ssa.Value { return cond }, func() {
			body(it)
			if ab.IsBlockFinish() {
				return
			}
			next := "done"
			switch it.Op {
			case syntax.Fallthrough:
				next = "force"
			case syntax.Resume:
				next = "test"
			}
			ab.put(state, ab.EmitConstInst(next))
		})
		b.Build()
		ab.foldTrivialPhis()
	}
	return ab.ReadValue(out)
}
