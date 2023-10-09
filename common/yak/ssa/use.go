package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func ReplaceValue(v Value, to Value) {
	for _, user := range v.GetUsers() {
		user.ReplaceValue(v, to)
		// user.InferenceType()
		to.AddUser(user)
		v.RemoveUser(user)
	}
	// delete user in v.Value
	if user, ok := v.(User); ok {
		if toUser, ok := to.(User); ok {
			values := user.GetValues()
			for _, value := range values {
				switch v := value.(type) {
				// 		//TODO:handler field chain direction
				case *Field:
					v.Obj = toUser
					toUser.AddValue(v)
					v.RemoveUser(user)
					user.RemoveValue(v)
					// AddValue(user, v)
				default:
					// 			value.RemoveUser(user)
				}
			}
		}
	}
	if iv, ok := v.(InstructionValue); ok {
		iv.GetParent().ReplaceSymbolTable(iv, to)
		iv.GetParent().builder.ReplaceVariable(iv.GetVariable(), v, to)
	}
}

func GetUser(v Value) []User {
	user, ok := v.(User)
	return lo.Filter(v.GetUsers(), func(u User, _ int) bool {
		if utils.IsNil(u) || (ok && u == user) {
			return false
		} else {
			return true
		}
	})
}

func AddValue(user User, v Value) {
	if index := slices.Index(user.GetValues(), v); index == -1 {
		user.AddValue(v)
	}
}

// func GetValue(user User) []Value {
// 	value, ok := user.(Value)
// 	var values []Value
// 	if phi, ok := user.(*Phi); ok {
// 		values = phi.values
// 	} else {
// 		values = lo.Uniq(user.GetValues())
// 	}
// 	return lo.Uniq(lo.Filter(values, func(v Value, _ int) bool {
// 		if utils.IsNil(v) || (ok && v == value) {
// 			return false
// 		} else {
// 			return true
// 		}
// 	}))
// }

func AddUser(v Value, u User) {
	if index := slices.Index(v.GetUsers(), u); index == -1 {
		v.AddUser(u)
	}
}

func HasUser(n Node) bool {
	if v, ok := n.(Value); ok {
		if len(v.GetUsers()) != 0 {
			return true
		}
	}
	if u, ok := n.(User); ok {
		for _, v := range u.GetValues() {
			if _, ok := v.(*Field); ok {
				return true
			}
		}
	}
	return false
}

// ----------- Function
func (f *Function) GetValues() []Value { return nil }

func (f *Function) GetUsers() []User { return f.user }
func (f *Function) AddUser(u User)   { f.user = append(f.user, u) }

func (f *Function) RemoveUser(u User) { f.user = utils.Remove(f.user, u) }

// ----------- BasicBlock
func (b *BasicBlock) GetValues() []Value { return nil }

func (b *BasicBlock) GetUsers() []User { return b.user }
func (b *BasicBlock) AddUser(u User)   { b.user = append(b.user, u) }

func (b *BasicBlock) RemoveUser(u User) { b.user = utils.Remove(b.user, u) }

// // ----------- Phi
func (p *Phi) ReplaceValue(v Value, to Value) {
	// p.Edge = slices.Replace(p.Edge, 0, len(p.Edge), v, to)
	if index := slices.Index(p.Edge, v); index != -1 {
		p.Edge[index] = to
	} else {
		panic("phi not use this value")
	}
}

func (p *Phi) AddEdge(v Value) {
	p.Edge = append(p.Edge, v)
	p.AddValue(v)
}

// ----------- ConstInst
func (c *ConstInst) ReplaceValue(v, to Value) {
	panic("this const instruction con't replace ")
}

// ----------- undefine
func (c *Undefine) ReplaceValue(v, to Value) {
	panic("undefine instruction con't replace")
}

// // ----------- param
func (p *Parameter) ReplaceValue(v, to Value) {
	panic("parameter instruction con't replace")
}

// ----------- IF
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v {
		i.Cond = to
	} else {
		panic("if not use this value")
	}
}

func (i *If) GetUsers() []User { return nil }

func (i *If) GetValues() []Value  { return []Value{i.Cond} }
func (i *If) AddValue(v Value)    {}
func (i *If) RemoveValue(v Value) {}

// ----------- Loop
func (l *Loop) ReplaceValue(v Value, to Value) {
	if l.Cond == v {
		l.Cond = to
	} else if l.Init == v {
		l.Init = to
	} else if l.Step == v {
		l.Step = to
	} else {
		panic("loop not use this value")
	}
}

func (l *Loop) GetUsers() []User { return nil }

func (l *Loop) GetValues() []Value  { return []Value{l.Cond, l.Step, l.Init} }
func (l *Loop) AddValue(v Value)    {}
func (l *Loop) RemoveValue(v Value) {}

// ----------- Return
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v); index > -1 {
		r.Results[index] = to
	} else {
		panic("return not use this value")
	}
}

func (r *Return) GetUsers() []User { return nil }

func (r *Return) GetValues() []Value  { return r.Results }
func (r *Return) AddValue(v Value)    {}
func (r *Return) RemoveValue(v Value) {}

// // ----------- Call
func (c *Call) ReplaceValue(v Value, to Value) {
	if c.Method == v {
		c.Method = to
	} else if index := slices.Index(c.Args, v); index > -1 {
		c.Args[index] = to
	} else {
		panic("call not use this value")
	}

}

// ----------- Switch
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

func (sw *Switch) GetUsers() []User { return nil }

func (sw *Switch) GetValues() []Value {
	return append(
		lo.Map(sw.Label,
			func(label SwitchLabel, _ int) Value { return label.Value },
		),
		sw.Cond,
	)
}
func (sw *Switch) AddValue(v Value)    {}
func (sw *Switch) RemoveValue(v Value) {}

// // ----------- BinOp
func (b *BinOp) ReplaceValue(v Value, to Value) {
	if b.X == v {
		b.X = to
	} else if b.Y == v {
		b.Y = to
	} else {
		panic("BinOp not use this value")
	}

}

// // ----------- UnOp

func (u *UnOp) ReplaceValue(v Value, to Value) {
	if u.X == v {
		u.X = to
	} else {
		panic("UnOp not use this value")
	}
}

// // ----------- Interface
func (i *Make) ReplaceValue(v, to Value) {
	if i.Cap == v {
		i.Cap = to
	} else if i.Len == v {
		i.Len = v
	} else {
		panic("object not use this value")
	}
}

// // ----------- Field
func (f *Field) ReplaceValue(v, to Value) {
	if f.Key == v {
		f.Key = to
	} else if index := slices.Index(f.Update, v); index > -1 {
		f.Update[index] = to
	} else {
		panic("field not use this value")
	}
}

// ----------- Update
func (s *Update) ReplaceValue(v, to Value) {
	if s.Value == v {
		s.Value = to
	} else {
		panic("update not use this value")
	}
}

func (s *Update) GetUsers() []User { return []User{s.Address} }
func (s *Update) AddUser(u User)   {}
func (s *Update) RemoveUser(u User) {
	if s.Address == u {
		s.Address = nil
	} else {
		panic("update not have this user")
	}
}

func (s *Update) GetValues() []Value  { return []Value{s.Value} }
func (s *Update) AddValue(_ Value)    {}
func (s *Update) RemoveValue(_ Value) {}

// // ----------- Typecast
func (t *TypeCast) ReplaceValue(v, to Value) {
	if t.Value == v {
		t.Value = to
	} else {
		panic("type cast not use this value")
	}
}

// ----------- Assert
func (a *Assert) GetValues() []Value  { return []Value{a.Cond, a.MsgValue} }
func (a *Assert) GetUsers() []User    { return nil }
func (a *Assert) AddValue(v Value)    {}
func (a *Assert) RemoveValue(v Value) {}

func (a *Assert) ReplaceValue(v, to Value) {
	if a.Cond == v {
		a.Cond = to
	} else if a.MsgValue == v {
		a.MsgValue = to
	} else {
		panic("assert not use this value")
	}
}

// // ----------- Next
func (n *Next) ReplaceValue(v, to Value) {
	if n.Iter == v {
		n.Iter = to
	} else {
		panic("next instruction not use this value")
	}
}

// ------------- PANIC
func (p *Panic) ReplaceValue(v, to Value) {
	if p.Info == v {
		p.Info = to
	} else {
		panic("panic instruction not use this value")
	}
}
