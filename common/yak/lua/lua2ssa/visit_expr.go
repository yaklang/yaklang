package lua2ssa

import (
	"strconv"
	"strings"

	luaparser "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type luaAccess struct {
	value  ssa.Value
	name   string
	object ssa.Value
	key    ssa.Value
	member bool
}

func (b *singleFileBuilder) VisitExp(raw luaparser.IExpContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	exp, ok := raw.(*luaparser.ExpContext)
	if !ok || exp == nil {
		return nil
	}
	recoverRange := b.SetRange(exp)
	defer recoverRange()

	switch {
	case exp.Nil() != nil:
		return b.EmitConstInstNil()
	case exp.True() != nil:
		return b.EmitConstInst(true)
	case exp.False() != nil:
		return b.EmitConstInst(false)
	case exp.Number() != nil:
		return b.evalNumber(exp.Number())
	case exp.String_() != nil:
		return b.EmitConstInst(luaStringValue(exp.String_().GetText()))
	case exp.Ellipsis() != nil:
		if value := b.ReadValue("..."); value != nil {
			return value
		}
		return b.EmitUndefined("...")
	case exp.Functiondef() != nil:
		def, ok := exp.Functiondef().(*luaparser.FunctiondefContext)
		if !ok || def == nil {
			return b.EmitUndefined(exp.GetText())
		}
		return b.buildFunction("anonymous", def.Funcbody(), false)
	case exp.Prefixexp() != nil:
		return b.evalPrefixexp(exp.Prefixexp())
	case exp.Tableconstructor() != nil:
		return b.evalTable(exp.Tableconstructor())
	case exp.OperatorUnary() != nil:
		operand := b.visitExpAt(exp, 0)
		return b.evalUnary(exp.OperatorUnary().GetText(), operand)
	}

	if exp.OperatorAnd() != nil || exp.OperatorOr() != nil {
		left := b.visitExpAt(exp, 0)
		return b.BuildLuaLogical(left, func() ssa.Value {
			return b.visitExpAt(exp, 1)
		}, exp.OperatorAnd() != nil)
	}
	if len(exp.AllExp()) >= 2 {
		left := b.visitExpAt(exp, 0)
		right := b.visitExpAt(exp, 1)
		return b.evalBinary(exp, left, right)
	}
	return b.EmitUndefined(exp.GetText())
}

func (b *singleFileBuilder) visitExpAt(exp *luaparser.ExpContext, index int) ssa.Value {
	if exp == nil || index < 0 || index >= len(exp.AllExp()) {
		return nil
	}
	return b.VisitExp(exp.Exp(index))
}

func (b *singleFileBuilder) evalUnary(operator string, operand ssa.Value) ssa.Value {
	if operand == nil {
		operand = b.EmitConstInstNil()
	}
	switch operator {
	case "not":
		return b.EmitUnOp(ssa.OpNot, operand)
	case "-":
		return b.EmitUnOp(ssa.OpNeg, operand)
	case "~":
		return b.EmitUnOp(ssa.OpBitwiseNot, operand)
	case "#":
		// TODO(ssa-lua): runtime table/string length is not lowered; only constant strings are folded.
		if constant, ok := operand.(*ssa.ConstInst); ok {
			if text, ok := constant.GetRawValue().(string); ok {
				return b.EmitConstInst(int64(len(text)))
			}
		}
		return b.EmitUndefined("#")
	default:
		return b.EmitUndefined("lua-unary:" + operator)
	}
}

func (b *singleFileBuilder) evalBinary(exp *luaparser.ExpContext, left, right ssa.Value) ssa.Value {
	if left == nil {
		left = b.EmitConstInstNil()
	}
	if right == nil {
		right = b.EmitConstInstNil()
	}
	operator := ""
	switch {
	case exp.OperatorPower() != nil:
		operator = exp.OperatorPower().GetText()
	case exp.OperatorMulDivMod() != nil:
		operator = exp.OperatorMulDivMod().GetText()
	case exp.OperatorAddSub() != nil:
		operator = exp.OperatorAddSub().GetText()
	case exp.OperatorStrcat() != nil:
		operator = exp.OperatorStrcat().GetText()
	case exp.OperatorComparison() != nil:
		operator = exp.OperatorComparison().GetText()
	case exp.OperatorAnd() != nil:
		operator = exp.OperatorAnd().GetText()
	case exp.OperatorOr() != nil:
		operator = exp.OperatorOr().GetText()
	case exp.OperatorBitwise() != nil:
		operator = exp.OperatorBitwise().GetText()
	}
	return b.EmitBinOp(luaBinaryOpcode(operator), left, right)
}

func luaBinaryOpcode(operator string) ssa.BinaryOpcode {
	switch operator {
	case "+":
		return ssa.OpAdd
	case "-":
		return ssa.OpSub
	case "*":
		return ssa.OpMul
	case "/":
		return ssa.OpDiv
	case "//":
		return ssa.OpDiv
	case "%":
		return ssa.OpMod
	case "^":
		return ssa.OpPow
	case "..":
		return ssa.OpAdd
	case "<":
		return ssa.OpLt
	case ">":
		return ssa.OpGt
	case "<=":
		return ssa.OpLtEq
	case ">=":
		return ssa.OpGtEq
	case "~=":
		return ssa.OpNotEq
	case "==":
		return ssa.OpEq
	case "and":
		return ssa.OpAnd
	case "or":
		return ssa.OpOr
	case "&":
		return ssa.OpAnd
	case "|":
		return ssa.OpOr
	case "~":
		return ssa.OpXor
	case "<<":
		return ssa.OpShl
	case ">>":
		return ssa.OpShr
	default:
		return ssa.OpAdd
	}
}

func (b *singleFileBuilder) evalNumber(raw luaparser.INumberContext) ssa.Value {
	if raw == nil {
		return b.EmitConstInstNil()
	}
	text := raw.GetText()
	if integer, err := strconv.ParseInt(text, 0, 64); err == nil {
		return b.EmitConstInst(integer)
	}
	if floating, err := strconv.ParseFloat(text, 64); err == nil {
		return b.EmitConstInst(floating)
	}
	return b.EmitConstInst(text)
}

func luaStringValue(text string) string {
	if strings.HasPrefix(text, "[[") && strings.HasSuffix(text, "]]") {
		value := strings.TrimSuffix(strings.TrimPrefix(text, "[["), "]]")
		return strings.TrimPrefix(value, "\n")
	}
	if len(text) >= 2 {
		if unquoted, err := strconv.Unquote(text); err == nil {
			return unquoted
		}
		return strings.Trim(text, "\"'")
	}
	return text
}

func (b *singleFileBuilder) evalVarOrExp(raw luaparser.IVarOrExpContext) ssa.Value {
	ctx, ok := raw.(*luaparser.VarOrExpContext)
	if !ok || ctx == nil {
		return nil
	}
	if ctx.Var_() != nil {
		return b.evalVar(ctx.Var_()).value
	}
	if ctx.Exp() != nil {
		return b.VisitExp(ctx.Exp())
	}
	return nil
}

func (b *singleFileBuilder) evalPrefixexp(raw luaparser.IPrefixexpContext) ssa.Value {
	ctx, ok := raw.(*luaparser.PrefixexpContext)
	if !ok || ctx == nil {
		return nil
	}
	recoverRange := b.SetRange(ctx)
	defer recoverRange()

	value := b.evalVarOrExp(ctx.VarOrExp())
	access := luaAccess{value: value}
	for _, call := range ctx.AllNameAndArgs() {
		access.value = b.applyNameAndArgs(access.value, call)
		access.member = false
		access.name = ""
	}
	return access.value
}

func (b *singleFileBuilder) VisitFunctioncall(raw luaparser.IFunctioncallContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	call, ok := raw.(*luaparser.FunctioncallContext)
	if !ok || call == nil {
		return nil
	}
	recoverRange := b.SetRange(call)
	defer recoverRange()

	value := b.evalVarOrExp(call.VarOrExp())
	for _, nameAndArgs := range call.AllNameAndArgs() {
		value = b.applyNameAndArgs(value, nameAndArgs)
	}
	return value
}

func (b *singleFileBuilder) applyNameAndArgs(base ssa.Value, raw luaparser.INameAndArgsContext) ssa.Value {
	ctx, ok := raw.(*luaparser.NameAndArgsContext)
	if !ok || ctx == nil {
		return base
	}
	target := base
	if ctx.Colon() != nil && ctx.NAME() != nil {
		if target == nil {
			target = b.EmitUndefined("self")
		}
		target = b.ReadMemberCallMethod(target, b.EmitConstInst(ctx.NAME().GetText()))
	}
	if target == nil {
		target = b.EmitUndefined("call")
	}
	args := b.visitArgs(ctx.Args())
	if ctx.Colon() == nil && len(args) > 0 && base != nil {
		// Lua has no cast syntax; the standard conversions are the only
		// explicit type changes, so keep them as a dedicated TypeCast
		// instruction instead of a generic call.
		// TODO(ssa-lua): tonumber failure (nil) and tostring __tostring/metatable semantics are not modeled.
		switch base.GetName() {
		case "tonumber":
			return b.EmitTypeCast(args[0], ssa.CreateNumberType())
		case "tostring":
			return b.EmitTypeCast(args[0], ssa.CreateStringType())
		}
	}
	call := b.NewCall(target, args)
	return b.EmitCall(call)
}

func (b *singleFileBuilder) visitArgs(raw luaparser.IArgsContext) []ssa.Value {
	ctx, ok := raw.(*luaparser.ArgsContext)
	if !ok || ctx == nil {
		return nil
	}
	if ctx.Explist() != nil {
		return b.visitExpList(ctx.Explist())
	}
	if ctx.Tableconstructor() != nil {
		return []ssa.Value{b.evalTable(ctx.Tableconstructor())}
	}
	if ctx.String_() != nil {
		return []ssa.Value{b.EmitConstInst(luaStringValue(ctx.String_().GetText()))}
	}
	return nil
}

func (b *singleFileBuilder) visitExpList(raw luaparser.IExplistContext) []ssa.Value {
	ctx, ok := raw.(*luaparser.ExplistContext)
	if !ok || ctx == nil {
		return nil
	}
	values := make([]ssa.Value, 0, len(ctx.AllExp()))
	for _, exp := range ctx.AllExp() {
		values = append(values, b.VisitExp(exp))
	}
	return values
}

func (b *singleFileBuilder) evalVar(raw luaparser.IVarContext) luaAccess {
	ctx, ok := raw.(*luaparser.VarContext)
	if !ok || ctx == nil {
		return luaAccess{}
	}
	access := luaAccess{}
	if ctx.NAME() != nil {
		access.name = ctx.NAME().GetText()
		access.value = b.ReadValue(access.name)
		if access.value == nil {
			access.value = b.EmitUndefined(access.name)
		}
	} else if ctx.Exp() != nil {
		access.value = b.VisitExp(ctx.Exp())
	}
	for _, suffix := range ctx.AllVarSuffix() {
		access = b.applyVarSuffix(access, suffix)
	}
	return access
}

func (b *singleFileBuilder) applyVarSuffix(access luaAccess, raw luaparser.IVarSuffixContext) luaAccess {
	ctx, ok := raw.(*luaparser.VarSuffixContext)
	if !ok || ctx == nil {
		return access
	}
	for _, call := range ctx.AllNameAndArgs() {
		access.value = b.applyNameAndArgs(access.value, call)
		access.member = false
		access.name = ""
	}
	if ctx.LBracket() != nil {
		key := b.VisitExp(ctx.Exp())
		if key == nil {
			key = b.EmitConstInstNil()
		}
		object := access.value
		if object == nil {
			object = b.EmitUndefined("table")
		}
		return luaAccess{
			value:  b.ReadMemberCallValue(object, key),
			object: object,
			key:    key,
			member: true,
		}
	}
	if ctx.Dot() != nil && ctx.NAME() != nil {
		key := b.EmitConstInst(ctx.NAME().GetText())
		object := access.value
		if object == nil {
			object = b.EmitUndefined("table")
		}
		return luaAccess{
			value:  b.ReadMemberCallValue(object, key),
			object: object,
			key:    key,
			member: true,
		}
	}
	return access
}

func (b *singleFileBuilder) assignAccess(access luaAccess, value ssa.Value) {
	if value == nil {
		value = b.EmitConstInstNil()
	}
	if access.member {
		b.AssignVariable(b.CreateMemberCallVariable(access.object, access.key), value)
		return
	}
	if access.name != "" {
		b.AssignVariable(b.CreateVariable(access.name), value)
	}
}

func (b *singleFileBuilder) evalTable(raw luaparser.ITableconstructorContext) ssa.Value {
	ctx, ok := raw.(*luaparser.TableconstructorContext)
	if !ok || ctx == nil {
		return b.EmitUndefined("table")
	}
	container := b.EmitEmptyContainer()
	fieldlist := ctx.Fieldlist()
	if fieldlist == nil {
		return container
	}
	fields, ok := fieldlist.(*luaparser.FieldlistContext)
	if !ok || fields == nil {
		return container
	}
	arrayIndex := int64(1)
	for _, rawField := range fields.AllField() {
		field, ok := rawField.(*luaparser.FieldContext)
		if !ok || field == nil {
			continue
		}
		var key ssa.Value
		var value ssa.Value
		switch {
		case field.LBracket() != nil:
			key = b.VisitExp(field.Exp(0))
			value = b.VisitExp(field.Exp(1))
		case field.NAME() != nil:
			key = b.EmitConstInst(field.NAME().GetText())
			value = b.VisitExp(field.Exp(0))
		default:
			key = b.EmitConstInst(arrayIndex)
			arrayIndex++
			value = b.VisitExp(field.Exp(0))
		}
		if key == nil {
			key = b.EmitConstInstNil()
		}
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateMemberCallVariable(container, key), value)
	}
	return container
}

func (b *singleFileBuilder) buildFunction(name string, raw luaparser.IFuncbodyContext, method bool) *ssa.Function {
	body, ok := raw.(*luaparser.FuncbodyContext)
	if !ok || body == nil {
		return nil
	}
	previous := b.FunctionBuilder
	// Lua functions close over their lexical environment. A function
	// declared inside another function is a child function and captures
	// free values; top-level functions keep the historical global lookup.
	nested := b.functionDepth > 0
	// TODO(ssa-lua): upvalue mutation and _ENV lookup are not modeled; captured values are SSA FreeValues.
	previousSupport := previous.SupportClosure
	if previous != nil {
		previous.SupportClosure = nested
	}
	fn := b.NewFunc(name)
	b.FunctionBuilder = b.PushFunction(fn)
	b.SupportClosure = nested
	b.SetForceCapture(nested)
	b.functionDepth++
	defer func() { b.functionDepth-- }()
	if method {
		b.NewParam("self")
	}
	if parlist := body.Parlist(); parlist != nil {
		params, ok := parlist.(*luaparser.ParlistContext)
		if ok && params != nil {
			if names := params.Namelist(); names != nil {
				if nameList, ok := names.(*luaparser.NamelistContext); ok && nameList != nil {
					for _, param := range nameList.AllNAME() {
						b.NewParam(param.GetText())
					}
				}
			}
			if params.Ellipsis() != nil {
				b.NewParam("...")
			}
		}
	}
	b.VisitBlock(body.Block())
	b.Finish()
	b.SetForceCapture(false)
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	if previous != nil {
		previous.SupportClosure = previousSupport
	}
	return fn
}
