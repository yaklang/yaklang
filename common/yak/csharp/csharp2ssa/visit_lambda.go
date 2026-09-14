package csharp2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// lambda / 匿名方法 / 局部函数 / LINQ 查询表达式。
// 匿名函数沿用 java2ssa 的做法：NewFunc + PushFunction，外层变量通过父作用域直接可读。

// newClosureFunction creates a function as a child of the current function. C#
// enables closure support per anonymous/local function instead of globally: this
// keeps ordinary methods isolated while giving lambdas their lexical parent,
// stable qualified names and free-value capture.
func (b *singleFileBuilder) newClosureFunction(name string) (*ssa.Function, func()) {
	newFunc := b.newClosureFunctionShell(name)
	return newFunc, b.pushClosureFunction(newFunc)
}

// newClosureFunctionShell creates the stable child-function identity without
// capturing the parent's current scope. Local functions use this during the
// statement-list prepass so calls may resolve before the declaration; the
// actual PushFunction at the declaration point still captures the right scope.
func (b *singleFileBuilder) newClosureFunctionShell(name string) *ssa.Function {
	parent := b.FunctionBuilder
	parentClosure := parent.SupportClosure
	parent.SupportClosure = true
	defer func() { parent.SupportClosure = parentClosure }()

	parentName := b.csharpFunctionName(parent.Function)
	if name == "" && parent.Function != nil {
		if b.anonymousSerial == nil {
			b.anonymousSerial = make(map[*ssa.Function]uint64)
		}
		b.anonymousSerial[parent.Function]++
		name = fmt.Sprintf("%s$%d", parentName, b.anonymousSerial[parent.Function])
	} else if name != "" && parentName != "" {
		name = parentName + "." + name
	}
	newFunc := parent.NewFunc(name)
	// Preserve the lexical name when the function is later assigned to a
	// delegate variable. SetVerboseName is write-once.
	newFunc.SetVerboseName(newFunc.GetName())
	return newFunc
}

// pushClosureFunction starts populating an existing child-function shell.
// Keeping creation separate from PushFunction is what lets local functions be
// visible throughout their containing block without capturing variables too
// early during the declaration prepass.
func (b *singleFileBuilder) pushClosureFunction(newFunc *ssa.Function) func() {
	parent := b.FunctionBuilder
	parentClosure := parent.SupportClosure
	parent.SupportClosure = true
	b.FunctionBuilder = parent.PushFunction(newFunc)
	b.SupportClosure = true
	b.SetForceCapture(true)

	return func() {
		b.SetForceCapture(false)
		b.FunctionBuilder = b.PopFunction()
		parent.SupportClosure = parentClosure
	}
}

// csharpFunctionName removes the UUID-bearing implementation name used by
// method skeletons. Lambda/local-function identities should be deterministic
// and describe the source-level nesting (for example Main.Main$1).
func (b *singleFileBuilder) csharpFunctionName(f *ssa.Function) string {
	if f == nil {
		return ""
	}
	if class := b.MarkedThisClassBlueprint; class != nil && f.GetMethodName() != "" {
		return class.Name + "." + f.GetMethodName()
	}
	if name := f.GetVerboseName(); name != "" {
		return name
	}
	return f.GetName()
}

// newClosureParam mirrors FunctionBuilder.NewParam but binds the parameter
// directly in the function scope. That is intentional: a parameter may shadow
// a host/plugin extern with the same short name (notably `x`) and must not be
// diagnosed as an attempted write to that extern.
func (b *singleFileBuilder) newClosureParam(name string, pos ...ssa.CanStartStopToken) *ssa.Parameter {
	if name == "" {
		return nil
	}
	param := ssa.NewParam(name, false, b.FunctionBuilder)
	b.Params = append(b.Params, param.GetId())
	param.FormalParameterIndex = len(b.Params) - 1
	variable := b.CreateVariableForce(name, pos...)
	if b.CurrentRange != nil {
		variable.AddRange(b.CurrentRange, false)
	}
	b.CurrentBlock.ScopeTable.AssignVariable(variable, param)
	return param
}

func (b *singleFileBuilder) VisitLambdaExpression(raw csharpparser.ILambda_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Lambda_expressionContext)
	if !ok || i == nil {
		return nil
	}
	newFunc, restore := b.newClosureFunction("")
	{
		b.visitAnonymousFunctionSignature(i.Anonymous_function_signature())
		b.visitAnonymousFunctionBody(i.Anonymous_function_body())
		b.Finish()
	}
	restore()
	return newFunc
}

func (b *singleFileBuilder) VisitAnonymousMethodExpression(raw csharpparser.IAnonymous_method_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Anonymous_method_expressionContext)
	if !ok || i == nil {
		return nil
	}
	newFunc, restore := b.newClosureFunction("")
	{
		b.visitExplicitAnonymousSignature(i.Explicit_anonymous_function_signature())
		b.visitFunctionBodyBlock(i.Block())
		b.Finish()
	}
	restore()
	return newFunc
}

func (b *singleFileBuilder) visitAnonymousFunctionSignature(raw csharpparser.IAnonymous_function_signatureContext) {
	i, _ := raw.(*csharpparser.Anonymous_function_signatureContext)
	if i == nil {
		return
	}
	if i.Explicit_anonymous_function_signature() != nil {
		b.visitExplicitAnonymousSignature(i.Explicit_anonymous_function_signature())
		return
	}
	is, _ := i.Implicit_anonymous_function_signature().(*csharpparser.Implicit_anonymous_function_signatureContext)
	if is == nil {
		return
	}
	if p, _ := is.Implicit_anonymous_function_parameter().(*csharpparser.Implicit_anonymous_function_parameterContext); p != nil {
		if name := identText(p.Identifier()); name != "" {
			b.newClosureParam(name, p.Identifier())
		}
		return
	}
	list, _ := is.Implicit_anonymous_function_parameter_list().(*csharpparser.Implicit_anonymous_function_parameter_listContext)
	if list == nil {
		return
	}
	for _, p := range list.AllImplicit_anonymous_function_parameter() {
		pc, _ := p.(*csharpparser.Implicit_anonymous_function_parameterContext)
		if pc == nil {
			continue
		}
		if name := identText(pc.Identifier()); name != "" {
			b.newClosureParam(name, pc.Identifier())
		}
	}
}

func (b *singleFileBuilder) visitExplicitAnonymousSignature(raw csharpparser.IExplicit_anonymous_function_signatureContext) {
	i, _ := raw.(*csharpparser.Explicit_anonymous_function_signatureContext)
	if i == nil {
		return
	}
	list, _ := i.Explicit_anonymous_function_parameter_list().(*csharpparser.Explicit_anonymous_function_parameter_listContext)
	if list == nil {
		return
	}
	for _, p := range list.AllExplicit_anonymous_function_parameter() {
		pc, _ := p.(*csharpparser.Explicit_anonymous_function_parameterContext)
		if pc == nil {
			continue
		}
		name := identText(pc.Identifier())
		if name == "" {
			continue
		}
		param := b.newClosureParam(name, pc.Identifier())
		if pc.Type_() != nil {
			b.rememberDeclaredParameterType(param, b.VisitType(pc.Type_()))
		}
	}
}

func (b *singleFileBuilder) visitAnonymousFunctionBody(raw csharpparser.IAnonymous_function_bodyContext) {
	i, _ := raw.(*csharpparser.Anonymous_function_bodyContext)
	if i == nil {
		return
	}
	switch {
	case i.Block() != nil:
		b.visitFunctionBodyBlock(i.Block())
	case i.Expression() != nil:
		if v := b.VisitExpression(i.Expression()); !utils.IsNil(v) {
			b.EmitReturn([]ssa.Value{v})
		}
	case i.Null_conditional_invocation_expression() != nil:
		if v := b.VisitNullConditionalInvocationExpression(i.Null_conditional_invocation_expression()); !utils.IsNil(v) {
			b.EmitReturn([]ssa.Value{v})
		}
	case i.Variable_reference() != nil:
		if v := b.VisitVariableReference(i.Variable_reference()); !utils.IsNil(v) {
			b.EmitReturn([]ssa.Value{v})
		}
	}
}

// ---------------------------------------------------------------- local functions

// predeclareLocalFunction creates and binds a local-function shell for the
// containing statement-list scope. Its signature is populated immediately so
// an earlier call has real parameters and a return type; its body is deliberately
// deferred until the declaration point so closure capture sees the right scope.
func (b *singleFileBuilder) predeclareLocalFunction(raw csharpparser.ILocal_function_declarationContext) *ssa.Function {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Local_function_declarationContext)
	if !ok || i == nil {
		return nil
	}
	header, _ := i.Local_function_header().(*csharpparser.Local_function_headerContext)
	if header == nil {
		return nil
	}
	name := identText(header.Identifier())
	if name == "" {
		return nil
	}
	if b.localFunctionShells == nil {
		b.localFunctionShells = make(map[*csharpparser.Local_function_declarationContext]*ssa.Function)
	}
	if shell := b.localFunctionShells[i]; shell != nil {
		b.AssignVariable(b.CreateVariable(name), shell)
		return shell
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()
	parent := b.FunctionBuilder
	shell := b.newClosureFunctionShell(name)
	b.localFunctionShells[i] = shell
	parent.AssignVariable(parent.CreateVariable(name), shell)
	b.buildLocalFunctionSignature(i, header, shell)
	return shell
}

func (b *singleFileBuilder) buildLocalFunctionSignature(
	i *csharpparser.Local_function_declarationContext,
	header *csharpparser.Local_function_headerContext,
	shell *ssa.Function,
) {
	if i == nil || header == nil || shell == nil {
		return
	}
	restore := b.pushClosureFunction(shell)
	defer restore()

	if i.Return_type() != nil {
		b.SetCurrentReturnType(b.VisitReturnType(i.Return_type()))
	} else if rt, _ := i.Ref_return_type().(*csharpparser.Ref_return_typeContext); rt != nil {
		b.SetCurrentReturnType(b.VisitType(rt.Type_()))
	}
	b.visitLocalFunctionParameterList(header.Parameter_list())
	shell.ParamLength = len(shell.Params)

	parameterTypes := make([]ssa.Type, 0, len(shell.Params))
	parameterValues := make([]*ssa.Parameter, 0, len(shell.Params))
	for _, parameterID := range shell.Params {
		parameter, ok := shell.GetValueById(parameterID)
		if !ok || utils.IsNil(parameter) {
			parameterTypes = append(parameterTypes, ssa.CreateAnyType())
			continue
		}
		typ := parameter.GetType()
		if typ == nil {
			typ = ssa.CreateAnyType()
		}
		parameterTypes = append(parameterTypes, typ)
		if value, ok := ssa.ToParameter(parameter); ok {
			parameterValues = append(parameterValues, value)
		}
	}
	returnType := shell.GetCurrentReturnType()
	if returnType == nil {
		returnType = ssa.CreateAnyType()
	}
	functionType := ssa.NewFunctionType("", parameterTypes, returnType, localFunctionHasParameterArray(header.Parameter_list()))
	functionType.This = shell
	functionType.ParameterValue = parameterValues
	shell.SetType(functionType)
}

func localFunctionHasParameterArray(raw csharpparser.IParameter_listContext) bool {
	i, _ := raw.(*csharpparser.Parameter_listContext)
	return i != nil && i.Parameter_array() != nil
}

func (b *singleFileBuilder) VisitLocalFunctionDeclaration(raw csharpparser.ILocal_function_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Local_function_declarationContext)
	if !ok || i == nil {
		return
	}
	header, _ := i.Local_function_header().(*csharpparser.Local_function_headerContext)
	if header == nil {
		return
	}
	name := identText(header.Identifier())
	if name == "" {
		return
	}
	newFunc := b.localFunctionShells[i]
	if newFunc == nil {
		newFunc = b.predeclareLocalFunction(i)
	}
	if newFunc == nil || newFunc.IsFinished() {
		return
	}
	restore := b.pushClosureFunction(newFunc)
	{
		// Signature parameters were created during the statement-list prepass.
		// Preserve the variadic builder flag without creating them a second time.
		b.registerLocalFunctionReferenceParameters(header.Parameter_list())
		if localFunctionHasParameterArray(header.Parameter_list()) {
			b.HandlerEllipsis()
		}
		if body, _ := i.Local_function_body().(*csharpparser.Local_function_bodyContext); body != nil {
			switch {
			case body.Block() != nil:
				b.visitFunctionBodyBlock(body.Block())
			case body.Expression() != nil:
				if v := b.VisitExpression(body.Expression()); !utils.IsNil(v) {
					b.EmitReturn([]ssa.Value{v})
				}
			case body.Null_conditional_invocation_expression() != nil:
				if v := b.VisitNullConditionalInvocationExpression(body.Null_conditional_invocation_expression()); !utils.IsNil(v) {
					b.EmitReturn([]ssa.Value{v})
				}
			}
		} else if rb, _ := i.Ref_local_function_body().(*csharpparser.Ref_local_function_bodyContext); rb != nil {
			if rb.Block() != nil {
				b.visitFunctionBodyBlock(rb.Block())
			} else if rb.Variable_reference() != nil {
				if v := b.VisitVariableReference(rb.Variable_reference()); !utils.IsNil(v) {
					b.EmitReturn([]ssa.Value{v})
				}
			}
		}
		b.Finish()
	}
	restore()
}

func (b *singleFileBuilder) registerLocalFunctionReferenceParameters(raw csharpparser.IParameter_listContext) {
	list, _ := raw.(*csharpparser.Parameter_listContext)
	if list == nil {
		return
	}
	fixed, _ := list.Fixed_parameters().(*csharpparser.Fixed_parametersContext)
	if fixed == nil {
		return
	}
	for index, rawParam := range fixed.AllFixed_parameter() {
		param, _ := rawParam.(*csharpparser.Fixed_parameterContext)
		modifier := csharpFixedParameterModifier(param)
		if param == nil || (modifier != "ref" && modifier != "out") {
			continue
		}
		b.ReferenceParameter(identText(param.Identifier()), index, ssa.PointerSideEffect)
	}
}

func (b *singleFileBuilder) visitLocalFunctionParameterList(raw csharpparser.IParameter_listContext) {
	i, _ := raw.(*csharpparser.Parameter_listContext)
	if i == nil {
		return
	}
	if fixed, _ := i.Fixed_parameters().(*csharpparser.Fixed_parametersContext); fixed != nil {
		for _, rawParam := range fixed.AllFixed_parameter() {
			paramCtx, _ := rawParam.(*csharpparser.Fixed_parameterContext)
			if paramCtx == nil {
				continue
			}
			name := identText(paramCtx.Identifier())
			param := b.newClosureParam(name, paramCtx.Identifier())
			if param == nil {
				continue
			}
			modifier := csharpFixedParameterModifier(paramCtx)
			if modifier == "ref" || modifier == "out" {
				// Local functions use closure parameters, but ref/out still carry the
				// same positional pointer side effect as ordinary source methods.
				b.ReferenceParameter(name, param.FormalParameterIndex, ssa.PointerSideEffect)
			}
			if paramCtx.Type_() != nil {
				b.rememberDeclaredParameterType(param, b.VisitType(paramCtx.Type_()))
			}
			if def, _ := paramCtx.Default_argument().(*csharpparser.Default_argumentContext); def != nil && def.Expression() != nil {
				if value := b.VisitExpression(def.Expression()); !utils.IsNil(value) {
					param.SetDefault(value)
				}
			}
		}
	}
	if params, _ := i.Parameter_array().(*csharpparser.Parameter_arrayContext); params != nil {
		param := b.newClosureParam(identText(params.Identifier()), params.Identifier())
		if param != nil {
			if params.Array_type() != nil {
				b.rememberDeclaredParameterType(param, b.visitArrayType(params.Array_type()))
			}
			b.HandlerEllipsis()
		}
	}
}

// ---------------------------------------------------------------- LINQ query expressions

// VisitQueryExpression lowers query syntax to its LINQ method-call shape. Keeping
// where/select bodies as child functions is important for interprocedural data
// flow and also matches how equivalent method-syntax lambdas are represented.
func (b *singleFileBuilder) VisitQueryExpression(raw csharpparser.IQuery_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Query_expressionContext)
	if !ok || i == nil {
		return nil
	}
	from, _ := i.From_clause().(*csharpparser.From_clauseContext)
	if from == nil {
		return b.EmitUndefined("query")
	}
	source := b.VisitExpression(from.Expression())
	if utils.IsNil(source) {
		source = b.EmitUndefined("query-source")
	}
	source = b.querySourceValue(source)
	return b.visitQueryBody(source, newQueryRangeScope(identText(from.Identifier())), i.Query_body())
}

// querySourceValue preserves the expression behind a range-source variable.
// Assignment aliases are useful for references, but a query chain should retain
// the source call itself (`foo().where(...)`, not `nums.where(...)`). Reuse the
// already-emitted call: cloning it would evaluate a side-effecting source twice.
func (b *singleFileBuilder) querySourceValue(value ssa.Value) ssa.Value {
	call, ok := ssa.ToCall(value)
	if !ok || call == nil {
		return value
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || utils.IsNil(method) {
		return value
	}
	if methodName := method.GetVerboseName(); methodName != "" {
		value.SetVerboseName(methodName + "()")
	} else if methodName = method.GetName(); methodName != "" {
		value.SetVerboseName(methodName + "()")
	}
	return value
}

// queryRangeScope describes the element currently flowing through a lowered
// query. The first from-clause can use its element directly. Clauses which
// introduce another range variable (`let`, a later `from`, or `join`) must use
// a transparent carrier so every earlier range variable remains in scope.
type queryRangeScope struct {
	names       []string
	transparent bool
}

func newQueryRangeScope(name string) queryRangeScope {
	return queryRangeScope{names: []string{name}}
}

func (scope queryRangeScope) with(name string) queryRangeScope {
	names := append([]string(nil), scope.names...)
	if name != "" {
		names = append(names, name)
	}
	return queryRangeScope{names: names, transparent: true}
}

// bindQueryRange creates the lambda parameter for the current query element and
// binds every source-level range name in the lambda scope. For a transparent
// element the source-level names are member reads, never unresolved captures.
func (b *singleFileBuilder) bindQueryRange(scope queryRangeScope, token ssa.CanStartStopToken) map[string]ssa.Value {
	values := make(map[string]ssa.Value, len(scope.names))
	if !scope.transparent && len(scope.names) == 1 {
		name := scope.names[0]
		param := b.newClosureParam(name, token)
		if param == nil {
			param = b.newClosureParam("$range", token)
		}
		if param != nil && name != "" {
			values[name] = param
		}
		return values
	}

	carrier := b.newClosureParam("$transparent", token)
	if carrier == nil {
		return values
	}
	for _, name := range scope.names {
		if name == "" {
			continue
		}
		value := b.ReadMemberCallValue(carrier, b.EmitConstInstPlaceholder(name))
		if utils.IsNil(value) {
			value = b.EmitUndefined(name)
		}
		b.AssignVariable(b.CreateLocalVariable(name), value)
		values[name] = value
	}
	return values
}

func (b *singleFileBuilder) emitQueryCarrier(scope queryRangeScope, values map[string]ssa.Value, introducedName string, introduced ssa.Value) ssa.Value {
	carrier := b.EmitMakeWithoutType(nil, nil)
	for _, name := range scope.names {
		value := values[name]
		if utils.IsNil(value) {
			value = b.PeekValue(name)
		}
		if utils.IsNil(value) {
			value = b.EmitUndefined(name)
		}
		b.AssignVariable(b.CreateMemberCallVariable(carrier, b.EmitConstInstPlaceholder(name)), value)
	}
	if introducedName != "" {
		if utils.IsNil(introduced) {
			introduced = b.EmitUndefined(introducedName)
		}
		b.AssignVariable(b.CreateMemberCallVariable(carrier, b.EmitConstInstPlaceholder(introducedName)), introduced)
	}
	return carrier
}

func (b *singleFileBuilder) buildQueryLambda(scope queryRangeScope, token ssa.CanStartStopToken, body func() ssa.Value) ssa.Value {
	fn, restore := b.newClosureFunction("")
	b.bindQueryRange(scope, token)
	if body != nil {
		if value := body(); !utils.IsNil(value) && !b.IsBlockFinish() {
			b.EmitReturn([]ssa.Value{value})
		}
	}
	b.Finish()
	restore()
	return fn
}

// buildQueryResultSelector constructs the result-selector used by SelectMany,
// Join and GroupJoin. Its first parameter is the current query element and its
// second is the newly introduced value; the result is a transparent carrier.
func (b *singleFileBuilder) buildQueryResultSelector(
	scope queryRangeScope,
	outerToken ssa.CanStartStopToken,
	introducedName string,
	introducedToken ssa.CanStartStopToken,
) ssa.Value {
	fn, restore := b.newClosureFunction("")
	values := b.bindQueryRange(scope, outerToken)
	introduced := b.newClosureParam(introducedName, introducedToken)
	if introduced == nil {
		introduced = b.newClosureParam("$introduced", introducedToken)
	}
	carrier := b.emitQueryCarrier(scope, values, introducedName, introduced)
	if !b.IsBlockFinish() {
		b.EmitReturn([]ssa.Value{carrier})
	}
	b.Finish()
	restore()
	return fn
}

func (b *singleFileBuilder) queryMethodCall(source ssa.Value, method string, args ...ssa.Value) ssa.Value {
	if utils.IsNil(source) {
		source = b.EmitUndefined("query-source")
	}
	// Query syntax models the Enumerable extension-method form. Use a valid
	// member value and pass the source explicitly as the first argument instead
	// of turning it into an instance-method receiver implicitly.
	callee := b.ReadMemberCallValue(source, b.EmitConstInstPlaceholder(method))
	callArgs := make([]ssa.Value, 0, len(args)+1)
	callArgs = append(callArgs, source)
	callArgs = append(callArgs, args...)
	call := b.emitCall(callee, callArgs, nil, method)
	if !utils.IsNil(call) && source.GetVerboseName() != "" {
		call.SetVerboseName(source.GetVerboseName())
	}
	return call
}

func (b *singleFileBuilder) visitQueryBody(source ssa.Value, scope queryRangeScope, raw csharpparser.IQuery_bodyContext) ssa.Value {
	i, _ := raw.(*csharpparser.Query_bodyContext)
	if i == nil {
		return source
	}
	for _, clause := range i.AllQuery_body_clause() {
		source, scope = b.visitQueryBodyClause(source, scope, clause)
	}
	result := b.visitSelectOrGroupClause(source, scope, i.Select_or_group_clause())
	if qc, _ := i.Query_continuation().(*csharpparser.Query_continuationContext); qc != nil {
		return b.visitQueryBody(result, newQueryRangeScope(identText(qc.Identifier())), qc.Query_body())
	}
	return result
}

func (b *singleFileBuilder) visitQueryBodyClause(source ssa.Value, scope queryRangeScope, raw csharpparser.IQuery_body_clauseContext) (ssa.Value, queryRangeScope) {
	i, _ := raw.(*csharpparser.Query_body_clauseContext)
	if i == nil {
		return source, scope
	}
	switch {
	case i.From_clause() != nil:
		from, _ := i.From_clause().(*csharpparser.From_clauseContext)
		if from != nil {
			introducedName := identText(from.Identifier())
			selector := b.buildQueryLambda(scope, from.Identifier(), func() ssa.Value {
				return b.VisitExpression(from.Expression())
			})
			resultSelector := b.buildQueryResultSelector(scope, from, introducedName, from.Identifier())
			source = b.queryMethodCall(source, "selectMany", selector, resultSelector)
			scope = scope.with(introducedName)
		}
	case i.Let_clause() != nil:
		let, _ := i.Let_clause().(*csharpparser.Let_clauseContext)
		if let != nil {
			introducedName := identText(let.Identifier())
			selector := b.buildQueryLambda(scope, let.Identifier(), func() ssa.Value {
				values := make(map[string]ssa.Value, len(scope.names))
				for _, name := range scope.names {
					values[name] = b.PeekValue(name)
				}
				introduced := b.VisitExpression(let.Expression())
				return b.emitQueryCarrier(scope, values, introducedName, introduced)
			})
			source = b.queryMethodCall(source, "select", selector)
			scope = scope.with(introducedName)
		}
	case i.Where_clause() != nil:
		where, _ := i.Where_clause().(*csharpparser.Where_clauseContext)
		if where != nil && where.Boolean_expression() != nil {
			predicate := b.buildQueryLambda(scope, where, func() ssa.Value {
				return b.VisitExpression(where.Boolean_expression().Expression())
			})
			source = b.queryMethodCall(source, "where", predicate)
		}
	case i.Join_clause() != nil:
		join, _ := i.Join_clause().(*csharpparser.Join_clauseContext)
		if join != nil {
			exprs := join.AllExpression()
			if len(exprs) >= 3 {
				inner := b.VisitExpression(exprs[0])
				outerKey := b.buildQueryLambda(scope, join, func() ssa.Value { return b.VisitExpression(exprs[1]) })
				joinName := identText(join.Identifier())
				innerKey := b.buildQueryLambda(newQueryRangeScope(joinName), join.Identifier(), func() ssa.Value { return b.VisitExpression(exprs[2]) })
				resultSelector := b.buildQueryResultSelector(scope, join, joinName, join.Identifier())
				source = b.queryMethodCall(source, "join", inner, outerKey, innerKey, resultSelector)
				scope = scope.with(joinName)
			}
		}
	case i.Join_into_clause() != nil:
		join, _ := i.Join_into_clause().(*csharpparser.Join_into_clauseContext)
		if join != nil {
			exprs, ids := join.AllExpression(), join.AllIdentifier()
			if len(exprs) >= 3 && len(ids) >= 2 {
				inner := b.VisitExpression(exprs[0])
				outerKey := b.buildQueryLambda(scope, join, func() ssa.Value { return b.VisitExpression(exprs[1]) })
				joinName := identText(ids[0])
				innerKey := b.buildQueryLambda(newQueryRangeScope(joinName), ids[0], func() ssa.Value { return b.VisitExpression(exprs[2]) })
				intoName := identText(ids[1])
				resultSelector := b.buildQueryResultSelector(scope, join, intoName, ids[1])
				source = b.queryMethodCall(source, "groupJoin", inner, outerKey, innerKey, resultSelector)
				scope = scope.with(intoName)
			}
		}
	case i.Orderby_clause() != nil:
		orderBy, _ := i.Orderby_clause().(*csharpparser.Orderby_clauseContext)
		if orderBy != nil {
			if orderings, _ := orderBy.Orderings().(*csharpparser.OrderingsContext); orderings != nil {
				for index, rawOrdering := range orderings.AllOrdering() {
					if ordering, _ := rawOrdering.(*csharpparser.OrderingContext); ordering != nil {
						selector := b.buildQueryLambda(scope, ordering, func() ssa.Value {
							return b.VisitExpression(ordering.Expression())
						})
						method := "thenBy"
						if index == 0 {
							method = "orderBy"
						}
						if direction, _ := ordering.Ordering_direction().(*csharpparser.Ordering_directionContext); direction != nil && direction.KW_DESCENDING() != nil {
							method += "Descending"
						}
						source = b.queryMethodCall(source, method, selector)
					}
				}
			}
		}
	}
	return source, scope
}

func (b *singleFileBuilder) visitSelectOrGroupClause(source ssa.Value, scope queryRangeScope, raw csharpparser.ISelect_or_group_clauseContext) ssa.Value {
	i, _ := raw.(*csharpparser.Select_or_group_clauseContext)
	if i == nil {
		return source
	}
	if selectClause, _ := i.Select_clause().(*csharpparser.Select_clauseContext); selectClause != nil {
		selector := b.buildQueryLambda(scope, selectClause, func() ssa.Value {
			return b.VisitExpression(selectClause.Expression())
		})
		return b.queryMethodCall(source, "select", selector)
	}
	if group, _ := i.Group_clause().(*csharpparser.Group_clauseContext); group != nil {
		exprs := group.AllExpression()
		if len(exprs) >= 2 {
			element := b.buildQueryLambda(scope, group, func() ssa.Value { return b.VisitExpression(exprs[0]) })
			key := b.buildQueryLambda(scope, group, func() ssa.Value { return b.VisitExpression(exprs[1]) })
			return b.queryMethodCall(source, "groupBy", key, element)
		}
	}
	return source
}
