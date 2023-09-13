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
		}
	}
	if IsConst(b.Y) {
		// y const
	}

	return b
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
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() == y.Number())
		}
	case OpNotEq:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() != y.Number())
		}
	}
	return nil
}
