package ssa

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func (f *Function) SetReg(i Instruction) {
	if _, ok := f.InstReg[i]; !ok {
		reg := fmt.Sprintf("t%d", len(f.InstReg))
		f.InstReg[i] = reg
	}
}

func GetTypeStr(n Value) string {
	return fmt.Sprintf(
		"<%s> ", n.GetType(),
	)
}

func (p *Position) String() string {
	return fmt.Sprintf(
		"%3d:%-3d - %3d:%-3d: %s",
		p.StartLine, p.StartColumn,
		p.EndLine, p.EndColumn,
		p.SourceCode,
	)
}

func getStr(v Value) string {
	return getStrFlag(v, true)
}
func getStrFlag(v Value, hasType bool) (op string) {
	if utils.IsNil(v) {
		return "<nil>"
	}
	if hasType {
		op += GetTypeStr(v)
	}
	switch v := v.(type) {
	// case Instruction:
	case *Make:
		// if m.Func.symbol == m {
		// 	return m.Func.Name + "-symbol"
		// }
	case *ConstInst:
		op += v.Const.String()
		return
	case *Parameter:
		op += v.String()
		return
	case *Function:
		op += v.GetVariable()
		return
	case *Field:
		if v.OutCapture {
			return getStr(v.Key) + "(modify)"
		}
	}

	if f := v.GetFunc(); f != nil {
		if str, ok := f.InstReg[v]; ok {
			op += str
		}
	}
	return op
}

// function

type FunctionAsmFlag int

const (
	DisAsmDefault FunctionAsmFlag = 1 << iota
	DisAsmWithSource
)

// implement value
func (f *Function) String() string {
	return f.DisAsm(DisAsmDefault)
}
func (f *Function) DisAsm(flag FunctionAsmFlag) string {
	ret := f.GetVariable() + " "
	ret += strings.Join(
		lo.Map(f.Param, func(item *Parameter, _ int) string { return GetTypeStr(item) + item.variable }),
		", ")
	ret += "\n"

	if parent := f.parent; parent != nil {
		ret += fmt.Sprintf("parent: %s\n", parent.GetVariable())
	}

	if f.Pos != nil {
		ret += fmt.Sprintf("pos: %s\n", f.Pos)
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.Map(f.FreeValues, func(key Value, _ int) string {
				return getStr(key)
			}),
			// f.FreeValue,
			", ") + "\n"
	}
	if len(f.SideEffects) > 0 {
		ret += "sideEffects: " + strings.Join(
			lo.MapToSlice(f.SideEffects, func(name string, v Value) string { return name }),
			",",
		) + "\n"
	}
	if f.GetType() != nil {
		ret += "type: " + f.GetType().String() + "\n"
	}

	for _, b := range f.Blocks {
		if flag&DisAsmWithSource == 0 {
			ret += b.String() + "\n"
			for _, p := range b.Phis {
				ret += fmt.Sprintf("\t%s\n", p)
			}
			for _, i := range b.Insts {
				ret += fmt.Sprintf("\t%s\n", i)
			}
		} else {
			ret += b.String()
			if bPos := b.GetPosition(); bPos != nil {
				ret += bPos.String()
			}
			ret += "\n"
			insts := make([]string, 0, len(b.Insts)+len(b.Phis))
			pos := make([]string, 0, len(b.Insts)+len(b.Phis))
			for _, p := range b.Phis {
				insts = append(insts, fmt.Sprintf("\t%s", p))
				position := p.GetPosition()
				if position == nil {
					pos = append(pos, "")
				} else {
					pos = append(pos, position.String())
				}
			}
			for _, i := range b.Insts {
				insts = append(insts, fmt.Sprintf("\t%s", i))
				if p := i.GetPosition(); p != nil {
					pos = append(pos, i.GetPosition().String())
				} else {
					pos = append(pos, "")
				}
			}
			// get MaxLen
			max := 0
			for _, s := range insts {
				if len(s) > max {
					max = len(s)
				}
			}
			format := fmt.Sprintf("\t%%-%ds\t\t%%s\n", max)
			for i := range insts {
				ret += fmt.Sprintf(format, insts[i], pos[i])
			}
		}
	}
	return ret
}

// ----------- basic block
func (b *BasicBlock) String() string {
	ret := b.GetVariable() + ":"
	if len(b.Preds) != 0 {
		ret += " <- "
		for _, pred := range b.Preds {
			ret += pred.GetVariable() + " "
		}
	}
	if !utils.IsNil(b.Condition) {
		ret += " (" + getStrFlag(b.Condition, false) + ")"
	}
	return ret
}

// ----------- const
func (c Const) String() string {
	return c.str
}

// ----------- const instruction
func (c *ConstInst) String() string {
	return c.Const.String()
}

// ----------- undefined
func (u *Undefined) String() string {
	return fmt.Sprintf("%s = undefined-%s", getStr(u), u.GetVariable())
}

// ----------- Phi
func (p *Phi) String() string {
	ret := fmt.Sprintf("%s = phi ", getStr(p))
	for i := range p.Edge {
		v := p.Edge[i]
		b := p.GetBlock().Preds[i]
		if utils.IsNil(v) {
			continue
		}
		ret += fmt.Sprintf("[%s, %s] ", getStr(v), b.GetVariable())
	}
	return ret
}

// ----------- Parameter
func (p *Parameter) String() string {
	return p.variable
}

// ----------- Jump
func (j *Jump) String() string {
	return fmt.Sprintf("jump -> %s", j.To.GetVariable())
}

// ----------- IF
func (i *If) String() string {
	// return i.StringByFunc(DefaultValueString)
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.Cond), i.True.GetVariable(), i.False.GetVariable())
}

// ----------- Loop
func (l *Loop) String() string {
	return fmt.Sprintf("Loop [%s; %s; %s] body -> %s, exit -> %s", getStr(l.Init), getStr(l.Cond), getStr(l.Step), l.Body.GetVariable(), l.Exit.GetVariable())
}

// ----------- Return
func (r *Return) String() string {
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
	)
}

// ----------- Call
func (c *Call) String() string {
	methodStr := getStr(c.Method)
	argStr := strings.Join(
		lo.Map(c.Args, func(v Value, _ int) string { return getStr(v) }),
		", ",
	)
	binding := strings.Join(
		lo.Map(c.binding, func(v Value, _ int) string {
			return getStr(v)
		}),
		", ",
	)
	drop := ""
	if c.IsDropError {
		drop = "~"
	}

	if c.Async {
		return fmt.Sprintf(
			"go %s (%s) [%s]",
			methodStr, argStr, binding,
		)
	} else {
		return fmt.Sprintf(
			"%s = call %s (%s)%s [%s]",
			getStr(c),
			methodStr, argStr, drop, binding,
		)
	}
}

func (s *SideEffect) String() string {
	return fmt.Sprintf("%s = side-effect %s [%s]", getStr(s), getStr(s.target), s.GetVariable())
}

// ----------- Switch
func (sw *Switch) String() string {
	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		getStr(sw.Cond),
		sw.DefaultBlock.GetVariable(),
		strings.Join(
			lo.Map(sw.Label, func(label SwitchLabel, _ int) string {
				return fmt.Sprintf("%s:%s", getStr(label.Value), label.Dest.GetVariable())
			}),
			", ",
		),
	)
}

// ----------- BinOp
func (b *BinOp) String() string {
	return fmt.Sprintf("%s = %s %s %s", getStr(b), getStr(b.X), BinaryOpcodeName[b.Op], getStr(b.Y))
}

// ----------- UnOp
func (u *UnOp) String() string {
	return fmt.Sprintf("%s = %s %s", getStr(u), UnaryOpcodeName[u.Op], getStr(u.X))
}

// ----------- Interface
func (i *Make) String() string {
	if i.parentI != nil {
		return fmt.Sprintf(
			"%s = %s [%s:%s:%s]",
			getStr(i), getStr(i.parentI), getStr(i.low), getStr(i.high), getStr(i.step),
		)
	} else {
		return fmt.Sprintf(
			"%s = make %s [%s, %s]",
			getStr(i), i.GetType(), getStr(i.Len), getStr(i.Cap),
		)
	}
}

// ----------- Field
func (f *Field) String() string {
	return fmt.Sprintf(
		"%s = %s field[%s]",
		getStr(f), getStr(f.Obj), getStr(f.Key),
	)
}

// ----------- Update
func (s *Update) String() string {
	return fmt.Sprintf(
		"update [%s] = %s",
		getStr(s.Address), getStr(s.Value),
	)
}

func (t *TypeCast) String() string {
	return fmt.Sprintf(
		"%s = type-case[%s] %s",
		getStr(t), t.GetType(), getStr(t.Value),
	)
}

func (t *TypeValue) String() string {
	return fmt.Sprintf(
		"%s = type-value[%s]",
		getStr(t), t.GetType(),
	)
}

func (a *Assert) String() string {
	msg := a.Msg
	if a.MsgValue != nil {
		msg = getStr(a.MsgValue)
	}

	return fmt.Sprintf(
		"assert[%s] %s",
		getStr(a.Cond), msg,
	)
}

func (n *Next) String() string {
	return fmt.Sprintf(
		"%s = next[%s]",
		getStr(n), getStr(n.Iter),
	)
}

func (e *ErrorHandler) String() string {
	finalName := "nil"
	if e.final != nil {
		finalName = e.final.GetVariable()
	}
	return fmt.Sprintf(
		"try %s; catch %s; final %s; rest %s",
		e.try.GetVariable(), e.catch.GetVariable(), finalName, e.done.GetVariable(),
	)
}

func (p *Panic) String() string {
	return fmt.Sprintf(
		"panic %s",
		getStr(p.Info),
	)
}

func (r *Recover) String() string {
	return getStr(r) + " = recover"
}
