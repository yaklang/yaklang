package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func ReplaceAllValue(v Value, to Value) {
	ReplaceValue(v, to, func(i Instruction) bool { return false })
}

func ReplaceValue(v Value, to Value, skip func(Instruction) bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("=============================\n"+"replace value panic: %v", r)
		}
	}()

	for _, variable := range v.GetAllVariables() {
		// TODO: handler variable replace value
		variable.Replace(v, to)
		// variable = to
		to.AddVariable(variable)
		v.GetProgram().SetInstructionWithName(variable.GetName(), to)
	}

	deleteInst := make([]User, 0)
	for _, user := range v.GetUsers() {
		if skip(user) {
			continue
		}
		RefreshToCache(v, user)
		user.ReplaceValue(v, to)
		if ex, ok := ToExternLib(user); ok {
			ReplaceMemberCall(ex, to)
		}

		to.AddUser(user)
		deleteInst = append(deleteInst, user)
	}
	for _, user := range deleteInst {
		v.RemoveUser(user)
	}
}

func filterNilValue(v Value, _ int) bool {
	if utils.IsNil(v) {
		return false
	} else {
		return true
	}
}

// ----------- Function
func (f *Function) HasValues() bool   { return false }
func (f *Function) GetValues() Values { return nil }

// ----------- BasicBlock
func (b *BasicBlock) HasValues() bool   { return false }
func (b *BasicBlock) GetValues() Values { return nil }

// ----------- Phi
func (p *Phi) HasValues() bool   { return true }
func (p *Phi) GetValues() Values { return lo.Filter(p.Edge, filterNilValue) }

func (p *Phi) ReplaceValue(v Value, to Value) {
	// p.Edge = slices.Replace(p.Edge, 0, len(p.Edge), v, to)
	if index := slices.Index(p.Edge, v); index != -1 {
		p.Edge[index] = to
	} else {
		log.Warnf("phi not use this value")
	}
}

func (p *Phi) GetControlFlowConditions() []Value {
	if p == nil {
		return nil
	}
	if p.CFGEntryBasicBlock == nil {
		return nil
	}

	block, ok := ToBasicBlock(p.CFGEntryBasicBlock)
	if !ok {
		log.Warnf("phi's cfg block enter is not a valid *BasicBlock")
		return nil
	}
	relative, ok := block.IsCFGEnterBlock()
	if !ok {
		return nil
	}
	return lo.FilterMap(relative, func(ins Instruction, i int) (Value, bool) {
		switch ret := ins.(type) {
		case *If:
			return ret.Cond, true
		default:
			result, ok := ret.(Value)
			if ok {
				return result, true
			}
			return nil, false
		}
	})
}

// / ---- extern lib
func (e *ExternLib) HasValues() bool   { return true }
func (e *ExternLib) GetValues() Values { return lo.Filter(e.Member, filterNilValue) }
func (e *ExternLib) ReplaceValue(v Value, to Value) {
	if index := slices.Index(e.Member, v); index != -1 {
		e.Member[index] = to
		e.MemberMap[v.GetName()] = to
	}
}

// // ----------- param
func (p *Parameter) HasValues() bool   { return false }
func (p *Parameter) GetValues() Values { return nil }

func (p *ParameterMember) HasValues() bool   { return false }
func (p *ParameterMember) GetValues() Values { return nil }

// ----------- ConstInst
func (c *ConstInst) HasValues() bool {
	return c.Origin != nil
}

func (c *ConstInst) GetValues() Values {
	if c.Origin != nil {
		return c.Origin.GetValues()
	} else {
		return nil
	}
}

func (c *ConstInst) ReplaceValue(v Value, to Value) {
	if c.Origin != nil {
		c.Origin.ReplaceValue(v, to)
	}
}

// ----------- undefined
func (u *Undefined) HasValues() bool   { return false }
func (u *Undefined) GetValues() Values { return nil }

// ----------- BinOp
func (b *BinOp) HasValues() bool   { return true }
func (b *BinOp) GetValues() Values { return []Value{b.X, b.Y} }

func (b *BinOp) ReplaceValue(v Value, to Value) {
	if b.X == v {
		b.X = to
	} else if b.Y == v {
		b.Y = to
	} else {
		panic("BinOp not use this value")
	}
}

// ----------- UnOp
func (n *UnOp) HasValues() bool   { return true }
func (n *UnOp) GetValues() Values { return []Value{n.X} }

func (u *UnOp) ReplaceValue(v Value, to Value) {
	if u.X == v {
		u.X = to
	} else {
		panic("UnOp not use this value")
	}
}

// ----------- Call
func (c *Call) HasValues() bool { return true }
func (c *Call) GetValues() Values {
	ret := make(Values, 0, len(c.Args)+len(c.Binding)+1)
	ret = append(ret, c.Method)
	for _, v := range c.Args {
		ret = append(ret, v)
	}
	for _, v := range c.Binding {
		ret = append(ret, v)
	}
	return lo.Filter(ret, filterNilValue)
}

func (c *Call) ReplaceValue(v Value, to Value) {
	if c.Method == v {
		c.Method = to
		c.handlerObjectMethod()
		c.handleCalleeFunction()
		c.handlerReturnType()
	} else if index := slices.Index(c.Args, v); index > -1 {
		c.Args[index] = to
	} else if binding, ok := c.Binding[v.GetName()]; ok && binding == v {
		c.Binding[v.GetName()] = to
	} else if index := slices.Index(c.ArgMember, v); index > -1 {
		c.ArgMember[index] = to
	} else {
		panic("call not use this value")
	}
}

// ------------ SideEffect
func (s *SideEffect) HasValues() bool   { return true }
func (s *SideEffect) GetValues() Values { return []Value{s.CallSite, s.Value} }
func (s *SideEffect) ReplaceValue(v Value, to Value) {
	if s.CallSite == v {
		c, ok := ToCall(to)
		if !ok {
			log.Errorf("SideEffect not use this value")
			return
		}
		s.CallSite = c
	} else if s.Value == v {
		s.Value = to
	} else {
		panic("SideEffect not use this value")
	}
}

// ----------- Return
func (r *Return) HasValues() bool   { return true }
func (r *Return) GetValues() Values { return r.Results }
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v); index > -1 {
		r.Results[index] = to
	} else {
		panic("return not use this value")
	}
}

// node
func (r *Return) HasUsers() bool  { return false }
func (r *Return) GetUsers() Users { return nil }

// // ----------- Make
func (i *Make) HasValues() bool { return true }
func (i *Make) GetValues() Values {
	vs := []Value{i.Cap, i.Len, i.high, i.low, i.step, i.parentI}
	return lo.Filter(vs, filterNilValue)
}
func (i *Make) ReplaceValue(v, to Value) {
	if i.Cap == v {
		i.Cap = to
	} else if i.Len == v {
		i.Len = v
	} else if i.high == v {
		i.high = v
	} else if i.low == v {
		i.low = v
	} else if i.step == v {
		i.step = v
	} else if i.parentI == v {
		i.parentI = v
	} else {
		log.Errorf("======================\n"+
			"BUG or make not use this value: object not use this value: %v"+
			"=========================", v)
	}
}

// // ----------- Next
func (n *Next) HasValues() bool   { return true }
func (n *Next) GetValues() Values { return []Value{n.Iter} }
func (n *Next) ReplaceValue(v, to Value) {
	if n.Iter == v {
		n.Iter = to
	} else {
		panic("next instruction not use this value")
	}
}

// ----------- Assert
func (a *Assert) HasValues() bool { return true }
func (a *Assert) GetValues() Values {
	ret := []Value{a.Cond, a.MsgValue}
	return lo.Filter(ret, filterNilValue)
}

func (a *Assert) ReplaceValue(v, to Value) {
	if a.Cond == v {
		a.Cond = to
	} else if a.MsgValue == v {
		a.MsgValue = to
	} else {
		panic("assert not use this value")
	}
}
func (r *Assert) HasUsers() bool  { return false }
func (r *Assert) GetUsers() Users { return nil }

// // ----------- Typecast
func (t *TypeCast) HasValues() bool   { return true }
func (t *TypeCast) GetValues() Values { return []Value{t.Value} }
func (t *TypeCast) ReplaceValue(v, to Value) {
	if t.Value == v {
		t.Value = to
	} else {
		panic("type cast not use this value")
	}
}

// ------------ type value
func (t *TypeValue) HasValues() bool   { return false }
func (t *TypeValue) GetValues() Values { return nil }

// ------------- PANIC
func (p *Panic) HasValues() bool   { return true }
func (p *Panic) GetValues() Values { return []Value{p.Info} }
func (p *Panic) ReplaceValue(v, to Value) {
	if p.Info == v {
		p.Info = to
	} else {
		panic("panic instruction not use this value")
	}
}

// ---------- RECOVER
func (r *Recover) HasValues() bool   { return false }
func (r *Recover) GetValues() Values { return nil }

// ----------- IF
func (i *If) HasValues() bool { return true }
func (i *If) GetValues() Values {
	// return lo.Filter([]Value{i.Cond}, filterNilValue)
	if i.Cond == nil {
		return []Value{}
	}
	return []Value{i.Cond}
}
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v {
		i.Cond = to
	} else {
		panic("if not use this value")
	}
}
func (r *If) HasUsers() bool  { return false }
func (r *If) GetUsers() Users { return nil }

// ----------- Loop
func (l *Loop) HasValues() bool { return true }
func (l *Loop) GetValues() Values {
	return lo.Filter([]Value{l.Cond, l.Init, l.Step, l.Key}, filterNilValue)
}
func (l *Loop) ReplaceValue(v Value, to Value) {
	if l.Cond == v {
		l.Cond = to
	} else if l.Init == v {
		l.Init = to
	} else if l.Step == v {
		l.Step = to
	} else if l.Key == v {
		l.Key = to
	} else {
		panic("loop not use this value")
	}
}
func (r *Loop) HasUsers() bool  { return false }
func (r *Loop) GetUsers() Users { return nil }

// ----------- Jump
func (l *Jump) HasValues() bool { return true }
func (l *Jump) GetValues() Values {
	return lo.Filter([]Value{l.To}, filterNilValue)
}
func (l *Jump) ReplaceValue(v Value, to Value) {
	if l.To == v {
		l.To = to
	} else {
		panic("jump not use this value")
	}
}
func (r *Jump) HasUsers() bool  { return false }
func (r *Jump) GetUsers() Users { return nil }

// ----------- Switch
func (sw *Switch) HasValues() bool { return true }
func (sw *Switch) GetValues() Values {
	ret := make(Values, 0, len(sw.Label)+1)
	lo.ForEach(sw.Label,
		func(label SwitchLabel, _ int) {
			if v := label.Value; v != nil {
				ret = append(ret, v)
			}
		},
	)
	if sw.Cond != nil {
		ret = append(ret, sw.Cond)
	}
	return ret
}
func (sw *Switch) ReplaceValue(v Value, to Value) {
	if sw.Cond == v {
		sw.Cond = to
	}
	for _, c := range sw.Label {
		if c.Value == v {
			c.Value = to
		}
	}
}

func (r *Switch) HasUsers() bool  { return false }
func (r *Switch) GetUsers() Users { return nil }

// ----------- ErrorHandler
func (e *ErrorHandler) HasValues() bool { return true }
func (e *ErrorHandler) GetValues() Values {
	var vs Values = e.Catch
	vs = append(vs, e.Final, e.Done, e.Try)
	return lo.Filter(vs, filterNilValue)
}
func (e *ErrorHandler) ReplaceValue(v Value, to Value) {
	// Check in catches
	for i, c := range e.Catch {
		if c == v {
			e.Catch[i] = to
			return
		}
	}
	// Check other fields
	if e.Final == v {
		e.Final = to.(*BasicBlock)
	} else if e.Done == v {
		e.Done = to.(*BasicBlock)
	} else if e.Try == v {
		e.Try = to.(*BasicBlock)
	} else {
		panic("error handler not use this value")
	}
}
func (e *ErrorHandler) HasUsers() bool  { return false }
func (e *ErrorHandler) GetUsers() Users { return nil }
