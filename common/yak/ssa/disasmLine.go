package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

func lineDisasm(vs Values) string {
	return strings.Join(
		lo.Map(vs, func(a Value, _ int) string {
			return a.LineDisasm()
		}),
		",",
	)
}
func (f *Function) LineDisasm() string {
	return fmt.Sprintf("%s", f.GetVariable())
	// return fmt.Sprintf("fun(%s)", f.GetVariable())
}

func (b *BasicBlock) LineDisasm() string {
	// return fmt.Sprintf("block(%s)", b.GetVariable())
	return fmt.Sprintf("%s", b.GetVariable())
}

func (p *Parameter) LineDisasm() string {
	// return fmt.Sprintf("param(%s)", p.GetVariable())
	return fmt.Sprintf("%s", p.GetVariable())
}

func (p *Phi) LineDisasm() string {
	return fmt.Sprintf("phi(%s)", lineDisasm(p.Edge))
}

func (c *ConstInst) LineDisasm() string {
	// return fmt.Sprintf("const(%s)", c.String())
	if !c.isIdentify && reflect.TypeOf(c.Const.value).Kind() == reflect.String {
		return fmt.Sprintf("\"%s\"", c.String())
	} else {
		return fmt.Sprintf("%s", c.String())
	}
}

func (u *Undefined) LineDisasm() string {
	// return fmt.Sprintf("undefined(%s)", u.GetVariable())
	return fmt.Sprintf("%s", u.GetVariable())
}

func (b *BinOp) LineDisasm() string {
	return fmt.Sprintf("%s(%s, %s)", BinaryOpcodeName[b.Op], b.X.LineDisasm(), b.Y.LineDisasm())
	// return fmt.Sprintf("%s %s %s)", b.X.LineDisasm(), BinaryOpcodeName[b.Op], b.Y.LineDisasm())
}

func (u *UnOp) LineDisasm() string {
	return fmt.Sprintf("%s(%s)", UnaryOpcodeName[u.Op], u.X.LineDisasm())
	// return fmt.Sprintf("%s%s", UnaryOpcodeName[u.Op], u.X.LineDisasm())
}

func (c *Call) LineDisasm() string {
	arg := ""
	if len(c.Args) != 0 {
		arg = lineDisasm(c.Args)
	}
	binding := ""
	if len(c.binding) != 0 {
		binding = ", binding(" + lineDisasm(c.binding) + ")"
	}

	return fmt.Sprintf("%s(%s%s)",
		c.Method.LineDisasm(),
		arg, binding,
	)
}

func (s *SideEffect) LineDisasm() string {
	return fmt.Sprintf("side-effect(%s, %s)", s.target.LineDisasm(), s.GetVariable())
}

func (m *Make) LineDisasm() string {
	return fmt.Sprintf("make(%s)", m.GetType())
}

func (f *Field) LineDisasm() string {
	// return fmt.Sprintf("field(%s, %s)", f.Obj.LineDisasm(), f.Key.LineDisasm())
	return fmt.Sprintf("%s.%s", f.Obj.LineDisasm(), f.Key.LineDisasm())
	// return fmt.Sprintf("%s[%s]", f.Obj.LineDisasm(), f.Key.LineDisasm())
}

func (u *Update) LineDisasm() string {
	return fmt.Sprintf("update(%s, %s)", u.Address.LineDisasm(), u.Value.LineDisasm())
}

func (u *Next) LineDisasm() string {
	return fmt.Sprintf("next(%s)", u.Iter.LineDisasm())
}

func (t *TypeCast) LineDisasm() string {
	return fmt.Sprintf("castType(%s, %s)", t.GetType().String(), t.Value.LineDisasm())
}

func (t *TypeValue) LineDisasm() string {
	return fmt.Sprintf("typeValue(%s)", t.GetType())
}

func (r *Recover) LineDisasm() string {
	return "recover"
}

func (r *Return) LineDisasm() string {
	return fmt.Sprintf("return(%s)", lineDisasm(r.Results))
}

func (a *Assert) LineDisasm() string {
	return fmt.Sprintf("assert(%s, %s)", a.Cond.LineDisasm(), a.MsgValue.LineDisasm())
}

func (p *Panic) LineDisasm() string {
	return fmt.Sprintf("panic(%s)", p.Info.LineDisasm())
}

func (p *Jump) LineDisasm() string         { return "" }
func (p *ErrorHandler) LineDisasm() string { return "" }

func (i *If) LineDisasm() string {
	return fmt.Sprintf("if (%s) {%s} else {%s}", i.Cond.LineDisasm(), i.True.LineDisasm(), i.False.LineDisasm())
}
func (l *Loop) LineDisasm() string {
	return fmt.Sprintf("loop(%s)", l.Cond.LineDisasm())
}

func (s Switch) LineDisasm() string {
	return fmt.Sprintf(
		"switch(%s) {case:%s}",
		s.Cond.LineDisasm(),
		strings.Join(
			lo.Map(s.Label, func(label SwitchLabel, _ int) string {
				return fmt.Sprintf("%s: %s", label.Value.LineDisasm(), label.Dest.LineDisasm())
			}),
			",",
		),
	)
}
