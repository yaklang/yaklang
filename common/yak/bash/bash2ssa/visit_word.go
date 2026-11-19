package bash2ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"mvdan.cc/sh/v3/syntax"
)

// staticWord returns the literal text of a word when no expansion is present.
// Single and double quotes are both static here; the caller only uses this for
// command identity and wrapper/eval payload extraction.
func staticWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	return staticParts(word.Parts)
}

func staticParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			inner, ok := staticParts(p.Parts)
			if !ok {
				return "", false
			}
			b.WriteString(inner)
		default:
			return "", false
		}
	}
	return b.String(), true
}

func (ab *astBuilder) buildWord(word *syntax.Word) ssa.Value {
	if word == nil {
		return nil
	}
	if len(word.Parts) == 0 {
		return ab.EmitConstInst("")
	}
	defer ab.setRange(word)()

	switch len(word.Parts) {
	case 1:
		return ab.buildWordPart(word.Parts[0])
	}

	var out ssa.Value
	for _, part := range word.Parts {
		value := ab.buildWordPart(part)
		if utils.IsNil(value) {
			continue
		}
		if utils.IsNil(out) {
			out = value
			continue
		}
		if merged := ab.EmitBinOp(ssa.OpAdd, out, value); !utils.IsNil(merged) {
			out = merged
		}
	}
	return out
}

func (ab *astBuilder) buildWordPart(part syntax.WordPart) ssa.Value {
	switch p := part.(type) {
	case *syntax.Lit:
		return ab.EmitConstInst(p.Value)
	case *syntax.SglQuoted:
		return ab.EmitConstInst(p.Value)
	case *syntax.DblQuoted:
		if len(p.Parts) == 0 {
			return ab.EmitConstInst("")
		}
		return ab.buildWord(&syntax.Word{Parts: p.Parts})
	case *syntax.ParamExp:
		return ab.buildParamExp(p)
	case *syntax.CmdSubst:
		return ab.buildCmdSubst(p)
	case *syntax.ArithmExp:
		return ab.buildArithm(p.X)
	case *syntax.ProcSubst:
		return ab.buildProcSubst(p)
	case *syntax.ExtGlob:
		if p.Pattern != nil {
			return ab.EmitConstInst(p.Pattern.Value)
		}
		return ab.EmitUndefined("glob")
	case *syntax.BraceExp:
		return ab.buildBraceExp(p)
	default:
		return ab.EmitUndefined("")
	}
}

func (ab *astBuilder) buildParamExp(exp *syntax.ParamExp) ssa.Value {
	if exp == nil {
		return nil
	}
	defer ab.setRange(exp)()

	name := ""
	if exp.Param != nil {
		name = exp.Param.Value
	}
	if name == "" {
		name = "$?"
	}
	switch name {
	case "@", "*", "#", "?", "$", "!", "-", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		name = "$" + name
	}

	value := ab.readOrCreate(name)

	if exp.Exp != nil && exp.Exp.Word != nil {
		if fallback := ab.buildWord(exp.Exp.Word); !utils.IsNil(fallback) {
			if utils.IsNil(value) {
				value = fallback
			} else if merged := ab.EmitPhi(name+"$expansion", ssa.Values{value, fallback}); !utils.IsNil(merged) {
				value = merged
			}
		}
	}
	if exp.Index != nil {
		ab.buildArithm(exp.Index)
	}
	if exp.Slice != nil {
		ab.buildArithm(exp.Slice.Offset)
		ab.buildArithm(exp.Slice.Length)
	}
	if exp.Repl != nil {
		ab.buildWord(exp.Repl.Orig)
		ab.buildWord(exp.Repl.With)
	}
	return value
}

func (ab *astBuilder) buildCmdSubst(subst *syntax.CmdSubst) ssa.Value {
	if subst == nil {
		return nil
	}
	defer ab.setRange(subst)()

	previous := ab.pendingStdin
	ab.pendingStdin = nil
	out := ab.buildStmts(subst.Stmts)
	ab.pendingStdin = previous
	if utils.IsNil(out) {
		out = ab.EmitUndefined("$()")
	}
	return out
}

func (ab *astBuilder) buildProcSubst(subst *syntax.ProcSubst) ssa.Value {
	if subst == nil {
		return nil
	}
	defer ab.setRange(subst)()

	previous := ab.pendingStdin
	ab.pendingStdin = nil
	out := ab.buildStmts(subst.Stmts)
	ab.pendingStdin = previous
	if utils.IsNil(out) {
		out = ab.EmitUndefined("$()")
	}
	return out
}

func (ab *astBuilder) buildBraceExp(exp *syntax.BraceExp) ssa.Value {
	if exp == nil {
		return nil
	}
	var out ssa.Value
	for _, elem := range exp.Elems {
		value := ab.buildWord(elem)
		if utils.IsNil(value) {
			continue
		}
		if utils.IsNil(out) {
			out = value
		}
	}
	if utils.IsNil(out) {
		out = ab.EmitUndefined("brace")
	}
	return out
}

func (ab *astBuilder) buildArithm(expr syntax.ArithmExpr) ssa.Value {
	if expr == nil {
		return nil
	}
	defer ab.setRange(expr)()

	switch e := expr.(type) {
	case *syntax.Word:
		return ab.buildArithmWord(e)
	case *syntax.ParenArithm:
		return ab.buildArithm(e.X)
	case *syntax.UnaryArithm:
		if e.Op == syntax.Inc || e.Op == syntax.Dec {
			return ab.buildArithmStep(e)
		}
		value := ab.buildArithm(e.X)
		if utils.IsNil(value) {
			return nil
		}
		switch e.Op {
		case syntax.Not:
			return ab.EmitUnOp(ssa.OpNot, value)
		case syntax.BitNegation:
			return ab.EmitUnOp(ssa.OpBitwiseNot, value)
		case syntax.Plus:
			return ab.EmitUnOp(ssa.OpPlus, value)
		case syntax.Minus:
			return ab.EmitUnOp(ssa.OpNeg, value)
		default:
			return value
		}
	case *syntax.BinaryArithm:
		left := ab.buildArithm(e.X)
		right := ab.buildArithm(e.Y)
		if utils.IsNil(left) {
			return right
		}
		if utils.IsNil(right) {
			return left
		}
		if assignOp, ok := arithmAssignOp(e.Op); ok {
			if name, ok := arithmeticVariableName(e.X); ok {
				value := right
				if assignOp != ssa.OpNone {
					if old := ab.ReadValue(name); !utils.IsNil(old) {
						if merged := ab.EmitBinOp(assignOp, old, right); !utils.IsNil(merged) {
							value = merged
						}
					}
				}
				if variable := ab.CreateVariable(name); variable != nil {
					ab.AssignVariable(variable, value)
				}
				return value
			}
		}
		if op, ok := arithmBinOp(e.Op); ok {
			return ab.EmitBinOp(op, left, right)
		}
		return left
	default:
		return nil
	}
}

// buildArithmWord resolves a bare arithmetic word. In shell arithmetic a bare
// word is a variable read, not a string literal, so `$((N+1))` must read N.
func (ab *astBuilder) buildArithmWord(word *syntax.Word) ssa.Value {
	if word == nil {
		return nil
	}
	if name, ok := staticWord(word); ok {
		name = strings.TrimSpace(name)
		if name != "" {
			if number, err := strconv.ParseInt(name, 0, 64); err == nil {
				return ab.EmitConstInst(number)
			}
			if isArithmeticIdentifier(name) {
				return ab.readOrCreate(name)
			}
		}
	}
	return ab.buildWord(word)
}

// isArithmeticIdentifier reports whether name is a plain shell variable name.
// Array subscripts and base#number forms keep their word form instead of
// creating a variable with punctuation in its name.
func isArithmeticIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for index, r := range name {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if index == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// buildArithmStep lowers `i++` / `--i` to a versioned read-modify-write so a
// C-style loop updates the variable that feeds the loop Phi.
func (ab *astBuilder) buildArithmStep(step *syntax.UnaryArithm) ssa.Value {
	if step == nil {
		return nil
	}
	name, ok := arithmeticVariableName(step.X)
	if !ok {
		ab.buildArithm(step.X)
		return nil
	}
	old := ab.readOrCreate(name)
	if utils.IsNil(old) {
		return nil
	}
	op := ssa.BinaryOpcode(ssa.OpAdd)
	if step.Op == syntax.Dec {
		op = ssa.OpSub
	}
	value := ab.EmitBinOp(op, old, ab.EmitConstInst(int64(1)))
	if utils.IsNil(value) {
		value = old
	}
	if variable := ab.CreateVariable(name); variable != nil {
		ab.AssignVariable(variable, value)
	}
	return value
}

func arithmeticVariableName(expr syntax.ArithmExpr) (string, bool) {
	word, ok := expr.(*syntax.Word)
	if !ok {
		return "", false
	}
	name, ok := staticWord(word)
	if !ok || strings.TrimSpace(name) == "" {
		return "", false
	}
	return strings.TrimSpace(name), true
}

func arithmAssignOp(op syntax.BinAritOperator) (ssa.BinaryOpcode, bool) {
	switch op {
	case syntax.Assgn:
		return ssa.OpNone, true
	case syntax.AddAssgn:
		return ssa.OpAdd, true
	case syntax.SubAssgn:
		return ssa.OpSub, true
	case syntax.MulAssgn:
		return ssa.OpMul, true
	case syntax.QuoAssgn:
		return ssa.OpDiv, true
	case syntax.RemAssgn:
		return ssa.OpMod, true
	case syntax.AndAssgn:
		return ssa.OpAnd, true
	case syntax.OrAssgn:
		return ssa.OpOr, true
	case syntax.XorAssgn:
		return ssa.OpXor, true
	case syntax.ShlAssgn:
		return ssa.OpShl, true
	case syntax.ShrAssgn:
		return ssa.OpShr, true
	default:
		return "", false
	}
}

func arithmBinOp(op syntax.BinAritOperator) (ssa.BinaryOpcode, bool) {
	switch op {
	case syntax.Add:
		return ssa.OpAdd, true
	case syntax.Sub:
		return ssa.OpSub, true
	case syntax.Mul:
		return ssa.OpMul, true
	case syntax.Quo:
		return ssa.OpDiv, true
	case syntax.Rem:
		return ssa.OpMod, true
	case syntax.Pow:
		return ssa.OpPow, true
	case syntax.Eql:
		return ssa.OpEq, true
	case syntax.Neq:
		return ssa.OpNotEq, true
	case syntax.Gtr:
		return ssa.OpGt, true
	case syntax.Lss:
		return ssa.OpLt, true
	case syntax.Geq:
		return ssa.OpGtEq, true
	case syntax.Leq:
		return ssa.OpLtEq, true
	case syntax.And:
		return ssa.OpAnd, true
	case syntax.Or:
		return ssa.OpOr, true
	case syntax.Xor:
		return ssa.OpXor, true
	case syntax.Shl:
		return ssa.OpShl, true
	case syntax.Shr:
		return ssa.OpShr, true
	case syntax.AndArit:
		return ssa.OpLogicAnd, true
	case syntax.OrArit:
		return ssa.OpLogicOr, true
	default:
		return "", false
	}
}
