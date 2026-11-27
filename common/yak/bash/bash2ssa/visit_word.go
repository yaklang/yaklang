package bash2ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// staticWord returns the literal text of a word when no expansion is present.
// Single and double quotes are both static here; the caller only uses this for
// command identity and wrapper/eval payload extraction.
func staticWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	return staticParts(word.Parts, false)
}

// rawStaticWord returns a static word's source text without decoding quotes or
// escapes. Command identity keeps this form so ANSI-C obfuscation such as
// $'\x63\x75\x72\x6c' stays visible to static rules, matching bash's own
// word text before expansion.
func rawStaticWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	return rawParts(word.Parts)
}

func rawParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			inner, ok := rawParts(p.Parts)
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

func literalText(raw string, double bool) string {
	if !strings.Contains(raw, `\`) {
		return raw
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) && (!double || strings.ContainsRune("\\\"$`\n", rune(raw[i+1]))) {
			i++
			if raw[i] == '\n' {
				continue
			}
		}
		b.WriteByte(raw[i])
	}
	return b.String()
}
func quotedText(q *syntax.SglQuoted) string {
	if q.Dollar {
		s, _, _ := expand.Format(nil, q.Value, nil)
		return s
	}
	return q.Value
}
func staticParts(parts []syntax.WordPart, double bool) (string, bool) {
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(literalText(p.Value, double))
		case *syntax.SglQuoted:
			b.WriteString(quotedText(p))
		case *syntax.DblQuoted:
			inner, ok := staticParts(p.Parts, true)
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

	return ab.buildParts(word.Parts, false, false)
}

// buildArgWord builds a command argument or for-list word. Unquoted parameter
// and command expansions are wrapped in the split intrinsics here, including
// when they are concatenated with literals (--prefix=$dir, bar$(cmd)bar).
func (ab *astBuilder) buildArgWord(word *syntax.Word) ssa.Value {
	if word == nil {
		return nil
	}
	if len(word.Parts) == 0 {
		return ab.EmitConstInst("")
	}
	defer ab.setRange(word)()

	return ab.buildParts(word.Parts, false, true)
}

func (ab *astBuilder) buildParts(parts []syntax.WordPart, double, splitFields bool) ssa.Value {
	if text, ok := staticParts(parts, double); ok {
		return ab.EmitConstInst(text)
	}
	var out ssa.Value
	for _, part := range parts {
		value := ab.buildWordPart(part, double)
		if utils.IsNil(value) {
			continue
		}
		if splitFields && !double {
			value = ab.markUnquotedSplit(part, value)
		}
		if utils.IsNil(out) {
			out = value
			continue
		}
		out = ab.concat(out, value)
	}
	return out
}

// markUnquotedSplit tags a word part that undergoes shell word splitting. The
// intrinsic keeps the value as the first argument, so dataflow is unchanged,
// and rules can match the callee to tell it apart from a quoted expansion.
func (ab *astBuilder) markUnquotedSplit(part syntax.WordPart, value ssa.Value) ssa.Value {
	switch p := part.(type) {
	case *syntax.ParamExp:
		if !paramExpSplits(p) {
			return value
		}
		return ab.intrinsic("param-fields", value, ab.ifsValue())
	case *syntax.CmdSubst:
		return ab.intrinsic("cmd-fields", value, ab.ifsValue())
	default:
		return value
	}
}

// paramExpSplits reports whether an unquoted parameter expansion can produce
// more than one word. Special one-word parameters and ${#x} lengths cannot.
func paramExpSplits(exp *syntax.ParamExp) bool {
	if exp == nil || exp.Length {
		return false
	}
	if exp.Param == nil {
		return true
	}
	switch exp.Param.Value {
	case "?", "$", "!", "-", "#":
		return false
	default:
		return true
	}
}

func (ab *astBuilder) ifsValue() ssa.Value {
	if ifs := ab.PeekValue("IFS"); ifs != nil {
		return ifs
	}
	return ab.EmitConstInst(" \t\n")
}

func (ab *astBuilder) buildWordPart(part syntax.WordPart, double bool) ssa.Value {
	switch p := part.(type) {
	case *syntax.Lit:
		return ab.EmitConstInst(literalText(p.Value, double))
	case *syntax.SglQuoted:
		return ab.EmitConstInst(quotedText(p))
	case *syntax.DblQuoted:
		return ab.buildParts(p.Parts, true, false)
	case *syntax.ParamExp:
		return ab.buildParamExp(p)
	case *syntax.CmdSubst:
		return ab.buildCmdSubst(p)
	case *syntax.ArithmExp:
		return ab.text(ab.buildArithm(p.X))
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
	var value ssa.Value
	if positionalName(name) {
		value = ab.positional(name)
	} else {
		switch name {
		case "?", "$", "!", "-":
			name = "$" + name
		}
		value = ab.readOrCreate(name)
	}
	if exp.Index != nil {
		if w, ok := exp.Index.(*syntax.Word); ok && (w.Lit() == "@" || w.Lit() == "*") {
			if exp.Length {
				if _, concrete := ssa.ToMake(value); concrete {
					return ab.EmitConstInst(len(ssa.GetLastWinsMemberPairs(value)))
				}
				return ab.intrinsic("length", value)
			}
			if w.Lit() == "*" {
				var vs []ssa.Value
				for _, p := range ssa.GetLastWinsMemberPairs(value) {
					vs = append(vs, p.Member)
				}
				return ab.join(vs, " ")
			}
		} else {
			value = ab.ReadMemberCallValue(value, ab.arrayKey(name, exp.Index))
		}
	} else if !positionalName(name) {
		if _, ok := ssa.ToMake(value); ok {
			value = ab.ReadMemberCallValue(value, ab.EmitConstInst(0))
		}
	}
	if exp.Length {
		if c, ok := ssa.ToConstInst(value); ok {
			return ab.EmitConstInst(len([]rune(c.String())))
		}
		return ab.intrinsic("length", value)
	}
	if exp.Exp != nil {
		value = ab.parameterOperator(name, value, exp.Exp)
	}
	if exp.Slice != nil {
		// TODO(ssa-bash): slicing and replacement expansions evaluate their operands but return the whole value.
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

	out, status := ab.childShell(func() ssa.Value { ab.pendingStdin = nil; return ab.buildStmts(subst.Stmts) })
	ab.lastSubst = status
	if out == nil {
		out = ab.EmitConstInst("")
	}
	return out
}

// TODO(ssa-bash): process substitution returns the child stdout instead of a file-descriptor path.
func (ab *astBuilder) buildProcSubst(subst *syntax.ProcSubst) ssa.Value {
	if subst == nil {
		return nil
	}
	defer ab.setRange(subst)()
	out, _ := ab.childShell(func() ssa.Value { ab.pendingStdin = nil; return ab.buildStmts(subst.Stmts) })
	if out == nil {
		out = ab.EmitUndefined("<()")
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
		if e.Op == syntax.Assgn {
			if name, ok := arithmeticVariableName(e.X); ok {
				v := ab.buildArithm(e.Y)
				ab.put(name, v)
				return v
			}
		}
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
						if merged := ab.EmitBinOp(assignOp, ab.number(old), right); !utils.IsNil(merged) {
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
		// TODO(ssa-bash): ternary and other arithmetic forms fall back to nil.
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
			if syntax.ValidName(name) {
				return ab.number(ab.readOrCreate(name))
			}
		}
	}
	return ab.number(ab.buildWord(word))
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
	old := ab.number(ab.readOrCreate(name))
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
	ab.put(name, value)
	if step.Post {
		return old
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
