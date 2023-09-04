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
	if iv, ok := v.(InstructionValue); ok {
		iv.GetParent().ReplaceSymbolTable(iv, to)
	}
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

// ----------- Phi
func (p *Phi) ReplaceValue(v Value, to Value) {
	slices.Replace(p.Edge, 0, len(p.Edge), v, to)
}

func (p *Phi) GetUsers() []User { return p.user }
func (p *Phi) AddUser(u User)   { p.user = append(p.user, u) }

func (p *Phi) RemoveUser(u User) { p.user = utils.Remove(p.user, u) }

func (p *Phi) GetValues() []Value { return p.Edge }
func (p *Phi) AddValue(v Value)   {}

// ----------- Const
func (c *Const) GetValues() []Value { return nil }

func (c *Const) GetUsers() []User { return c.user }
func (c *Const) AddUser(u User)   { c.user = append(c.user, u) }

func (c *Const) RemoveUser(u User) { c.user = utils.Remove(c.user, u) }

// ----------- undifne
// node
func (u *Undefine) GetValues() []Value { return u.values }
func (c *Undefine) GetUsers() []User   { return c.user }

// value
func (c *Undefine) AddUser(u User)    { c.user = append(c.user, u) }
func (c *Undefine) RemoveUser(u User) { c.user = utils.Remove(c.user, u) }

// user
func (c *Undefine) AddValue(v Value)         { c.values = append(c.values, v) }
func (c *Undefine) RemoveValue(u User)       { c.user = utils.Remove(c.user, u) }
func (c *Undefine) ReplaceValue(v, to Value) { slices.Replace(c.values, 0, len(c.values), v, to) }

// ----------- param
func (p *Parameter) GetValues() []Value { return nil }

func (p *Parameter) GetUsers() []User { return p.user }

func (p *Parameter) AddUser(u User)    { p.user = append(p.user, u) }
func (p *Parameter) RemoveUser(u User) { p.user = utils.Remove(p.user, u) }

// ----------- IF
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v {
		i.Cond = to
	} else {
		panic("if not use this value")
	}
}

func (i *If) GetUsers() []User { return nil }

func (i *If) GetValues() []Value { return []Value{i.Cond} }
func (i *If) AddValue(v Value)   {}

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

func (l *Loop) GetValues() []Value { return []Value{l.Cond, l.Step, l.Init} }
func (l *Loop) AddValue(v Value)   {}

// ----------- Return
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v); index > -1 {
		r.Results[index] = to
	} else {
		panic("return not use this value")
	}
}

func (r *Return) GetUsers() []User { return nil }

func (r *Return) GetValues() []Value { return r.Results }
func (r *Return) AddValue(v Value)   {}

// ----------- Call
func (c *Call) ReplaceValue(v Value, to Value) {
	if index := slices.Index(c.Args, v); index > -1 {
		c.Args[index] = to
	} else {
		panic("return not use this value")
	}
}

func (c *Call) GetUsers() []User { return c.user }
func (c *Call) AddUser(u User)   { c.user = append(c.user, u) }

func (c *Call) RemoveUser(u User) { c.user = utils.Remove(c.user, u) }

func (c *Call) GetValues() []Value { return append(c.Args, append(c.binding, c.Method)...) }
func (c *Call) AddValue(v Value)   {}

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
func (sw *Switch) AddValue(v Value) {}

// ----------- BinOp
func (b *BinOp) ReplaceValue(v Value, to Value) {
	if b.X == v {
		b.X = to
	}

	if b.Y == v {
		b.Y = to
	}
}
func (b *BinOp) GetUsers() []User { return b.user }
func (b *BinOp) AddUser(u User)   { b.user = append(b.user, u) }

func (b *BinOp) RemoveUser(u User) { b.user = utils.Remove(b.user, u) }

func (b *BinOp) GetValues() []Value { return []Value{b.X, b.Y} }
func (b *BinOp) AddValue(v Value)   {}

// ----------- UnOp

func (u *UnOp) ReplaceValue(v Value, to Value) {
	if u.X == v {
		u.X = to
	} else {
		panic("unop not use this value")
	}
}

func (b *UnOp) GetUsers() []User { return b.user }
func (b *UnOp) AddUser(u User)   { b.user = append(b.user, u) }

func (b *UnOp) RemoveUser(u User) { b.user = utils.Remove(b.user, u) }

func (b *UnOp) GetValues() []Value { return []Value{b.X} }
func (b *UnOp) AddValue(v Value)   {}

// ----------- Interface
func (i *Interface) ReplaceValue(v, to Value) {
	if i.Cap == v {
		i.Cap = to
	} else if i.Len == v {
		i.Len = v
	} else {
		panic("interface not use this value")
	}
}

func (i *Interface) GetUsers() []User { return i.users }
func (i *Interface) AddUser(u User) {
	i.users = append(i.users, u)
}

func (i *Interface) RemoveUser(u User) {
	i.users = utils.Remove(i.users, u)
}

func (i *Interface) GetValues() []Value {
	vs := lo.MapToSlice(i.Field, func(key Value, f *Field) Value { return f })
	return vs
}
func (i *Interface) AddValue(v Value) {
	if f, ok := v.(*Field); ok {
		i.Field[f.Key] = f
	}
}

// ----------- Field
func (f *Field) ReplaceValue(v, to Value) {
	if f.Key == v {
		f.Key = to
	} else if index := slices.Index(f.Update, v); index > -1 {
		f.Update[index] = to
	} else {
		panic("field not use this value")
	}
}

func (f *Field) GetUsers() []User  { return append(f.users, f.I) }
func (f *Field) AddUser(u User)    { f.users = append(f.users, u) }
func (f *Field) RemoveUser(u User) { f.users = utils.Remove(f.users, u) }

func (f *Field) GetValues() []Value { return append(f.Update, f.Key) }
func (f *Field) AddValue(v Value) {
	if s, ok := v.(*Update); ok {
		f.Update = append(f.Update, s)
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

func (s *Update) GetValues() []Value { return []Value{s.Value} }
func (s *Update) AddValue(_ Value)   {}
