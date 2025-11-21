package ssa

import (
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

// Helper functions
func filterNilValue(v Value) bool {
	return v != nil
}

func ReplaceAllValue(v Value, to Value) {
	ReplaceValue(v, to, func(i Instruction) bool { return false })
}

func ReplaceValue(v Value, to Value, skip func(Instruction) bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("=============================\n"+"replace value panic: %v", r)
		}
	}()
	if v.getAnValue() == to.getAnValue() {
		return
	}

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
		user.ReplaceValue(v, to)

		to.AddUser(user)
		deleteInst = append(deleteInst, user)
	}
	for _, user := range deleteInst {
		v.RemoveUser(user)
	}
	v.RefreshString()
	to.RefreshString()
}

// ----------- Function
func (f *Function) HasValues() bool   { return false }
func (f *Function) GetValues() Values { return nil }

// ----------- BasicBlock
func (b *BasicBlock) HasValues() bool   { return false }
func (b *BasicBlock) GetValues() Values { return nil }

// ----------- Phi
func (p *Phi) HasValues() bool   { return true }
func (p *Phi) GetValues() Values { return p.GetValuesByIDs(p.Edge) }

func (p *Phi) ReplaceValue(v Value, to Value) {
	// p.Edge = slices.Replace(p.Edge, 0, len(p.Edge), v, to)
	if index := slices.Index(p.Edge, v.GetId()); index != -1 {
		p.Edge[index] = to.GetId()
	} else {
		log.Warnf("phi not use this value")
	}
}

func (p *Phi) GetControlFlowConditions() []Value {
	if p == nil {
		return nil
	}
	if p.CFGEntryBasicBlock <= 0 {
		return nil
	}

	inst, ok := p.GetInstructionById(p.CFGEntryBasicBlock)
	if !ok {
		log.Warnf("phi's cfg block enter is not a valid instruction")
		return nil
	}
	block, ok := ToBasicBlock(inst)
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
			val, ok := p.GetValueById(ret.Cond)
			return val, ok
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
func (e *ExternLib) GetValues() Values { return e.GetValuesByIDs(e.Member) }
func (e *ExternLib) ReplaceValue(v Value, to Value) {
	if index := slices.Index(e.Member, v.GetId()); index != -1 {
		e.Member[index] = to.GetId()
		e.MemberMap[v.GetName()] = to.GetId()
	}
}

// // ----------- param
func (p *Parameter) HasValues() bool   { return false }
func (p *Parameter) GetValues() Values { return nil }

func (p *ParameterMember) HasValues() bool   { return false }
func (p *ParameterMember) GetValues() Values { return nil }

// ----------- ConstInst
func (c *ConstInst) HasValues() bool {
	return c.Origin > 0
}

func (c *ConstInst) GetValues() Values {
	if c.Origin > 0 {
		val, ok := c.GetValueById(c.Origin)
		if ok {
			return val.GetValues()
		}
	}
	return nil
}

func (c *ConstInst) ReplaceValue(v Value, to Value) {
	if c.Origin > 0 {
		user, ok := c.GetUsersByID(c.Origin)
		if ok {
			user.ReplaceValue(v, to)
		}
	}
}

// ----------- undefined
func (u *Undefined) HasValues() bool   { return false }
func (u *Undefined) GetValues() Values { return nil }

// ----------- BinOp
func (b *BinOp) HasValues() bool   { return true }
func (b *BinOp) GetValues() Values { return b.GetValuesByIDs([]int64{b.X, b.Y}) }

func (b *BinOp) ReplaceValue(v Value, to Value) {
	if b.X == v.GetId() {
		b.X = to.GetId()
	} else if b.Y == v.GetId() {
		b.Y = to.GetId()
	} else {
		panic("BinOp not use this value")
	}
}

// ----------- UnOp
func (n *UnOp) HasValues() bool   { return true }
func (n *UnOp) GetValues() Values { return n.GetValuesByIDs([]int64{n.X}) }

func (u *UnOp) ReplaceValue(v Value, to Value) {
	if u.X == v.GetId() {
		u.X = to.GetId()
	} else {
		panic("UnOp not use this value")
	}
}

// ----------- Call
func (c *Call) HasValues() bool { return true }
func (c *Call) GetValues() Values {
	ret := make(Values, 0, len(c.Args)+len(c.Binding)+1)
	if method, ok := c.GetValueById(c.Method); ok {
		ret = append(ret, method)
	}
	ret = append(ret, c.GetValuesByIDs(c.Args)...)
	for _, v := range c.Binding {
		if val, ok := c.GetValueById(v); ok {
			ret = append(ret, val)
		}
	}
	return ret
}

func (c *Call) ReplaceValue(v Value, to Value) {
	if c.Method == v.GetId() {
		c.Method = to.GetId()
		c.handlerObjectMethod()
		c.handleCalleeFunction()
		c.handlerReturnType()
	}
	lo.ForEach(c.Args, func(id int64, index int) {
		if id == v.GetId() {
			c.Args[index] = to.GetId()
		}
		return
	})

	lo.ForEach(c.Args, func(id int64, index int) {
		if id == v.GetId() {
			c.Args[index] = to.GetId()
		}
		return
	})

	lo.ForEach(c.ArgMember, func(id int64, index int) {
		c.ArgMember[index] = to.GetId()
	})
}

// ------------ SideEffect
func (s *SideEffect) HasValues() bool   { return true }
func (s *SideEffect) GetValues() Values { return s.GetValuesByIDs([]int64{s.CallSite, s.Value}) }
func (s *SideEffect) ReplaceValue(v Value, to Value) {
	if s.CallSite == v.GetId() {
		s.CallSite = to.GetId()
	} else if s.Value == v.GetId() {
		s.Value = to.GetId()
	} else {
		panic("SideEffect not use this value")
	}
}

// ----------- Return
func (r *Return) HasValues() bool   { return true }
func (r *Return) GetValues() Values { return r.GetValuesByIDs(r.Results) }
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v.GetId()); index > -1 {
		r.Results[index] = to.GetId()
	} else {
		panic("return not use this value")
	}
}
func (r *Return) HasUsers() bool  { return false }
func (r *Return) GetUsers() Users { return nil }

// // ----------- Make
func (i *Make) HasValues() bool { return true }
func (i *Make) GetValues() Values {
	ids := []int64{i.Cap, i.Len, i.high, i.low, i.step, i.parentI}
	return i.GetValuesByIDs(ids)
}
func (i *Make) ReplaceValue(v, to Value) {
	if i.Cap == v.GetId() {
		i.Cap = to.GetId()
	} else if i.Len == v.GetId() {
		i.Len = to.GetId()
	} else if i.high == v.GetId() {
		i.high = to.GetId()
	} else if i.low == v.GetId() {
		i.low = to.GetId()
	} else if i.step == v.GetId() {
		i.step = to.GetId()
	} else if i.parentI == v.GetId() {
		i.parentI = to.GetId()
	} else {
		log.Errorf("======================\n"+
			"BUG or make not use this value: object not use this value: %v"+
			"=========================", v)
	}
}

// // ----------- Next
func (n *Next) HasValues() bool   { return true }
func (n *Next) GetValues() Values { return n.GetValuesByIDs([]int64{n.Iter}) }
func (n *Next) ReplaceValue(v, to Value) {
	if n.Iter == v.GetId() {
		n.Iter = to.GetId()
	} else {
		panic("next instruction not use this value")
	}
}

// ----------- Assert
func (a *Assert) HasValues() bool { return true }
func (a *Assert) GetValues() Values {
	ret := a.GetValuesByIDs([]int64{a.Cond, a.MsgValue})
	return ret
}

func (a *Assert) ReplaceValue(v, to Value) {
	if a.Cond == v.GetId() {
		a.Cond = to.GetId()
	} else if a.MsgValue == v.GetId() {
		a.MsgValue = to.GetId()
	} else {
		panic("assert not use this value")
	}
}
func (r *Assert) HasUsers() bool  { return false }
func (r *Assert) GetUsers() Users { return nil }

// // ----------- Typecast
func (t *TypeCast) HasValues() bool   { return true }
func (t *TypeCast) GetValues() Values { return t.GetValuesByIDs([]int64{t.Value}) }
func (t *TypeCast) ReplaceValue(v, to Value) {
	if t.Value == v.GetId() {
		t.Value = to.GetId()
	} else {
		panic("type cast not use this value")
	}
}

// ------------ type value
func (t *TypeValue) HasValues() bool   { return false }
func (t *TypeValue) GetValues() Values { return nil }

// ------------- PANIC
func (p *Panic) HasValues() bool   { return true }
func (p *Panic) GetValues() Values { return p.GetValuesByIDs([]int64{p.Info}) }
func (p *Panic) ReplaceValue(v, to Value) {
	if p.Info == v.GetId() {
		p.Info = to.GetId()
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
	if i.Cond == 0 {
		return []Value{}
	}
	return i.GetValuesByIDs([]int64{i.Cond})
}
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v.GetId() {
		i.Cond = to.GetId()
	} else {
		panic("if not use this value")
	}
}
func (r *If) HasUsers() bool  { return false }
func (r *If) GetUsers() Users { return nil }

// ----------- Loop
func (l *Loop) HasValues() bool { return true }
func (l *Loop) GetValues() Values {
	return l.GetValuesByIDs([]int64{l.Cond, l.Init, l.Step, l.Key})
}
func (l *Loop) ReplaceValue(v Value, to Value) {
	if l.Cond == v.GetId() {
		l.Cond = to.GetId()
	} else if l.Init == v.GetId() {
		l.Init = to.GetId()
	} else if l.Step == v.GetId() {
		l.Step = to.GetId()
	} else if l.Key == v.GetId() {
		l.Key = to.GetId()
	} else {
		panic("loop not use this value")
	}
}
func (r *Loop) HasUsers() bool  { return false }
func (r *Loop) GetUsers() Users { return nil }

// ----------- Jump
func (l *Jump) HasValues() bool { return true }
func (l *Jump) GetValues() Values {
	if l.To == 0 {
		return nil
	}
	if val, ok := l.GetValueById(l.To); ok {
		return []Value{val}
	}
	return nil
}
func (l *Jump) ReplaceValue(v Value, to Value) {
	if l.To == v.GetId() {
		l.To = to.GetId()
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
			if v := label.Value; v != 0 {
				if val, ok := sw.GetValueById(v); ok {
					ret = append(ret, val)
				}
			}
		},
	)
	if sw.Cond != 0 {
		if val, ok := sw.GetValueById(sw.Cond); ok {
			ret = append(ret, val)
		}
	}
	return ret
}
func (sw *Switch) ReplaceValue(v Value, to Value) {
	if sw.Cond == v.GetId() {
		sw.Cond = to.GetId()
	}
	for _, c := range sw.Label {
		if c.Value == v.GetId() {
			c.Value = to.GetId()
		}
	}
}

func (r *Switch) HasUsers() bool  { return false }
func (r *Switch) GetUsers() Users { return nil }

// ----------- ErrorHandler
func (e *ErrorHandler) HasValues() bool { return true }
func (e *ErrorHandler) GetValues() Values {
	var vs Values
	for _, c := range e.Catch {
		if c != 0 {
			if val, ok := e.GetValueById(c); ok {
				vs = append(vs, val)
			}
		}
	}
	if e.Final != 0 {
		if val, ok := e.GetValueById(e.Final); ok {
			vs = append(vs, val)
		}
	}
	if e.Done != 0 {
		if val, ok := e.GetValueById(e.Done); ok {
			vs = append(vs, val)
		}
	}
	if e.Try != 0 {
		if val, ok := e.GetValueById(e.Try); ok {
			vs = append(vs, val)
		}
	}
	return vs
}
func (e *ErrorHandler) ReplaceValue(v Value, to Value) {
	// Check in catches
	for i, c := range e.Catch {
		if c == v.GetId() {
			e.Catch[i] = to.GetId()
			return
		}
	}
	// Check other fields
	if e.Final == v.GetId() {
		e.Final = to.GetId()
	} else if e.Done == v.GetId() {
		e.Done = to.GetId()
	} else if e.Try == v.GetId() {
		e.Try = to.GetId()
	} else {
		panic("error handler not use this value")
	}
}
func (e *ErrorHandler) HasUsers() bool  { return false }
func (e *ErrorHandler) GetUsers() Users { return nil }

func (e *ErrorCatch) HasValues() bool { return true }
func (e *ErrorCatch) GetValues() Values {
	var vs Values
	if e.CatchBody != 0 {
		if val, ok := e.GetValueById(e.CatchBody); ok {
			vs = append(vs, val)
		}
	}
	if e.Exception != 0 {
		if val, ok := e.GetValueById(e.Exception); ok {
			vs = append(vs, val)
		}
	}
	return vs
}
func (e *ErrorCatch) ReplaceValue(v Value, to Value) {
	if e.CatchBody == v.GetId() {
		e.CatchBody = to.GetId()
	} else if e.Exception == v.GetId() {
		e.Exception = to.GetId()
	} else {
		panic("error catch not use this value")
	}
}
func (e *ErrorCatch) HasUsers() bool  { return false }
func (e *ErrorCatch) GetUsers() Users { return nil }
