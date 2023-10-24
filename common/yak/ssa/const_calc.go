package ssa

type BinaryOpcode int

const (
	// Binary
	OpShl BinaryOpcode = iota // <<

	OpLogicAnd // &&
	OpLogicOr  // ||

	OpShr    // >>
	OpAnd    // &
	OpAndNot // &^
	OpOr     // |
	OpXor    // ^
	OpAdd    // +
	OpSub    // -
	OpDiv    // /
	OpMod    // %
	// mul
	OpMul // *

	// boolean opcode
	OpGt    // >
	OpLt    // <
	OpGtEq  // >=
	OpLtEq  // <=
	OpEq    // ==
	OpNotEq // != <>
	OpIn    //  a in b

	OpSend // <-
)

var BinaryOpcodeName = map[BinaryOpcode]string{
	OpLogicAnd: `&&`,
	OpLogicOr:  `||`,

	OpAnd:    `and`,
	OpAndNot: `and-not`,
	OpOr:     `or`,
	OpXor:    `xor`,
	OpShl:    `shl`,
	OpShr:    `shr`,
	OpAdd:    `add`,
	OpSub:    `sub`,
	OpMod:    `mod`,
	OpMul:    `mul`,
	OpDiv:    `div`,
	OpGt:     `gt`,
	OpLt:     `lt`,
	OpLtEq:   `lt-eq`,
	OpGtEq:   `gt-eq`,
	OpNotEq:  `neq`,
	OpEq:     `eq`,
	OpIn:     `in`,
	OpSend:   `send`,
}

type UnaryOpcode int

const (
	OpNone       UnaryOpcode = iota
	OpNot                    // !
	OpPlus                   // +
	OpNeg                    // -
	OpChan                   // <-
	OpBitwiseNot             // ^
)

var UnaryOpcodeName = map[UnaryOpcode]string{
	OpNone:       ``,
	OpNot:        `not`,
	OpPlus:       `plus`,
	OpNeg:        `neg`,
	OpChan:       `chan`,
	OpBitwiseNot: `bitwise-not`,
}

func HandlerBinOp(b *BinOp) Value {
	if cX, ok := ToConst(b.X); ok {
		if cY, ok := ToConst(b.Y); ok {
			// both const
			if v := CalcConstBinary(cX, cY, b.Op); v != nil {
				return v
			}

		} else {
			// x const
			if v := CalcConstBinarySide(cX, b.Y, b.Op); v != nil {
				return v
			}
		}
	}
	if c, ok := ToConst(b.Y); ok {
		// y const
		if v := CalcConstBinarySide(c, b.X, b.Op); v != nil {
			return v
		}
	}

	// both not const

	return CalcBinary(b)
}

func HandlerUnOp(u *UnOp) Value {
	if c, ok := ToConst(u.X); ok {
		if v := CalcConstUnary(c, u.Op); v != nil {
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

func CalcConstBinarySide(c *ConstInst, v Value, op BinaryOpcode) Value {
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

func CalcConstBinary(x, y *ConstInst, op BinaryOpcode) *ConstInst {
	switch op {
	case OpShl:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() << y.Number())
		}
	case OpShr:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() >> y.Number())
		}
	case OpAnd, OpLogicAnd:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() & y.Number())
		} else if x.IsBoolean() && y.IsBoolean() {
			return NewConst(x.Boolean() && y.Boolean())
		} else if x.IsBoolean() && y.IsNumber() {
			return NewConst(x.Boolean() && y.Number() != 0)
		} else if x.IsNumber() && y.IsBoolean() {
			return NewConst(x.Number() != 0 && y.Boolean())
		}
	case OpAndNot:

	case OpOr, OpLogicOr:
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
func CalcConstUnary(x *ConstInst, op UnaryOpcode) *ConstInst {
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
