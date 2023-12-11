package ssa

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/samber/lo"
)

var symbol map[Value]string
var symbolLock sync.RWMutex

func init() {
	symbol = make(map[Value]string)
}

func setSymbolVariable(v Value, name string) {
	symbolLock.Lock()
	defer symbolLock.Unlock()
	symbol[v] = name
}

func unsetSymbolVariable(v Value) {
	symbolLock.Lock()
	defer symbolLock.Unlock()
	delete(symbol, v)
}

func readSymbolVariable(v Value) string {
	symbolLock.RLock()
	defer symbolLock.RUnlock()
	if name, ok := symbol[v]; ok {
		return name
	} else {
		return ""
	}
}

func lineDisasm(v Value) string {
	if id := readSymbolVariable(v); id != "" {
		return id
	} else {
		return v.LineDisasm()
	}
}

func lineDisasms(vs Values) string {
	return strings.Join(
		lo.Map(vs, func(a Value, _ int) string {
			return lineDisasm(a)
		}),
		",",
	)
}

func (f *Function) LineDisasm() string {
	return fmt.Sprintf("%s", f.GetName())
	// return fmt.Sprintf("fun(%s)", f.GetVariable())
}

func (b *BasicBlock) LineDisasm() string {
	// return fmt.Sprintf("block(%s)", b.GetVariable())
	return fmt.Sprintf("%s", b.GetName())
}

func (p *Parameter) LineDisasm() string {
	// return fmt.Sprintf("param(%s)", p.GetVariable())
	return fmt.Sprintf("%s", p.GetName())
}

func (p *Phi) LineDisasm() string {
	setSymbolVariable(p, p.GetName())
	ret := fmt.Sprintf("phi(%s)[%s]", p.GetName(), lineDisasms(p.Edge))
	unsetSymbolVariable(p)
	return ret
	// ret := p.GetVariable()
	// ret := ""

	// return ret
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
	return fmt.Sprintf("%s", u.GetName())
}

func (b *BinOp) LineDisasm() string {
	return fmt.Sprintf("%s(%s, %s)", BinaryOpcodeName[b.Op], lineDisasm(b.X), lineDisasm(b.Y))
}

func (u *UnOp) LineDisasm() string {
	return fmt.Sprintf("%s(%s)", UnaryOpcodeName[u.Op], lineDisasm(u.X))
}

func (c *Call) LineDisasm() string {
	arg := ""
	if len(c.Args) != 0 {
		arg = lineDisasms(c.Args)
	}
	binding := ""
	if len(c.binding) != 0 {
		binding = ", binding(" + lineDisasms(c.binding) + ")"
	}

	return fmt.Sprintf("%s(%s%s)",
		lineDisasm(c.Method),
		arg, binding,
	)
}

func (s *SideEffect) LineDisasm() string {
	return fmt.Sprintf("side-effect(%s, %s)", lineDisasm(s.target), s.GetName())
}

func (m *Make) LineDisasm() string {
	return fmt.Sprintf("make(%s)", m.GetType())
}

func (f *Field) LineDisasm() string {
	return fmt.Sprintf("%s.%s", lineDisasm(f.Obj), lineDisasm(f.Key))
}

func (u *Update) LineDisasm() string {
	return fmt.Sprintf("update(%s, %s)", lineDisasm(u.Address), lineDisasm(u.Value))
}

func (u *Next) LineDisasm() string {
	return fmt.Sprintf("next(%s)", lineDisasm(u.Iter))
}

func (t *TypeCast) LineDisasm() string {
	return fmt.Sprintf("castType(%s, %s)", t.GetType().String(), lineDisasm(t.Value))
}

func (t *TypeValue) LineDisasm() string {
	return fmt.Sprintf("typeValue(%s)", t.GetType())
}

func (r *Recover) LineDisasm() string {
	return "recover"
}

func (r *Return) LineDisasm() string {
	return fmt.Sprintf("return(%s)", lineDisasms(r.Results))
}

func (a *Assert) LineDisasm() string {
	return fmt.Sprintf("assert(%s, %s)", lineDisasm(a.Cond), lineDisasm(a.MsgValue))
}

func (p *Panic) LineDisasm() string {
	return fmt.Sprintf("panic(%s)", lineDisasm(p.Info))
}

func (p *Jump) LineDisasm() string         { return "" }
func (p *ErrorHandler) LineDisasm() string { return "" }

func (i *If) LineDisasm() string {
	return fmt.Sprintf("if (%s) {%s} else {%s}", lineDisasm(i.Cond), lineDisasm(i.True), lineDisasm(i.False))
}
func (l *Loop) LineDisasm() string {
	return fmt.Sprintf("loop(%s)", lineDisasm(l.Cond))
}

func (s Switch) LineDisasm() string {
	return fmt.Sprintf(
		"switch(%s) {case:%s}",
		lineDisasm(s.Cond),
		strings.Join(
			lo.Map(s.Label, func(label SwitchLabel, _ int) string {
				return fmt.Sprintf("%s: %s", lineDisasm(label.Value), lineDisasm(label.Dest))
			}),
			",",
		),
	)
}
