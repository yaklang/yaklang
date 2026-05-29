package python2ssa

import (
	"strings"

	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// visitDottedNameValue resolves a dotted_name (e.g. app.route) to an SSA callee value.
func (b *singleFileBuilder) visitDottedNameValue(dotted pythonparser.IDotted_nameContext) ssa.Value {
	ctx, ok := dotted.(*pythonparser.Dotted_nameContext)
	if !ok || ctx == nil {
		return nil
	}
	if inner := ctx.Dotted_name(); inner != nil {
		obj := b.visitDottedNameValue(inner)
		if obj == nil {
			return nil
		}
		nameCtx := ctx.Name()
		if nameCtx == nil {
			return obj
		}
		memberKey := b.EmitConstInst(nameCtx.GetText())
		obj = b.ensureDynamicObjectType(obj)
		return b.ensureDynamicValueType(b.ReadMemberCallMethod(obj, memberKey))
	}
	nameCtx := ctx.Name()
	if nameCtx == nil {
		return nil
	}
	if nc, ok := nameCtx.(*pythonparser.NameContext); ok {
		if v, ok := b.VisitName(nc).(ssa.Value); ok {
			return v
		}
	}
	return nil
}

func isRouteDecorator(dotted pythonparser.IDotted_nameContext) bool {
	if dotted == nil {
		return false
	}
	name := dotted.GetText()
	return name == "route" || strings.HasSuffix(name, ".route")
}

// visitDecoratorCall lowers @app.route(...) to an app.route(...) Call SSA node.
// For route decorators, the handler function is prepended as the first call argument
// so SyntaxFlow can bind app.route(...) to the view handler directly.
func (b *singleFileBuilder) visitDecoratorCall(dec pythonparser.IDecoratorContext, handler ssa.Value) ssa.Value {
	decCtx, ok := dec.(*pythonparser.DecoratorContext)
	if !ok || decCtx == nil {
		return nil
	}
	callee := b.visitDottedNameValue(decCtx.Dotted_name())
	if callee == nil {
		return nil
	}
	if arglist := decCtx.Arglist(); arglist != nil {
		args := b.VisitArglist(arglist)
		if isRouteDecorator(decCtx.Dotted_name()) && handler != nil {
			args = append([]ssa.Value{handler}, args...)
		}
		call := b.NewCall(callee, args)
		return b.ensureDynamicValueType(b.EmitCall(call))
	}
	return callee
}

// applyDecorators desugars decorator chains into nested Call instructions.
// Python applies decorators bottom-up (closest to def first).
func (b *singleFileBuilder) applyDecorators(decorators []pythonparser.IDecoratorContext, target ssa.Value) ssa.Value {
	if target == nil || len(decorators) == 0 {
		return target
	}
	handler := target
	result := target
	for i := len(decorators) - 1; i >= 0; i-- {
		decCall := b.visitDecoratorCall(decorators[i], handler)
		if decCall == nil {
			continue
		}
		call := b.NewCall(decCall, []ssa.Value{result})
		if wrapped := b.EmitCall(call); wrapped != nil {
			result = b.ensureDynamicValueType(wrapped)
		}
	}
	return result
}
