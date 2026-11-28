package lowir2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// lowerIntrinsic translates architecture-specific data-processing intrinsics
// into ordinary YakSSA arithmetic and bitwise operations when the LowIR operands
// satisfy exact semantic preconditions. Unrecognized or unsound forms fall back
// to opaque intrinsic markers.
func (c *converter) lowerIntrinsic(name string, instruction core.Instruction) (ssa.Value, bool) {
	switch name {
	case "x86.rol":
		return c.lowerRotate(instruction, true)
	case "x86.ror", "arm64.ror", "arm64.rorv":
		return c.lowerRotate(instruction, false)
	case "arm64.extr":
		return c.lowerARM64Extr(instruction)
	case "x86.div.quot":
		return c.lowerX86Div(instruction, false, false)
	case "x86.div.rem":
		return c.lowerX86Div(instruction, false, true)
	case "x86.idiv.quot":
		return c.lowerX86Div(instruction, true, false)
	case "x86.idiv.rem":
		return c.lowerX86Div(instruction, true, true)
	case "arm64.ubfx", "arm64.ubfiz", "arm64.sbfx", "arm64.sbfiz":
		return c.lowerARM64Bitfield(name, instruction)
	default:
		return nil, false
	}
}

func (c *converter) intrinsicBits(instruction core.Instruction) uint16 {
	if len(instruction.Results) > 0 && instruction.Results[0].Type.Bits > 0 {
		return instruction.Results[0].Type.Bits
	}
	if bits := c.instructionWidth(instruction); bits > 0 {
		return bits
	}
	return 64
}

func (c *converter) emitConstTyped(value uint64, typ ssa.Type) ssa.Value {
	constant := c.builder.EmitConstInst(int64(value))
	if constant != nil && typ != nil {
		constant.SetType(typ)
	}
	return constant
}

func (c *converter) emitMasked(value ssa.Value, bits uint16) ssa.Value {
	if value == nil || bits == 0 || bits >= 64 {
		return value
	}
	mask := (uint64(1) << bits) - 1
	utyp := c.typeFor(integerType(bits))
	return c.typedBinOp(ssa.OpAnd, c.normalizeUnsigned(value, bits), c.emitConstTyped(mask, utyp), integerType(bits))
}

func (c *converter) lowerRotate(instruction core.Instruction, left bool) (ssa.Value, bool) {
	if len(instruction.Operands) < 3 {
		return nil, false
	}
	bits := c.intrinsicBits(instruction)
	if bits == 0 || bits > 64 || (bits&(bits-1)) != 0 {
		return nil, false
	}
	val := c.operandValue(instruction.Operands[1])
	count := c.operandValue(instruction.Operands[2])
	if val == nil || count == nil || val.IsUndefined() || count.IsUndefined() {
		return nil, false
	}
	return c.emitRotate(val, count, bits, left)
}

func (c *converter) lowerARM64Extr(instruction core.Instruction) (ssa.Value, bool) {
	// arm64.extr has [sym, rn, rm, lsb]; when rn == rm it is a pure rotate right by lsb.
	if len(instruction.Operands) < 4 || instruction.Operands[1].Kind != core.OperandValue || instruction.Operands[2].Kind != core.OperandValue {
		return nil, false
	}
	if instruction.Operands[1].Value != instruction.Operands[2].Value {
		return nil, false
	}
	bits := c.intrinsicBits(instruction)
	if bits == 0 || bits > 64 || (bits&(bits-1)) != 0 {
		return nil, false
	}
	val := c.operandValue(instruction.Operands[1])
	count := c.operandValue(instruction.Operands[3])
	if val == nil || count == nil || val.IsUndefined() || count.IsUndefined() {
		return nil, false
	}
	return c.emitRotate(val, count, bits, false)
}

func (c *converter) emitRotate(val, count ssa.Value, bits uint16, left bool) (ssa.Value, bool) {
	uval := c.normalizeUnsigned(val, bits)
	if uval == nil {
		return nil, false
	}
	utype := integerType(bits)
	ssaUType := c.typeFor(utype)
	mask := ^uint64(0)
	if bits < 64 {
		mask = (uint64(1) << bits) - 1
	}

	if countConst, ok := ssa.ToConstInst(count); ok {
		cval, ok := unsignedConstant(countConst, bits)
		if !ok {
			if raw, okRaw := constInteger(countConst); okRaw {
				cval, ok = uint64(raw), true
			}
		}
		if ok {
			shift := cval % uint64(bits)
			if shift == 0 {
				return uval, true
			}
			if valConst, okVal := ssa.ToConstInst(uval); okVal {
				if ua, okU := unsignedConstant(valConst, bits); okU {
					var res uint64
					if left {
						res = ((ua << shift) | (ua >> (uint64(bits) - shift))) & mask
					} else {
						res = ((ua >> shift) | (ua << (uint64(bits) - shift))) & mask
					}
					return c.emitConstTyped(res, ssaUType), true
				}
			}
			inv := uint64(bits) - shift
			s1, s2 := shift, inv
			if !left {
				s1, s2 = inv, shift
			}
			hi := c.typedBinOp(ssa.OpShl, uval, c.emitConstTyped(s1, ssaUType), utype)
			if bits < 64 {
				hi = c.emitMasked(hi, bits)
			}
			lo := c.typedBinOp(ssa.OpShr, uval, c.emitConstTyped(s2, ssaUType), utype)
			out := c.typedBinOp(ssa.OpOr, hi, lo, utype)
			return out, out != nil
		}
	}

	// Dynamic count: mask with (bits - 1) and use (-c) & (bits - 1) so count == 0 shifts by 0 without UB.
	shiftMask := uint64(bits - 1)
	ucount := c.typedBinOp(ssa.OpAnd, c.normalizeUnsigned(count, bits), c.emitConstTyped(shiftMask, ssaUType), utype)
	zero := c.emitConstTyped(0, ssaUType)
	negCount := c.typedBinOp(ssa.OpSub, zero, ucount, utype)
	invCount := c.typedBinOp(ssa.OpAnd, negCount, c.emitConstTyped(shiftMask, ssaUType), utype)
	lAmt, rAmt := ucount, invCount
	if !left {
		lAmt, rAmt = invCount, ucount
	}
	hi := c.typedBinOp(ssa.OpShl, uval, lAmt, utype)
	if bits < 64 {
		hi = c.emitMasked(hi, bits)
	}
	lo := c.typedBinOp(ssa.OpShr, uval, rAmt, utype)
	out := c.typedBinOp(ssa.OpOr, hi, lo, utype)
	return out, out != nil
}

func (c *converter) lowerX86Div(instruction core.Instruction, signed, remainder bool) (ssa.Value, bool) {
	// Operands: [symbol, high(dx/rdx), low(ax/rax), divisor].
	if len(instruction.Operands) < 4 {
		return nil, false
	}
	bits := c.intrinsicBits(instruction)
	if bits == 0 || bits > 64 {
		return nil, false
	}
	high := c.operandValue(instruction.Operands[1])
	low := c.operandValue(instruction.Operands[2])
	divisor := c.operandValue(instruction.Operands[3])
	if high == nil || low == nil || divisor == nil || high.IsUndefined() || low.IsUndefined() || divisor.IsUndefined() {
		return nil, false
	}
	if signed {
		if !c.isSignExtensionOf(high, low, bits) {
			return nil, false
		}
	} else {
		if !c.isZeroConstantValue(high, bits) {
			return nil, false
		}
	}
	var op ssa.BinaryOpcode = ssa.OpDiv
	if remainder {
		op = ssa.OpMod
	}
	var l, r ssa.Value
	var resultType ssa.Type
	if signed {
		l, r = c.normalizeSigned(low, bits), c.normalizeSigned(divisor, bits)
		resultType = c.signedTypeFor(bits)
	} else {
		l, r = c.normalizeUnsigned(low, bits), c.normalizeUnsigned(divisor, bits)
		resultType = c.typeFor(integerType(bits))
	}
	if lc, lok := ssa.ToConstInst(l); lok {
		if rc, rok := ssa.ToConstInst(r); rok {
			if folded, ok := orderedBinaryFolds(op, signed, lc, rc, bits); ok {
				folded = truncateSigned(folded, bits)
				res := c.builder.EmitConstInst(folded)
				if res != nil {
					res.SetType(resultType)
				}
				return res, res != nil
			}
		}
	}
	res := c.builder.EmitBinOp(op, l, r)
	if res == nil {
		return nil, false
	}
	res.SetType(resultType)
	return res, true
}

func (c *converter) unwrapCastsAndMask(v ssa.Value, bits uint16) ssa.Value {
	fn := c.function
	for depth := 0; depth < 8 && v != nil; depth++ {
		if cast, ok := ssa.ToTypeCast(v); ok && cast != nil && cast.Value != 0 {
			if prev, exists := fn.GetValueById(cast.Value); exists && prev != nil {
				v = prev
				continue
			}
		}
		if bin, ok := ssa.ToBinOp(v); ok && bin.Op == ssa.OpAnd {
			mask := ^uint64(0)
			if bits > 0 && bits < 64 {
				mask = (uint64(1) << bits) - 1
			}
			lv, _ := fn.GetValueById(bin.X)
			rv, _ := fn.GetValueById(bin.Y)
			if rc, okC := ssa.ToConstInst(rv); okC {
				if cval, okU := constInteger(rc); okU && uint64(cval) == mask && lv != nil {
					v = lv
					continue
				}
			}
			if lc, okC := ssa.ToConstInst(lv); okC {
				if cval, okU := constInteger(lc); okU && uint64(cval) == mask && rv != nil {
					v = rv
					continue
				}
			}
		}
		break
	}
	return v
}

// unwrapPartialInsert recovers the inserted low bits from a partial-register write
// pattern: Or(AndNot(_, mask), And(inserted, mask)) -> inserted.
func (c *converter) unwrapPartialInsert(v ssa.Value) ssa.Value {
	bin, ok := ssa.ToBinOp(v)
	if !ok || bin.Op != ssa.OpOr {
		return v
	}
	fn := c.function
	right, okR := fn.GetValueById(bin.Y)
	if !okR || right == nil {
		return v
	}
	if masked, okAnd := ssa.ToBinOp(right); okAnd && masked.Op == ssa.OpAnd {
		if inner, okInner := fn.GetValueById(masked.X); okInner && inner != nil {
			return inner
		}
	}
	return v
}

func (c *converter) isZeroConstantValue(v ssa.Value, bits uint16) bool {
	v = c.unwrapCastsAndMask(c.unwrapPartialInsert(v), bits)
	if cnst, ok := ssa.ToConstInst(v); ok {
		if num, okI := constInteger(cnst); okI {
			if bits > 0 && bits < 64 {
				return (uint64(num) & ((uint64(1) << bits) - 1)) == 0
			}
			return num == 0
		}
	}
	return false
}

// isSignExtensionOf verifies whether high equals low arithmetically shifted right
// by (bits - 1), matching CDQ/CQO/CWD or explicit sign extension of the low register.
func (c *converter) isSignExtensionOf(high, low ssa.Value, bits uint16) bool {
	if bits < 2 {
		return false
	}
	h := c.unwrapCastsAndMask(c.unwrapPartialInsert(high), bits)
	shiftBin, ok := ssa.ToBinOp(h)
	if !ok || shiftBin.Op != ssa.OpShr {
		return false
	}
	fn := c.function
	srcVal, okS := fn.GetValueById(shiftBin.X)
	amtVal, okA := fn.GetValueById(shiftBin.Y)
	if !okS || !okA || srcVal == nil || amtVal == nil {
		return false
	}
	if !isSignedIntegerType(srcVal.GetType()) {
		return false
	}
	amtConst, okC := ssa.ToConstInst(amtVal)
	if !okC {
		return false
	}
	amtNum, okN := constInteger(amtConst)
	if !okN || uint64(amtNum) != uint64(bits-1) {
		return false
	}
	return c.sameSourceValue(srcVal, low, bits)
}

func (c *converter) sameSourceValue(a, b ssa.Value, bits uint16) bool {
	ua := c.unwrapCastsAndMask(c.unwrapPartialInsert(a), bits)
	ub := c.unwrapCastsAndMask(c.unwrapPartialInsert(b), bits)
	if ua == nil || ub == nil {
		return false
	}
	if ua.GetId() == ub.GetId() {
		return true
	}
	return ua == ub
}

func (c *converter) lowerARM64Bitfield(name string, instruction core.Instruction) (ssa.Value, bool) {
	if len(instruction.Operands) < 4 {
		return nil, false
	}
	bits := c.intrinsicBits(instruction)
	if bits == 0 || bits > 64 {
		return nil, false
	}
	src := c.operandValue(instruction.Operands[1])
	lsbVal := c.operandValue(instruction.Operands[2])
	widthVal := c.operandValue(instruction.Operands[3])
	if src == nil || lsbVal == nil || widthVal == nil || src.IsUndefined() {
		return nil, false
	}
	lsbConst, okL := ssa.ToConstInst(lsbVal)
	widthConst, okW := ssa.ToConstInst(widthVal)
	if !okL || !okW {
		return nil, false
	}
	lsb, ok1 := constInteger(lsbConst)
	width, ok2 := constInteger(widthConst)
	if !ok1 || !ok2 || lsb < 0 || width <= 0 || uint64(lsb)+uint64(width) > uint64(bits) {
		return nil, false
	}
	lsbU := uint64(lsb)
	widthU := uint64(width)
	utype := integerType(bits)
	ssaUType := c.typeFor(utype)
	fullMask := ^uint64(0)
	if bits < 64 {
		fullMask = (uint64(1) << bits) - 1
	}

	switch strings.TrimPrefix(name, "arm64.") {
	case "ubfx":
		if sc, okS := ssa.ToConstInst(c.normalizeUnsigned(src, bits)); okS {
			if sv, okV := unsignedConstant(sc, bits); okV {
				res := (sv >> lsbU) & ((uint64(1)<<widthU - 1) | func() uint64 {
					if widthU == 64 {
						return ^uint64(0)
					}
					return 0
				}())
				return c.emitConstTyped(res, ssaUType), true
			}
		}
		v := c.normalizeUnsigned(src, bits)
		if lsbU > 0 {
			v = c.typedBinOp(ssa.OpShr, v, c.emitConstTyped(lsbU, ssaUType), utype)
		}
		if widthU < uint64(bits) {
			mask := (uint64(1) << widthU) - 1
			v = c.typedBinOp(ssa.OpAnd, v, c.emitConstTyped(mask, ssaUType), utype)
		}
		return v, v != nil

	case "ubfiz":
		v := c.normalizeUnsigned(src, bits)
		if widthU < uint64(bits) {
			mask := (uint64(1) << widthU) - 1
			v = c.typedBinOp(ssa.OpAnd, v, c.emitConstTyped(mask, ssaUType), utype)
		}
		if lsbU > 0 {
			v = c.typedBinOp(ssa.OpShl, v, c.emitConstTyped(lsbU, ssaUType), utype)
			if bits < 64 {
				v = c.emitMasked(v, bits)
			}
		}
		return v, v != nil

	case "sbfx":
		stype := c.signedTypeFor(bits)
		top := uint64(bits) - lsbU - widthU
		down := uint64(bits) - widthU
		v := c.normalizeUnsigned(src, bits)
		if top > 0 {
			v = c.typedBinOp(ssa.OpShl, v, c.emitConstTyped(top, ssaUType), utype)
			if bits < 64 {
				v = c.emitMasked(v, bits)
			}
		}
		sv := c.normalizeSigned(v, bits)
		if down > 0 {
			sv = c.builder.EmitBinOp(ssa.OpShr, sv, c.emitConstTyped(down, stype))
			if sv == nil {
				return nil, false
			}
			sv.SetType(stype)
		}
		return sv, sv != nil

	case "sbfiz":
		stype := c.signedTypeFor(bits)
		top := uint64(bits) - widthU
		v := c.normalizeUnsigned(src, bits)
		if top > 0 {
			v = c.typedBinOp(ssa.OpShl, v, c.emitConstTyped(top, ssaUType), utype)
			if bits < 64 {
				v = c.emitMasked(v, bits)
			}
		}
		sv := c.normalizeSigned(v, bits)
		if top > 0 {
			sv = c.builder.EmitBinOp(ssa.OpShr, sv, c.emitConstTyped(top, stype))
			if sv == nil {
				return nil, false
			}
			sv.SetType(stype)
		}
		if lsbU > 0 {
			uv := c.normalizeUnsigned(sv, bits)
			uv = c.typedBinOp(ssa.OpShl, uv, c.emitConstTyped(lsbU, ssaUType), utype)
			if bits < 64 {
				_ = fullMask
				uv = c.emitMasked(uv, bits)
			}
			sv = c.normalizeSigned(uv, bits)
		}
		return sv, sv != nil
	}
	return nil, false
}
