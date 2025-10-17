package ssa

import (
	"math"
)

const (
	OpNone = ``
)

type BinaryOpcode string

const (
	OpLogicAnd = `LogicAnd`
	OpLogicOr  = `LogicOr`
	OpAnd      = `and`
	OpAndNot   = `and-not`
	OpOr       = `or`
	OpXor      = `xor`
	OpShl      = `shl`
	OpShr      = `shr`
	OpAdd      = `add`
	OpSub      = `sub`
	OpMod      = `mod`
	OpMul      = `mul`
	OpDiv      = `div`
	OpGt       = `gt`
	OpLt       = `lt`
	OpLtEq     = `ltEq`
	OpGtEq     = `gtEq`
	OpNotEq    = `neq`
	OpEq       = `eq`
	OpIn       = `in`
	OpSend     = `send`
	OpPow      = `pow`
)

var CompareOpcode = []string{OpGt, OpLt, OpLtEq, OpGtEq, OpNotEq, OpEq}

func IsCompareOpcode(i BinaryOpcode) bool {
	for _, v := range CompareOpcode {
		if v == string(i) {
			return true
		}
	}
	return false
}

type UnaryOpcode string

const (
	OpNot        = `not`
	OpPlus       = `plus`
	OpNeg        = `neg`
	OpChan       = `chan`
	OpBitwiseNot = `bitwise-not`
)

func HandlerBinOp(b *BinOp) (ret Value) {
	defer func() {
		if c, ok := ToConstInst(ret); ok {
			c.Origin = b.GetId()
		}
	}()

	x, ok := b.GetValueById(b.X)
	if !ok || x == nil {
		return CalcBinary(b)
	}
	y, ok := b.GetValueById(b.Y)
	if !ok || y == nil {
		return CalcBinary(b)
	}
	if cX, ok := ToConstInst(x); ok {
		if cY, ok := ToConstInst(y); ok {
			// both const
			if v := CalcConstBinary(cX, cY, b.Op); v != nil {
				return v
			}

		} else {
			// x const
			if v := CalcConstBinarySide(cX, y, b.Op); v != nil {
				return v
			}
		}
	}
	if c, ok := ToConstInst(y); ok {
		// y const
		if v := CalcConstBinarySide(c, x, b.Op); v != nil {
			return v
		}
	}

	// both not const

	return CalcBinary(b)
}

func HandlerUnOp(u *UnOp) (ret Value) {
	defer func() {
		if c, ok := ToConstInst(ret); ok {
			c.Origin = u.GetId()
		}
	}()

	x, ok := u.GetValueById(u.X)
	if !ok || x == nil {
		return u
	}
	if c, ok := ToConstInst(x); ok {
		if v := CalcConstUnary(c, u.Op); v != nil {
			return v
		}
	}
	return u
}

func CalcBinary(b *BinOp) Value {
	isNot := func(xid, yid int64) bool {
		x, ok := b.GetValueById(xid)
		if !ok || x == nil {
			return false
		}
		y, ok := b.GetValueById(yid)
		if !ok || y == nil {
			return false
		}
		if u, ok := x.(*UnOp); ok {
			if u.X == y.GetId() && u.Op == OpNot {
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
	case OpPow:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(math.Pow(float64(x.Number()), float64(y.Number())))
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
		if x.IsFloat() && y.IsFloat() {
			return NewConst(x.Float() + y.Float())
		}
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() + y.Number())
		}
		if x.IsString() && y.IsString() {
			return NewConst(x.VarString() + y.VarString())
		}
	case OpSub:
		if x.IsFloat() && y.IsFloat() {
			return NewConst(x.Float() - y.Float())
		}
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() - y.Number())
		}
	case OpMul:
		if x.IsFloat() && y.IsFloat() {
			return NewConst(x.Float() * y.Float())
		}
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() * y.Number())
		}
	case OpDiv:
		if x.IsFloat() && y.IsFloat() {
			if x.Float() == 0 || y.Float() == 0 {
				return NewConst(0)
			}
			return NewConst(x.Float() / y.Float())
		}
		if x.IsNumber() && y.IsNumber() {
			if x.Number() == 0 || y.Number() == 0 {
				return NewConst(0)
			}
			return NewConst(x.Number() / y.Number())
		}
	case OpMod:
		if x.IsNumber() && y.IsNumber() {
			return NewConst(x.Number() % y.Number())
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
	case OpBitwiseNot:
		if x.IsNumber() {
			return NewConst(^x.Number())
		}
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
			return NewConst(-(x.Number()))
		}
	case OpChan:
	}

	return nil
}
