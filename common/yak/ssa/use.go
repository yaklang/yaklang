package ssa

import (
	"golang.org/x/exp/slices"
)

func ReplaceValue(v Value, to Value) {
	for _, user := range v.GetUsers() {
		user.ReplaceValue(v, to)
		to.AddUser(user)
	}
}

// ----------- Function
func (f *Function) GetUsers() []User { return f.user }
func (f *Function) AddUser(u User)   { f.user = append(f.user, u) }

// ----------- BasicBlock
func (b *BasicBlock) GetUsers() []User { return b.user }
func (b *BasicBlock) AddUser(u User)   { b.user = append(b.user, u) }

// ----------- Phi
func (p *Phi) ReplaceValue(v Value, to Value) {
	slices.Replace(p.Edge, 0, len(p.Edge), v, to)
}

func (p *Phi) GetUsers() []User { return p.user }
func (p *Phi) AddUser(u User)   { p.user = append(p.user, u) }

func (p *Phi) GetValues() []Value { return p.Edge }
func (p *Phi) AddValue(v Value)   {}

// ----------- Const
func (c *Const) GetUsers() []User { return c.user }
func (c *Const) AddUser(u User)   { c.user = append(c.user, u) }

// ----------- param
func (p *Parameter) GetUsers() []User {
	return p.user
}

func (p *Parameter) AddUser(u User) {
	p.user = append(p.user, u)
}

// ----------- Alloc
func (a *Alloc) GetUsers() []User { return a.user }
func (a *Alloc) AddUser(u User)   { a.user = append(a.user, u) }

func (a *Alloc) GetValues() []Value            { return nil }
func (a *Alloc) AddValue(_ Value)              {}
func (a *Alloc) ReplaceValue(_ Value, _ Value) {}

// ----------- Store
func (s *Store) GetUsers() []User { return nil }
func (s *Store) AddUser(u User)   {}

func (s *Store) GetValues() []Value { return []Value{s.value, s.alloc} }
func (s *Store) AddValue(v Value)   {}
func (s *Store) ReplaceValue(v Value, to Value) {
	if s.value == v {
		s.value = to
	} else if s.alloc == v {
		if to, ok := to.(*Alloc); ok {
			s.alloc = to
		}
		panic("load replace to value is not alloc")
	} else {
		panic("store not use this value")
	}
}

// ----------- Load
func (a *Load) GetUsers() []User { return a.user }
func (a *Load) AddUser(u User)   { a.user = append(a.user, u) }

func (a *Load) GetValues() []Value { return []Value{a.alloc} }
func (a *Load) AddValue(_ Value)   {}
func (a *Load) ReplaceValue(v Value, to Value) {
	if v == a.alloc {
		if to, ok := to.(*Alloc); ok {
			a.alloc = to
		}
		panic("load replace to value is not alloc")
	}
	panic("load replace v not load target")
}

// ----------- Jump
func (j *Jump) ReplaceValue(v Value, to Value) {
	panic("jump don't use value")
}

func (j *Jump) GetUsers() []User { return nil }
func (j *Jump) AddUser(u User)   {}

func (j *Jump) GetValues() []Value { return nil }
func (j *Jump) AddValue(u Value)   {}

// ----------- IF
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v {
		i.Cond = to
	} else {
		panic("if not use this value")
	}
}

func (i *If) GetUsers() []User { return i.user }
func (i *If) AddUser(u User)   { i.user = append(i.user, u) }

func (i *If) GetValues() []Value { return []Value{i.Cond} }
func (i *If) AddValue(v Value)   {}

// ----------- Return
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v); index > 0 {
		r.Results[index] = to
	} else {
		panic("return not use this value")
	}
}

func (r *Return) GetUsers() []User { return nil }
func (r *Return) AddUser(u User)   {}

func (r *Return) GetValues() []Value { return r.Results }
func (r *Return) AddValue(v Value)   {}

// ----------- Call
func (c *Call) ReplaceValue(v Value, to Value) {
	if index := slices.Index(c.Args, v); index > 0 {
		c.Args[index] = to
	} else {
		panic("return not use this value")
	}
}

func (c *Call) GetUsers() []User { return c.user }
func (c *Call) AddUser(u User)   { c.user = append(c.user, u) }

func (c *Call) GetValues() []Value { return c.Args }
func (c *Call) AddValue(v Value)   {}

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

func (b *BinOp) GetValues() []Value { return []Value{b.X, b.Y} }
func (b *BinOp) AddValue(v Value)   {}

// ----------- MakeClosure
func (m *Closure) ReplaceValue(v Value, to Value) {
	if index := slices.Index(m.Bindings, v); index > 0 {
		m.Bindings[index] = to
	} else {
		panic("makeclosure not use this value")
	}
}

func (m *Closure) GetUsers() []User { return m.user }
func (m *Closure) AddUser(u User)   { m.user = append(m.user, u) }

func (m *Closure) GetValues() []Value { return append(m.Bindings, m.Fn) }
func (m *Closure) AddValue(v Value)   {}
