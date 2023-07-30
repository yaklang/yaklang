package ssa

import "golang.org/x/exp/slices"

func ReplaceValue(v Value, to Value) {
	for _, user := range v.GetUser() {
		user.ReplaceValue(v, to)
	}
}

// ----------- Function
func (f *Function) GetUser() []User { return f.user }

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

func (b *BinOp) AddValue(v Value)  {}
func (b *BinOp) GetValue() []Value { return []Value{b.X, b.Y} }

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

// ----------- Jump
func (j *Jump) ReplaceValue(v Value, to Value) {
	panic("jump don't use value")
}

func (j *Jump) GetUser() []User { return nil }
func (j *Jump) AddUser(u User)  {}

func (j *Jump) GetValue() []Value { return nil }
func (j *Jump) AddValue(u Value)  {}
