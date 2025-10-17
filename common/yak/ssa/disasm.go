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
			item, ok := f.GetValueById(id)
			if !ok || utils.IsNil(item) {
				return "<nil>"
			} else {
				return fmt.Sprintf("%s(%d) %s", GetTypeStr(item), item.GetId(), item.GetName())
			}
		}),
		", ")
	ret += "\n"

	if id := f.parent; id > 0 {
		parent, ok := f.GetValueById(id)
		if ok && !utils.IsNil(parent) {
			ret += fmt.Sprintf("parent: %s\n", parent.GetName())
		} else {
			ret += fmt.Sprintf("parent: <nil:%d>\n", id)
		}
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.MapToSlice(f.FreeValues, func(name *Variable, id int64) string {
				item, ok := f.GetValueById(id)
				if !ok || utils.IsNil(item) {
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
				item, ok := f.GetValueById(id)
				if !ok || utils.IsNil(item) {
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
					key, ok := f.GetValueById(se.MemberCallKey)
					if !ok || utils.IsNil(key) {
						return fmt.Sprintf("parameter[%d].<nil key:%d>", se.MemberCallObjectIndex, se.MemberCallKey)
					}
					return fmt.Sprintf("parameter[%d].%s", se.MemberCallObjectIndex, key)
				case FreeValueMemberCall:
					key, ok := f.GetValueById(se.MemberCallKey)
					if !ok || utils.IsNil(key) {
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
				phi, ok := f.GetValueById(p)
				if !ok || utils.IsNil(phi) {
					ret += fmt.Sprintf("\t<nil phi:%d>\n", p)
				} else {
					ret += fmt.Sprintf("\t%s\n", phi)
				}
			}
			for _, id := range b.Insts {
				i, ok := b.GetInstructionById(id)
				if !ok || i == nil {
					ret += fmt.Sprintf("\t<nil inst:%d>\n", id)
					continue
				}
				if c, ok := ToConstInst(i); ok {
					if c.Origin > 0 {
						ret += fmt.Sprintf("\tt%d = %s by t%d \n", id, getStr(c), c.Origin)
					} else {
						ret += fmt.Sprintf("\tt%d = %s\n", id, getStr(c))
					}
					continue
				} else if _, ok := ToUndefined(i); ok {
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
				p, ok := b.GetValueById(id)
				if !ok || p == nil {
					insts = append(insts, fmt.Sprintf("\t<nil phi:%d>", id))
					pos = append(pos, "")
					continue
				}
				insts = append(insts, fmt.Sprintf("\t%s", p))
				r := p.GetRange()
				if r == nil {
					pos = append(pos, "")
				} else {
					pos = append(pos, r.String())
				}
			}
			for _, id := range b.Insts {
				i, ok := b.GetInstructionById(id)
				if !ok || i == nil {
					insts = append(insts, fmt.Sprintf("\t<nil inst:%d>", id))
					pos = append(pos, "")
					continue
				}
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
		block, ok := f.GetBasicBlockByID(b)
		if !ok || utils.IsNil(block) {
			log.Errorf("function %s has nil block: %d", f.GetName(), b)
			continue
		}
		ShowBlock(block)
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
			pred, ok := b.GetBasicBlockByID(id)
			if !ok || utils.IsNil(pred) {
				log.Infof("pred is nil: %v", id)
				continue
			}
			ret += pred.GetName() + " "
		}
	}
	if !utils.IsNil(b.Condition) {
		condition, ok := b.GetValueById(b.Condition)
		if ok && !utils.IsNil(condition) {
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
	return fmt.Sprintf("undefined[%s]%s", u.GetName(), valid)
}

// ----------- Phi
func (p *Phi) String() string {
	if utils.IsNil(p) {
		return ""
	}
	var valueNames []string
	for _, v := range p.Edge {
		val, ok := p.GetValueById(v)
		if !ok || val == nil {
			valueNames = append(valueNames, fmt.Sprintf("<nil:%d>", v))
		} else {
			valueNames = append(valueNames, getStrFlag(val, false))
		}
	}
	return fmt.Sprintf("%s = phi [%s]", getStrFlag(p, true), strings.Join(valueNames, ", "))
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
		key, ok := p.GetValueById(p.MemberCallKey)
		if !ok || utils.IsNil(key) {
			return fmt.Sprintf("parameter[%d].<nil key:%d>", p.MemberCallObjectIndex, p.MemberCallKey)
		}
		return fmt.Sprintf("parameter[%d].%s", p.MemberCallObjectIndex, key)
	case FreeValueMemberCall:
		key, ok := p.GetValueById(p.MemberCallKey)
		if !ok || utils.IsNil(key) {
			return fmt.Sprintf("freeValue-%s.<nil key:%d>", p.MemberCallObjectName, p.MemberCallKey)
		}
		return fmt.Sprintf("freeValue-%s.%s", p.MemberCallObjectName, key)
	case MoreParameterMember:
		key, ok := p.GetValueById(p.MemberCallKey)
		if !ok || utils.IsNil(key) {
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
	to, ok := j.GetValueById(j.To)
	if !ok || utils.IsNil(to) {
		return fmt.Sprintf("jump -> <nil target:%d>", j.To)
	}
	return fmt.Sprintf("jump -> %v", to.GetName())
}

// ----------- IF
func (i *If) String() string {
	if utils.IsNil(i) {
		return ""
	}
	cond, ok := i.GetValueById(i.Cond)
	if !ok || utils.IsNil(cond) {
		return "If [nil condition]"
	}
	trueBranch, ok := i.GetValueById(i.True)
	if !ok || utils.IsNil(trueBranch) {
		return fmt.Sprintf("If [%s] true -> nil", getStr(cond))
	}
	falseBranch, ok := i.GetValueById(i.False)
	if !ok || utils.IsNil(falseBranch) {
		return fmt.Sprintf("If [%s] true -> %s, false -> nil", getStr(cond), trueBranch.GetName())
	}
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(cond), trueBranch.GetName(), falseBranch.GetName())
}

// ----------- Loop
func (l *Loop) String() string {
	if utils.IsNil(l) {
		return ""
	}

	bodyValue, ok := l.GetValueById(l.Body)
	bodyName := "<nil>"
	if ok && !utils.IsNil(bodyValue) {
		bodyName = bodyValue.GetName()
	}

	exitValue, ok := l.GetValueById(l.Exit)
	exitName := "<nil>"
	if ok && !utils.IsNil(exitValue) {
		exitName = exitValue.GetName()
	}

	// Helper function to safely get string representation
	getValueStr := func(id int64) string {
		if val, ok := l.GetValueById(id); ok && val != nil {
			return getStr(val)
		}
		return fmt.Sprintf("<nil:%d>", id)
	}

	return fmt.Sprintf("Loop [%s; %s; %s] body -> %s, exit -> %s",
		getValueStr(l.Init),
		getValueStr(l.Cond),
		getValueStr(l.Step),
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
				result, ok := r.GetValueById(v)
				if !ok || utils.IsNil(result) {
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

	methodValue, ok := c.GetValueById(c.Method)
	methodStr := "<nil>"
	if ok && !utils.IsNil(methodValue) {
		methodStr = getStr(methodValue)
	}

	argStr := strings.Join(
		lo.Map(c.Args, func(id int64, index int) string {
			arg, ok := c.GetValueById(id)
			if !ok || utils.IsNil(arg) {
				return fmt.Sprintf("<nil arg:%d>", id)
			}
			return getStr(arg)
		}),
		", ",
	)

	binding := "binding[" + strings.Join(
		lo.MapToSlice(c.Binding, func(name string, v int64) string {
			bindValue, ok := c.GetValueById(v)
			if !ok || utils.IsNil(bindValue) {
				return fmt.Sprintf("%s:<nil:%d>", name, v)
			}
			return getStr(bindValue)
		}),
		", ",
	) + "]"

	member := "member[" + strings.Join(
		lo.Map(c.ArgMember, func(v int64, _ int) string {
			memberValue, ok := c.GetValueById(v)
			if !ok || utils.IsNil(memberValue) {
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
	if value, ok := s.GetValueById(s.Value); ok && !utils.IsNil(value) {
		valueStr = getStr(value)
	}

	callSiteStr := "<nil>"
	if callSite, ok := s.GetValueById(s.CallSite); ok && !utils.IsNil(callSite) {
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

	// Helper function to safely get string representation
	safeGetStr := func(id int64) string {
		if val, ok := sw.GetValueById(id); ok && val != nil {
			return getStr(val)
		}
		return fmt.Sprintf("<nil:%d>", id)
	}

	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		safeGetStr(sw.Cond),
		defaultBlockName,
		strings.Join(
			lo.Map(sw.Label, func(label SwitchLabel, _ int) string {
				valueStr := safeGetStr(label.Value)

				destValue, ok := sw.GetValueById(label.Dest)
				destName := "<nil>"
				if ok && !utils.IsNil(destValue) {
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
	if x, ok := b.GetValueById(b.X); ok && !utils.IsNil(x) {
		xStr = getStr(x)
	}

	yStr := "<nil>"
	if y, ok := b.GetValueById(b.Y); ok && !utils.IsNil(y) {
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
	if x, ok := u.GetValueById(u.X); ok && !utils.IsNil(x) {
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
		if parent, ok := i.GetValueById(i.parentI); ok && !utils.IsNil(parent) {
			parentStr = getStr(parent)
		}

		lowStr := "<nil>"
		if low, ok := i.GetValueById(i.low); ok && !utils.IsNil(low) {
			lowStr = getStr(low)
		}

		highStr := "<nil>"
		if high, ok := i.GetValueById(i.high); ok && !utils.IsNil(high) {
			highStr = getStr(high)
		}

		stepStr := "<nil>"
		if step, ok := i.GetValueById(i.step); ok && !utils.IsNil(step) {
			stepStr = getStr(step)
		}

		return fmt.Sprintf(
			"%s = %s [%s:%s:%s]",
			getStr(i), parentStr, lowStr, highStr, stepStr,
		)
	} else {
		lenStr := "<nil>"
		if len, ok := i.GetValueById(i.Len); ok && !utils.IsNil(len) {
			lenStr = getStr(len)
		}

		capStr := "<nil>"
		if cap, ok := i.GetValueById(i.Cap); ok && !utils.IsNil(cap) {
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
	if value, ok := t.GetValueById(t.Value); ok && !utils.IsNil(value) {
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
		if msgValue, ok := a.GetValueById(a.MsgValue); ok && !utils.IsNil(msgValue) {
			msg = getStr(msgValue)
		} else {
			msg = fmt.Sprintf("<nil msg:%d>", a.MsgValue)
		}
	}

	condStr := "<nil>"
	if cond, ok := a.GetValueById(a.Cond); ok && !utils.IsNil(cond) {
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
	if iter, ok := n.GetValueById(n.Iter); ok && !utils.IsNil(iter) {
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
		final, ok := e.GetValueById(e.Final)
		if ok && !utils.IsNil(final) {
			finalName = final.GetName()
		} else {
			finalName = fmt.Sprintf("<nil final:%d>", e.Final)
		}
	}

	tryBlock, ok := e.GetValueById(e.Try)
	tryName := "<nil>"
	if ok && !utils.IsNil(tryBlock) {
		tryName = tryBlock.GetName()
	} else {
		tryName = fmt.Sprintf("<nil try:%d>", e.Try)
	}

	doneBlock, ok := e.GetValueById(e.Done)
	doneName := "<nil>"
	if ok && !utils.IsNil(doneBlock) {
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
	if catchBody, ok := e.GetValueById(e.CatchBody); ok && !utils.IsNil(catchBody) {
		catchBodyStr = getStr(catchBody)
	}

	exceptionStr := "<nil>"
	if exception, ok := e.GetValueById(e.Exception); ok && !utils.IsNil(exception) {
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
	if info, ok := p.GetValueById(p.Info); ok && !utils.IsNil(info) {
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
