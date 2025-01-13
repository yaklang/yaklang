package ssa

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func GetTypeStr(n Value) string {
	return fmt.Sprintf(
		"<%s> ", n.GetType(),
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
		op += v.GetName()
		return
	}

	// if f := v.GetFunc(); f != nil {
	if id := v.GetId(); id != -1 {
		op += fmt.Sprintf("t%d", v.GetId())
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
	ret := f.GetName() + " "
	ret += strings.Join(
		lo.Map(f.Params, func(id int64, _ int) string {
			item := f.GetValueById(id)
			return fmt.Sprintf("%s(%d) %s", GetTypeStr(item), item.GetId(), item.GetName())
		}),
		", ")
	ret += "\n"

	if id := f.parent; id > 0 {
		parent := f.GetValueById(id)
		ret += fmt.Sprintf("parent: %s\n", parent.GetName())
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.MapToSlice(f.FreeValues, func(name *Variable, id int64) string {
				item := f.GetValueById(id)
				if utils.IsNil(item) {
					log.Infof("bb")
				}
				item = f.GetValueById(id)
				return fmt.Sprintf("%s:(%d)%s", name.GetName(), item.GetId(), item.GetName())
			}),
			// f.FreeValue,
			", ") + "\n"
	}
	if len(f.ParameterMembers) > 0 {
		ret += "parameterMember: " + strings.Join(
			lo.Map(f.ParameterMembers, func(id int64, _ int) string {
				item := f.GetValueById(id)
				return fmt.Sprintf("%s(%d) %s", GetTypeStr(item), item.GetId(), item.GetName())
			}),
			", ") + "\n"
	}
	if len(f.SideEffects) > 0 {
		ret += "sideEffects: " + strings.Join(
			lo.Map(f.SideEffects, func(se *FunctionSideEffect, _ int) string {
				switch se.MemberCallKind {
				case ParameterMemberCall:
					return fmt.Sprintf("parameter[%d].%s", se.MemberCallObjectIndex, f.GetValueById(se.MemberCallKey))
				case FreeValueMemberCall:
					return fmt.Sprintf("freeValue[%s].%s", se.MemberCallObjectName, f.GetValueById(se.MemberCallKey))
				}
				return se.Name
			}),
			",",
		) + "\n"
	}
	if f.GetType() != nil {
		ret += "type: " + f.GetType().String() + "\n"
	}

	ShowBlock := func(b *BasicBlock) {
		if utils.IsNil(b) {
			return
		}

		if flag&DisAsmWithSource == 0 {
			ret += b.String() + "\n"
			for _, p := range b.Phis {
				ret += fmt.Sprintf("\t%s\n", f.GetValueById(p))
			}
			for _, id := range b.Insts {
				i := b.GetInstructionById(id)
				if _, ok := ToConstInst(i); ok {
					continue
				}
				ret += fmt.Sprintf("\t%s\n", i)
			}
		} else {
			ret += b.String()
			if bPos := b.GetRange(); bPos != nil {
				ret += bPos.String()
			}
			ret += "\n"
			insts := make([]string, 0, len(b.Insts)+len(b.Phis))
			pos := make([]string, 0, len(b.Insts)+len(b.Phis))
			for _, id := range b.Phis {
				p := b.GetValueById(id)
				insts = append(insts, fmt.Sprintf("\t%s", p))
				r := p.GetRange()
				if r == nil {
					pos = append(pos, "")
				} else {
					pos = append(pos, r.String())
				}
			}
			for _, id := range b.Insts {
				i := b.GetInstructionById(id)
				insts = append(insts, fmt.Sprintf("\t%s", i))
				if p := i.GetRange(); p != nil {
					pos = append(pos, i.GetRange().String())
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

	for _, b := range f.Blocks {
		block := f.GetBasicBlockByID(b)
		_ = block
		if utils.IsNil(block) {
			log.Infof("bb")
		}
		ShowBlock(f.GetBasicBlockByID(b))
	}

	return ret
}

// ----------- basic block
func (b *BasicBlock) String() string {
	ret := b.GetName() + ":"
	if len(b.Preds) != 0 {
		ret += " <- "
		for _, id := range b.Preds {
			pred := b.GetBasicBlockByID(id)
			ret += pred.GetName() + " "
		}
	}
	if !utils.IsNil(b.Condition) {
		ret += " (" + getStrFlag(b.GetValueById(b.Condition), false) + ")"
	}
	return ret
}

// ----------- const
func (c Const) String() string {
	return c.str
}

// ----------- const instruction
func (c *ConstInst) String() string {
	if c == nil || utils.IsNil(c) {
		return ""
	}
	if c.Const == nil {
		return ""
	}
	return c.Const.String()
}

// ----------- undefined
func (u *Undefined) String() string {
	valid := ""
	if u.Kind == UndefinedMemberValid {
		valid = "(valid)"
	}
	if u.IsMember() {
		return fmt.Sprintf("%s = undefined-%s%s(from:%d)", getStr(u), u.GetVerboseName(), valid, u.GetObject().GetId())
	}
	return fmt.Sprintf("%s = undefined-%s", getStr(u), u.GetName())
}

// ----------- Phi
func (p *Phi) String() string {
	ret := fmt.Sprintf("%s = phi ", getStr(p))
	for _, v := range p.GetValues() {
		if utils.IsNil(v) {
			continue
		}
		b := v.GetBlock()
		ret += fmt.Sprintf("[%s, %s] ", getStr(v), b.GetVerboseName())
	}
	return ret
}

// ----------- Parameter
func (p *ParameterMember) String() string {
	switch p.MemberCallKind {
	case NoMemberCall:
		return "normal-member-call"
	case ParameterMemberCall:
		return fmt.Sprintf("parameter[%d].%s", p.MemberCallObjectIndex, p.GetValueById(p.MemberCallKey))
	case FreeValueMemberCall:
		return fmt.Sprintf("freeValue-%s.%s", p.MemberCallObjectName, p.GetValueById(p.MemberCallKey))
	case MoreParameterMember:
		return fmt.Sprintf("parameterMember[%d].%s", p.MemberCallObjectIndex, p.GetValueById(p.MemberCallKey))
	}
	return ""
}
func (p *Parameter) String() string {
	return p.GetName()
}

func (e *ExternLib) String() string {
	return e.GetName()
}

// ----------- Jump
func (j *Jump) String() string {
	return fmt.Sprintf("jump -> %v", j.GetValueById(j.To).GetName())
}

// ----------- IF
func (i *If) String() string {
	// return i.StringByFunc(DefaultValueString)
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.GetValueById(i.Cond)), i.GetValueById(i.True).GetName(), i.GetValueById(i.False).GetName())
}

// ----------- Loop
func (l *Loop) String() string {
	return fmt.Sprintf("Loop [%s; %s; %s] body -> %s, exit -> %s", getStr(l.GetValueById(l.Init)), getStr(l.GetValueById(l.Cond)), getStr(l.GetValueById(l.Step)), l.GetValueById(l.Body).GetName(), l.GetValueById(l.Exit).GetName())
}

// ----------- Return
func (r *Return) String() string {
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v int64, _ int) string { return getStr(r.GetValueById(v)) }),
			", ",
		),
	)
}

// ----------- Call
func (c *Call) String() string {
	methodStr := getStr(c.GetValueById(c.Method))
	argStr := strings.Join(
		lo.Map(c.Args, func(id int64, index int) string {
			// return fmt.Sprintf("%d: %s", index, getStr(v))
			return getStr(c.GetValueById(id))
		}),
		", ",
	)
	binding := "binding[" + strings.Join(
		lo.MapToSlice(c.Binding, func(name string, v int64) string {
			// return fmt.Sprintf("%s: %s", name, getStr(v))
			return getStr(c.GetValueById(v))
		}),
		", ",
	) + "]"
	member := "member[" + strings.Join(
		lo.Map(c.ArgMember, func(v int64, _ int) string {
			return getStr(c.GetValueById(v))
		}),
		", ") + "]"
	drop := ""
	if c.IsDropError {
		drop = "~"
	}

	if c.Async {
		return fmt.Sprintf(
			"go %s (%s) %s %s",
			methodStr, argStr, binding, member,
		)
	} else {
		return fmt.Sprintf(
			"%s = call %s (%s)%s %s %s",
			getStr(c),
			methodStr, argStr, drop, binding, member,
		)
	}
}
func (s *SideEffect) String() string {
	return fmt.Sprintf("%s = side-effect %s [%s] by %s", getStr(s), getStr(s.GetValueById(s.Value)), s.GetVerboseName(), getStr(s.GetValueById(s.CallSite)))
}

// ----------- Switch
func (sw *Switch) String() string {
	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		getStr(sw.GetValueById(sw.Cond)),
		sw.DefaultBlock.GetName(),
		strings.Join(
			lo.Map(sw.Label, func(label SwitchLabel, _ int) string {
				return fmt.Sprintf("%s:%s", getStr(sw.GetValueById(label.Value)), sw.GetValueById(label.Dest).GetName())
			}),
			", ",
		),
	)
}

// ----------- BinOp
func (b *BinOp) String() string {
	return fmt.Sprintf("%s = %s %s %s", getStr(b), getStr(b.GetValueById(b.X)), b.Op, getStr(b.GetValueById(b.Y)))
}

// ----------- UnOp
func (u *UnOp) String() string {
	return fmt.Sprintf("%s = %s %s", getStr(u), u.Op, getStr(u.GetValueById(u.X)))
}

// ----------- Interface
func (i *Make) String() string {
	if i.parentI > 0 {
		return fmt.Sprintf(
			"%s = %s [%s:%s:%s]",
			getStr(i), getStr(i.GetValueById(i.parentI)), getStr(i.GetValueById(i.low)), getStr(i.GetValueById(i.high)), getStr(i.GetValueById(i.step)),
		)
	} else {
		str := fmt.Sprintf(
			"%s = make %s [%s, %s]",
			getStr(i), i.GetType(), getStr(i.GetValueById(i.Len)), getStr(i.GetValueById(i.Cap)),
		)
		if i.name != "" {
			str += "// " + i.name
		}
		return str
	}
}

func (t *TypeCast) String() string {
	return fmt.Sprintf(
		"%s = type-case[%s] %s",
		getStr(t), t.GetType(), getStr(t.GetValueById(t.Value)),
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
	if a.MsgValue > 0 {
		msg = getStr(a.GetValueById(a.MsgValue))
	}

	return fmt.Sprintf(
		"assert[%s] %s",
		getStr(a.GetValueById(a.Cond)), msg,
	)
}

func (n *Next) String() string {
	return fmt.Sprintf(
		"%s = next[%s]",
		getStr(n), getStr(n.GetValueById(n.Iter)),
	)
}

func (e *ErrorHandler) String() string {
	finalName := "nil"
	if e.Final > 0 {
		finalName = e.GetValueById(e.Final).GetName()
	}
	return fmt.Sprintf(
		"try %s; catch %s; final %s; rest %s",
		e.GetValueById(e.Try).GetName(),
		"",
		finalName,
		e.GetValueById(e.Done).GetName(),
	)
}

func (e *ErrorCatch) String() string {
	return fmt.Sprintf(
		"catch %s; body %s; exception %s",
		e.GetName(), getStr(e.GetValueById(e.CatchBody)), getStr(e.GetValueById(e.Exception)),
	)
}

func (p *Panic) String() string {
	return fmt.Sprintf(
		"panic %s",
		getStr(p.GetValueById(p.Info)),
	)
}

func (r *Recover) String() string {
	return getStr(r) + " = recover"
}
