package ssa

func HandlerBinOp(b *BinOp) Value {
	if IsConst(b.X) {
		if IsConst(b.Y) {
			// both const
			if v := CalcConstBinary(ToConst(b.X), ToConst(b.Y), b.Op); v != nil {
				return v
			}

		} else {
			// x const
			if v := CalcConstBinarySide(ToConst(b.X), b.Y, b.Op); v != nil {
				return v
			}
		}
	}
	if IsConst(b.Y) {
		// y const
		if v := CalcConstBinarySide(ToConst(b.Y), b.X, b.Op); v != nil {
			return v
		}
	}

	// both not const

	return CalcBinary(b)
}

func HandlerUnOp(u *UnOp) Value {
	if IsConst(u.X) {
		if v := CalcConstUnary(ToConst(u.X), u.Op); v != nil {
			return v
		}
	}
	return u
}

func CalcBinary(b *BinOp) Value {
	isNot := func(x, y Value) bool {
		if u, ok := x.(*UnOp); ok {
			if u.X == y && u.Op == OpNot {
				return true
			}
		}
		return false
	}

	switch b.Op {
	case OpLogicOr:
		if isNot(b.X, b.Y) || isNot(b.Y, b.X) {
			// ~x || x
			return NewConst(true)
		}
	case OpLogicAnd:
		if isNot(b.X, b.Y) || isNot(b.Y, b.X) {
			// ~x && x
			return NewConst(false)
		}
	}
	return b
}

func CalcConstBinarySide(c *Const, v Value, op BinaryOpcode) Value {
	switch op {
	case OpLogicAnd:
		if c.IsBoolean() {
			if c.Boolean() {
				// true & A => A
				return v
			} else {
				// false & A => false
				return c
			}
		}
	case OpLogicOr:
		if c.IsBoolean() {
			if c.Boolean() {
				// true || A => true
				return c
			} else {
				// false || A => A
				return v
			}
		}
	case OpMul:
		if c.IsNumber() && c.Number() == 1 {
			// A * 1 => A
			return v
		}
	}
	return nil
}

func CalcConstBinary(x, y *Const, op BinaryOpcode) *Const {
	switch op {
	case OpShl:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() << y.Number())
		}
	case OpShr:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() >> y.Number())
		}
	case OpAnd:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() & y.Number())
		} else if x.IsBoolean() && y.IsBoolean() {
			return NewConst(x.Boolean() && y.Boolean())
		}
	case OpAndNot:

	case OpOr:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() | y.Number())
		} else if x.IsBoolean() && y.IsBoolean() {
			return NewConst(x.Boolean() || y.Boolean())
		}
	case OpXor:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() ^ y.Number())
		}
	case OpAdd:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() + y.Number())
		}
		if x.IsString() && y.IsString() {
			return NewConst(x.VarString() + y.VarString())
		}
	case OpSub:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() - y.Number())
		}
	case OpDiv:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() / y.Number())
		}
	case OpMod:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() % y.Number())
		}
	case OpMul:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() * y.Number())
		}
	case OpGt:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() > y.Number())
		}
	case OpLt:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() < y.Number())
		}
	case OpGtEq:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() >= y.Number())
		}
	case OpLtEq:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() <= y.Number())
		}
	case OpEq:
		if x.GetTypeKind() == y.GetTypeKind() {
			return NewConst(x.value == y.value)
		}
	case OpNotEq:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() != y.Number())
		}
	}
	return nil
}

// OpNone UnaryOpcode = iota
// OpNot              // !
// OpPlus             // +
// OpNeg              // -
// OpChan             // ->
func CalcConstUnary(x *Const, op UnaryOpcode) *Const {
	switch op {
	case OpNone:
		return x
	case OpNot:
		if x.IsBoolean() {
			return NewConst(!x.Boolean())
		}
	case OpPlus:
		if x.IsNumber() {
			return NewConst(+x.Number())
		}
	case OpNeg:
		if x.IsNumber() {
			return NewConst(-x.Number())
		}
	case OpChan:
	}

	return nil
}
