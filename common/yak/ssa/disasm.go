package ssa

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func GetTypeStr(n Value) string {
	if utils.IsNil(n) {
		return "<nil>"
	}
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
	if utils.IsNil(f) {
		return ""
	}
	return f.DisAsm(DisAsmDefault)
}
func (f *Function) DisAsm(flag FunctionAsmFlag) string {
	ret := f.GetName() + " "
	ret += strings.Join(
		lo.Map(f.Params, func(id int64, _ int) string {
			item := f.GetValueById(id)
			if utils.IsNil(item) {
				return "<nil>"
			} else {
				return fmt.Sprintf("%s(%d) %s", GetTypeStr(item), item.GetId(), item.GetName())
			}
		}),
		", ")
	ret += "\n"

	if id := f.parent; id > 0 {
		parent := f.GetValueById(id)
		if !utils.IsNil(parent) {
			ret += fmt.Sprintf("parent: %s\n", parent.GetName())
		} else {
			ret += fmt.Sprintf("parent: <nil:%d>\n", id)
		}
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.MapToSlice(f.FreeValues, func(name *Variable, id int64) string {
				item := f.GetValueById(id)
				if utils.IsNil(item) {
					return fmt.Sprintf("%s:(%d)<nil>", name.GetName(), id)
				}
				return fmt.Sprintf("%s:(%d)%s", name.GetName(), item.GetId(), item.GetName())
			}),
			// f.FreeValue,
			", ") + "\n"
	}
	if len(f.ParameterMembers) > 0 {
		ret += "parameterMember: " + strings.Join(
			lo.Map(f.ParameterMembers, func(id int64, _ int) string {
				item := f.GetValueById(id)
				if utils.IsNil(item) {
					return fmt.Sprintf("<nil>(%d)", id)
				}
				return fmt.Sprintf("%s(%d) %s", GetTypeStr(item), item.GetId(), item.GetName())
			}),
			", ") + "\n"
	}
	if len(f.SideEffects) > 0 {
		ret += "sideEffects: " + strings.Join(
			lo.Map(f.SideEffects, func(se *FunctionSideEffect, _ int) string {
				switch se.MemberCallKind {
				case ParameterMemberCall:
					key := f.GetValueById(se.MemberCallKey)
					if utils.IsNil(key) {
						return fmt.Sprintf("parameter[%d].<nil key:%d>", se.MemberCallObjectIndex, se.MemberCallKey)
					}
					return fmt.Sprintf("parameter[%d].%s", se.MemberCallObjectIndex, key)
				case FreeValueMemberCall:
					key := f.GetValueById(se.MemberCallKey)
					if utils.IsNil(key) {
						return fmt.Sprintf("freeValue[%s].<nil key:%d>", se.MemberCallObjectName, se.MemberCallKey)
					}
					return fmt.Sprintf("freeValue[%s].%s", se.MemberCallObjectName, key)
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
				phi := f.GetValueById(p)
				if utils.IsNil(phi) {
					ret += fmt.Sprintf("\t<nil phi:%d>\n", p)
				} else {
					ret += fmt.Sprintf("\t%s\n", phi)
				}
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
			log.Errorf("function %s has nil block: %d", f.GetName(), b)
			continue
		}
		ShowBlock(f.GetBasicBlockByID(b))
	}

	return ret
}

// ----------- basic block
func (b *BasicBlock) String() string {
	if utils.IsNil(b) {
		return ""
	}
	ret := b.GetName() + ":"
	if len(b.Preds) != 0 {
		ret += " <- "
		for _, id := range b.Preds {
			pred := b.GetBasicBlockByID(id)
			if utils.IsNil(pred) {
				log.Infof("pred is nil: %v", id)
				continue
			}
			ret += pred.GetName() + " "
		}
	}
	if !utils.IsNil(b.Condition) {
		condition := b.GetValueById(b.Condition)
		if !utils.IsNil(condition) {
			ret += " (" + getStrFlag(condition, false) + ")"
		} else {
			ret += " (<nil condition>)"
		}
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
	if utils.IsNil(u) {
		return ""
	}
	valid := ""
	if u.Kind == UndefinedMemberValid {
		valid = "(valid)"
	}
	if u.IsMember() {
		obj := u.GetObject()
		if utils.IsNil(obj) {
			return fmt.Sprintf("%s = undefined-%s%s(from:<nil>)", getStr(u), u.GetVerboseName(), valid)
		}
		return fmt.Sprintf("%s = undefined-%s%s(from:%d)", getStr(u), u.GetVerboseName(), valid, obj.GetId())
	}
	return fmt.Sprintf("%s = undefined-%s", getStr(u), u.GetName())
}

// ----------- Phi
func (p *Phi) String() string {
	if utils.IsNil(p) {
		return ""
	}
	ret := fmt.Sprintf("%s = phi ", getStr(p))
	for _, v := range p.GetValues() {
		if utils.IsNil(v) {
			continue
		}
		b := v.GetBlock()
		var verboseName string
		if b != nil {
			verboseName = b.GetVerboseName()
		}
		ret += fmt.Sprintf("[%s, %s] ", getStr(v), verboseName)
	}
	return ret
}

// ----------- Parameter
func (p *ParameterMember) String() string {
	if utils.IsNil(p) {
		return ""
	}
	switch p.MemberCallKind {
	case NoMemberCall:
		return "normal-member-call"
	case ParameterMemberCall:
		key := p.GetValueById(p.MemberCallKey)
		if utils.IsNil(key) {
			return fmt.Sprintf("parameter[%d].<nil key:%d>", p.MemberCallObjectIndex, p.MemberCallKey)
		}
		return fmt.Sprintf("parameter[%d].%s", p.MemberCallObjectIndex, key)
	case FreeValueMemberCall:
		key := p.GetValueById(p.MemberCallKey)
		if utils.IsNil(key) {
			return fmt.Sprintf("freeValue-%s.<nil key:%d>", p.MemberCallObjectName, p.MemberCallKey)
		}
		return fmt.Sprintf("freeValue-%s.%s", p.MemberCallObjectName, key)
	case MoreParameterMember:
		key := p.GetValueById(p.MemberCallKey)
		if utils.IsNil(key) {
			return fmt.Sprintf("parameterMember[%d].<nil key:%d>", p.MemberCallObjectIndex, p.MemberCallKey)
		}
		return fmt.Sprintf("parameterMember[%d].%s", p.MemberCallObjectIndex, key)
	}
	return ""
}
func (p *Parameter) String() string {
	if utils.IsNil(p) {
		return ""
	}
	return p.GetName()
}

func (e *ExternLib) String() string {
	if utils.IsNil(e) {
		return ""
	}
	return e.GetName()
}

// ----------- Jump
func (j *Jump) String() string {
	if utils.IsNil(j) {
		return ""
	}
	to := j.GetValueById(j.To)
	if utils.IsNil(to) {
		return fmt.Sprintf("jump -> <nil target:%d>", j.To)
	}
	return fmt.Sprintf("jump -> %v", to.GetName())
}

// ----------- IF
func (i *If) String() string {
	if utils.IsNil(i) {
		return ""
	}
	cond := i.GetValueById(i.Cond)
	if utils.IsNil(cond) {
		return "If [nil condition]"
	}
	trueBranch := i.GetValueById(i.True)
	if utils.IsNil(trueBranch) {
		return fmt.Sprintf("If [%s] true -> nil", getStr(cond))
	}
	falseBranch := i.GetValueById(i.False)
	if utils.IsNil(falseBranch) {
		return fmt.Sprintf("If [%s] true -> %s, false -> nil", getStr(cond), trueBranch.GetName())
	}
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(cond), trueBranch.GetName(), falseBranch.GetName())
}

// ----------- Loop
func (l *Loop) String() string {
	if utils.IsNil(l) {
		return ""
	}

	bodyValue := l.GetValueById(l.Body)
	exitValue := l.GetValueById(l.Exit)

	bodyName := "<nil>"
	if !utils.IsNil(bodyValue) {
		bodyName = bodyValue.GetName()
	}

	exitName := "<nil>"
	if !utils.IsNil(exitValue) {
		exitName = exitValue.GetName()
	}

	return fmt.Sprintf("Loop [%s; %s; %s] body -> %s, exit -> %s",
		getStr(l.GetValueById(l.Init)),
		getStr(l.GetValueById(l.Cond)),
		getStr(l.GetValueById(l.Step)),
		bodyName,
		exitName)
}

// ----------- Return
func (r *Return) String() string {
	if utils.IsNil(r) {
		return ""
	}
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v int64, _ int) string {
				result := r.GetValueById(v)
				if utils.IsNil(result) {
					return fmt.Sprintf("<nil:%d>", v)
				}
				return getStr(result)
			}),
			", ",
		),
	)
}

// ----------- Call
func (c *Call) String() string {
	if utils.IsNil(c) {
		return ""
	}

	methodValue := c.GetValueById(c.Method)
	methodStr := "<nil>"
	if !utils.IsNil(methodValue) {
		methodStr = getStr(methodValue)
	}

	argStr := strings.Join(
		lo.Map(c.Args, func(id int64, index int) string {
			arg := c.GetValueById(id)
			if utils.IsNil(arg) {
				return fmt.Sprintf("<nil arg:%d>", id)
			}
			return getStr(arg)
		}),
		", ",
	)

	binding := "binding[" + strings.Join(
		lo.MapToSlice(c.Binding, func(name string, v int64) string {
			bindValue := c.GetValueById(v)
			if utils.IsNil(bindValue) {
				return fmt.Sprintf("%s:<nil:%d>", name, v)
			}
			return getStr(bindValue)
		}),
		", ",
	) + "]"

	member := "member[" + strings.Join(
		lo.Map(c.ArgMember, func(v int64, _ int) string {
			memberValue := c.GetValueById(v)
			if utils.IsNil(memberValue) {
				return fmt.Sprintf("<nil:%d>", v)
			}
			return getStr(memberValue)
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
	if utils.IsNil(s) {
		return ""
	}

	valueStr := "<nil>"
	if value := s.GetValueById(s.Value); !utils.IsNil(value) {
		valueStr = getStr(value)
	}

	callSiteStr := "<nil>"
	if callSite := s.GetValueById(s.CallSite); !utils.IsNil(callSite) {
		callSiteStr = getStr(callSite)
	}

	return fmt.Sprintf("%s = side-effect %s [%s] by %s", getStr(s), valueStr, s.GetVerboseName(), callSiteStr)
}

// ----------- Switch
func (sw *Switch) String() string {
	if utils.IsNil(sw) {
		return ""
	}

	defaultBlockName := "<nil>"
	if !utils.IsNil(sw.DefaultBlock) {
		defaultBlockName = sw.DefaultBlock.GetName()
	}

	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		getStr(sw.GetValueById(sw.Cond)),
		defaultBlockName,
		strings.Join(
			lo.Map(sw.Label, func(label SwitchLabel, _ int) string {
				valueStr := getStr(sw.GetValueById(label.Value))

				destValue := sw.GetValueById(label.Dest)
				destName := "<nil>"
				if !utils.IsNil(destValue) {
					destName = destValue.GetName()
				}

				return fmt.Sprintf("%s:%s", valueStr, destName)
			}),
			", ",
		),
	)
}

// ----------- BinOp
func (b *BinOp) String() string {
	if utils.IsNil(b) {
		return ""
	}

	xStr := "<nil>"
	if x := b.GetValueById(b.X); !utils.IsNil(x) {
		xStr = getStr(x)
	}

	yStr := "<nil>"
	if y := b.GetValueById(b.Y); !utils.IsNil(y) {
		yStr = getStr(y)
	}

	return fmt.Sprintf("%s = %s %s %s", getStr(b), xStr, b.Op, yStr)
}

// ----------- UnOp
func (u *UnOp) String() string {
	if utils.IsNil(u) {
		return ""
	}

	xStr := "<nil>"
	if x := u.GetValueById(u.X); !utils.IsNil(x) {
		xStr = getStr(x)
	}

	return fmt.Sprintf("%s = %s %s", getStr(u), u.Op, xStr)
}

// ----------- Interface
func (i *Make) String() string {
	if utils.IsNil(i) {
		return ""
	}
	if i.parentI > 0 {
		parentStr := "<nil>"
		if parent := i.GetValueById(i.parentI); !utils.IsNil(parent) {
			parentStr = getStr(parent)
		}

		lowStr := "<nil>"
		if low := i.GetValueById(i.low); !utils.IsNil(low) {
			lowStr = getStr(low)
		}

		highStr := "<nil>"
		if high := i.GetValueById(i.high); !utils.IsNil(high) {
			highStr = getStr(high)
		}

		stepStr := "<nil>"
		if step := i.GetValueById(i.step); !utils.IsNil(step) {
			stepStr = getStr(step)
		}

		return fmt.Sprintf(
			"%s = %s [%s:%s:%s]",
			getStr(i), parentStr, lowStr, highStr, stepStr,
		)
	} else {
		lenStr := "<nil>"
		if len := i.GetValueById(i.Len); !utils.IsNil(len) {
			lenStr = getStr(len)
		}

		capStr := "<nil>"
		if cap := i.GetValueById(i.Cap); !utils.IsNil(cap) {
			capStr = getStr(cap)
		}

		str := fmt.Sprintf(
			"%s = make %s [%s, %s]",
			getStr(i), i.GetType(), lenStr, capStr,
		)
		if i.name != "" {
			str += "// " + i.name
		}
		return str
	}
}

func (t *TypeCast) String() string {
	if utils.IsNil(t) {
		return ""
	}

	valueStr := "<nil>"
	if value := t.GetValueById(t.Value); !utils.IsNil(value) {
		valueStr = getStr(value)
	}

	return fmt.Sprintf(
		"%s = type-case[%s] %s",
		getStr(t), t.GetType(), valueStr,
	)
}

func (t *TypeValue) String() string {
	if utils.IsNil(t) {
		return ""
	}
	return fmt.Sprintf(
		"%s = type-value[%s]",
		getStr(t), t.GetType(),
	)
}

func (a *Assert) String() string {
	if utils.IsNil(a) {
		return ""
	}
	msg := a.Msg
	if a.MsgValue > 0 {
		if msgValue := a.GetValueById(a.MsgValue); !utils.IsNil(msgValue) {
			msg = getStr(msgValue)
		} else {
			msg = fmt.Sprintf("<nil msg:%d>", a.MsgValue)
		}
	}

	condStr := "<nil>"
	if cond := a.GetValueById(a.Cond); !utils.IsNil(cond) {
		condStr = getStr(cond)
	}

	return fmt.Sprintf(
		"assert[%s] %s",
		condStr, msg,
	)
}

func (n *Next) String() string {
	if utils.IsNil(n) {
		return ""
	}

	iterStr := "<nil>"
	if iter := n.GetValueById(n.Iter); !utils.IsNil(iter) {
		iterStr = getStr(iter)
	}

	return fmt.Sprintf(
		"%s = next[%s]",
		getStr(n), iterStr,
	)
}

func (e *ErrorHandler) String() string {
	if utils.IsNil(e) {
		return ""
	}

	finalName := "nil"
	if e.Final > 0 {
		final := e.GetValueById(e.Final)
		if !utils.IsNil(final) {
			finalName = final.GetName()
		} else {
			finalName = fmt.Sprintf("<nil final:%d>", e.Final)
		}
	}

	tryBlock := e.GetValueById(e.Try)
	tryName := "<nil>"
	if !utils.IsNil(tryBlock) {
		tryName = tryBlock.GetName()
	} else {
		tryName = fmt.Sprintf("<nil try:%d>", e.Try)
	}

	doneBlock := e.GetValueById(e.Done)
	doneName := "<nil>"
	if !utils.IsNil(doneBlock) {
		doneName = doneBlock.GetName()
	} else {
		doneName = fmt.Sprintf("<nil done:%d>", e.Done)
	}

	return fmt.Sprintf(
		"try %s; catch %s; final %s; rest %s",
		tryName,
		"",
		finalName,
		doneName,
	)
}

func (e *ErrorCatch) String() string {
	if utils.IsNil(e) {
		return ""
	}

	catchBodyStr := "<nil>"
	if catchBody := e.GetValueById(e.CatchBody); !utils.IsNil(catchBody) {
		catchBodyStr = getStr(catchBody)
	}

	exceptionStr := "<nil>"
	if exception := e.GetValueById(e.Exception); !utils.IsNil(exception) {
		exceptionStr = getStr(exception)
	}

	return fmt.Sprintf(
		"catch %s; body %s; exception %s",
		e.GetName(), catchBodyStr, exceptionStr,
	)
}

func (p *Panic) String() string {
	if utils.IsNil(p) {
		return ""
	}

	infoStr := "<nil>"
	if info := p.GetValueById(p.Info); !utils.IsNil(info) {
		infoStr = getStr(info)
	}

	return fmt.Sprintf(
		"panic %s",
		infoStr,
	)
}

func (r *Recover) String() string {
	if utils.IsNil(r) {
		return ""
	}
	return getStr(r) + " = recover"
}
