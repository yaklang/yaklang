package ssa

import "golang.org/x/exp/slices"

func ReplaceValue(v Value, to Value) {
	for _, user := range v.GetUser() {
		user.ReplaceValue(v, to)
	}
}

// ----------- Function
func (f *Function) GetUser() []User { return f.user }
func (f *Function) AddUser(u User)  { f.user = append(f.user, u) }

// ----------- BasicBlock
func (b *BasicBlock) GetUser() []User { return b.user }
func (b *BasicBlock) AddUser(u User)  { b.user = append(b.user, u) }

// ----------- Phi
func (p *Phi) ReplaceValue(v Value, to Value) {
	slices.Replace(p.Edge, 0, len(p.Edge), v, to)
}

func (p *Phi) GetUser() []User { return p.user }
func (p *Phi) AddUser(u User)  { p.user = append(p.user, u) }

func (p *Phi) GetValue() []Value { return p.Edge }
func (p *Phi) AddValue(v Value)  {}

// ----------- Const
func (c *Const) GetUser() []User { return c.user }
func (c *Const) AddUser(u User)  { c.user = append(c.user, u) }

// ----------- param
func (p *Parameter) GetUser() []User {
	return p.user
}

func (p *Parameter) AddUser(u User) {
	p.user = append(p.user, u)
}

// ----------- Jump
func (j *Jump) ReplaceValue(v Value, to Value) {
	panic("jump don't use value")
}

func (j *Jump) GetUser() []User { return nil }
func (j *Jump) AddUser(u User)  {}

func (j *Jump) GetValue() []Value { return nil }
func (j *Jump) AddValue(u Value)  {}

// ----------- IF
func (i *If) ReplaceValue(v Value, to Value) {
	if i.Cond == v {
		i.Cond = to
	} else {
		panic("if not use this value")
	}
}

func (i *If) GetUser() []User { return i.user }
func (i *If) AddUser(u User)  { i.user = append(i.user, u) }

func (i *If) GetValue() []Value { return []Value{i.Cond} }
func (i *If) AddValue(v Value)  {}

// ----------- Return
func (r *Return) ReplaceValue(v Value, to Value) {
	if index := slices.Index(r.Results, v); index > 0 {
		r.Results[index] = to
	} else {
		panic("return not use this value")
	}
}

func (r *Return) GetUser() []User { return nil }
func (r *Return) AddUser(u User)  {}

func (r *Return) GetValue() []Value { return r.Results }
func (r *Return) AddValue(v Value)  {}

// ----------- Call
func (c *Call) ReplaceValue(v Value, to Value) {
	if index := slices.Index(c.Args, v); index > 0 {
		c.Args[index] = to
	} else {
		panic("return not use this value")
	}
}

func (c *Call) GetUser() []User { return c.user }
func (c *Call) AddUser(u User)  { c.user = append(c.user, u) }

func (c *Call) GetValue() []Value { return c.Args }
func (c *Call) AddValue(v Value)  {}

// ----------- BinOp
func (b *BinOp) ReplaceValue(v Value, to Value) {
	if b.X == v {
		b.X = to
	}

	if b.Y == v {
		b.Y = to
	}
}
func (b *BinOp) GetUser() []User { return b.user }
func (b *BinOp) AddUser(u User)  { b.user = append(b.user, u) }

func (b *BinOp) GetValue() []Value { return []Value{b.X, b.Y} }
func (b *BinOp) AddValue(v Value)  {}

// ----------- MakeClosure
func (m *MakeClosure) ReplaceValue(v Value, to Value) {
	if index := slices.Index(m.Bindings, v); index > 0 {
		m.Bindings[index] = to
	} else {
		panic("makeclosure not use this value")
	}
}

func (m *MakeClosure) GetUser() []User { return m.user }
func (m *MakeClosure) AddUser(u User)  { m.user = append(m.user, u) }

func (m *MakeClosure) GetValue() []Value { return append(m.Bindings, m.Fn) }

//TODO: this
func (m *MakeClosure) AddValue(v Value) {}
