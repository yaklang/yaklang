package csharp2ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// primary_expression 编译：成员访问、调用、索引、null 条件访问、对象/数组创建、tuple 等。
//
// 成员访问的关键是「纯标识符链」（a.b.c）的解析：首段可能是局部变量、类成员、
// 类型名（静态访问）或命名空间前缀（System.IO.File），见 resolveIdentifierChain。

var csharpPredefinedClrNames = map[string]string{
	"bool": "Boolean", "byte": "Byte", "char": "Char", "decimal": "Decimal",
	"double": "Double", "float": "Single", "int": "Int32", "long": "Int64",
	"object": "Object", "sbyte": "SByte", "short": "Int16", "string": "String",
	"uint": "UInt32", "ulong": "UInt64", "ushort": "UInt16",
}

// wellKnownNamespaces 帮助区分 `System.IO.File.X` 中的命名空间段与类型段。
var wellKnownNamespaces = map[string]bool{
	"System": true, "System.IO": true, "System.Text": true, "System.Net": true,
	"System.Net.Http": true, "System.Net.Sockets": true, "System.Web": true,
	"System.Web.UI": true, "System.Web.UI.WebControls": true, "System.Web.Mvc": true,
	"System.Web.Http": true, "System.Web.Security": true, "System.Data": true,
	"System.Data.SqlClient": true, "System.Data.Common": true, "System.Linq": true,
	"System.Collections": true, "System.Collections.Generic": true,
	"System.Collections.Concurrent": true, "System.Threading": true,
	"System.Threading.Tasks": true, "System.Diagnostics": true, "System.Security": true,
	"System.Security.Cryptography": true, "System.Security.Claims": true, "System.Xml": true,
	"System.Xml.Linq": true, "System.Xml.Serialization": true, "System.Reflection": true,
	"System.Runtime": true, "System.Runtime.Serialization": true,
	"System.Runtime.InteropServices": true, "System.Text.RegularExpressions": true,
	"System.Text.Json": true, "System.ComponentModel": true, "System.Globalization": true,
	"System.Drawing": true, "System.Configuration": true, "System.Runtime.CompilerServices": true,
	"Microsoft": true, "Microsoft.AspNetCore": true, "Microsoft.AspNetCore.Mvc": true,
	"Microsoft.AspNetCore.Http": true, "Microsoft.AspNetCore.Builder": true,
	"Microsoft.AspNetCore.Hosting": true, "Microsoft.AspNetCore.Authorization": true,
	"Microsoft.Extensions": true, "Microsoft.Extensions.DependencyInjection": true,
	"Microsoft.Extensions.Logging": true, "Microsoft.Extensions.Configuration": true,
	"Microsoft.EntityFrameworkCore": true, "Microsoft.Data.SqlClient": true,
	"Newtonsoft": true, "Newtonsoft.Json": true, "Newtonsoft.Json.Linq": true,
}

func (b *singleFileBuilder) VisitPrimaryExpression(raw csharpparser.IPrimary_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Primary_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if inner := i.Primary_expression(); inner != nil {
		return b.visitPostfixPrimary(i, inner)
	}
	switch {
	case i.Literal() != nil:
		return b.VisitLiteral(i.Literal())
	case i.Interpolated_string_expression() != nil:
		return b.VisitInterpolatedString(i.Interpolated_string_expression())
	case i.Simple_name() != nil:
		sn, _ := i.Simple_name().(*csharpparser.Simple_nameContext)
		if sn == nil {
			return b.EmitUndefined(i.GetText())
		}
		v := b.ReadIdentifierValue(identText(sn.Identifier()), sn)
		b.restoreMemberVerboseName(v)
		return v
	case i.Tuple_expression() != nil:
		return b.VisitTupleExpression(i.Tuple_expression())
	case i.Parenthesized_expression() != nil:
		pe, _ := i.Parenthesized_expression().(*csharpparser.Parenthesized_expressionContext)
		if pe == nil {
			return b.EmitUndefined(i.GetText())
		}
		return b.VisitExpression(pe.Expression())
	case i.Predefined_type() != nil:
		// string.Empty / int.Parse
		obj := b.predefinedTypeContainer(i.Predefined_type().GetText(), i.Predefined_type())
		if i.Identifier() != nil {
			return b.readMember(obj, identText(i.Identifier()), false)
		}
		return obj
	case i.Qualified_alias_member() != nil:
		segments := b.qualifiedAliasSegments(i.Qualified_alias_member())
		if i.Identifier() != nil {
			segments = append(segments, identText(i.Identifier()))
		}
		return b.resolveIdentifierChain(segments, i)
	case i.This_access() != nil:
		return b.thisValue()
	case i.Base_access() != nil:
		return b.VisitBaseAccess(i.Base_access())
	case i.Array_creation_expression() != nil:
		return b.VisitArrayCreation(i.Array_creation_expression())
	case i.Object_creation_expression() != nil:
		return b.VisitObjectCreation(i.Object_creation_expression())
	case i.Delegate_creation_expression() != nil:
		dc, _ := i.Delegate_creation_expression().(*csharpparser.Delegate_creation_expressionContext)
		if dc != nil && dc.Expression() != nil {
			return b.VisitExpression(dc.Expression())
		}
		return b.EmitUndefined(i.GetText())
	case i.Anonymous_object_creation_expression() != nil:
		return b.VisitAnonymousObjectCreation(i.Anonymous_object_creation_expression())
	case i.Typeof_expression() != nil:
		return b.VisitTypeofExpression(i.Typeof_expression())
	case i.Checked_expression() != nil:
		ce, _ := i.Checked_expression().(*csharpparser.Checked_expressionContext)
		if ce != nil {
			return b.VisitExpression(ce.Expression())
		}
	case i.Unchecked_expression() != nil:
		ue, _ := i.Unchecked_expression().(*csharpparser.Unchecked_expressionContext)
		if ue != nil {
			return b.VisitExpression(ue.Expression())
		}
	case i.Default_value_expression() != nil:
		return b.VisitDefaultValueExpression(i.Default_value_expression())
	case i.Nameof_expression() != nil:
		return b.VisitNameofExpression(i.Nameof_expression())
	case i.Anonymous_method_expression() != nil:
		return b.VisitAnonymousMethodExpression(i.Anonymous_method_expression())
	case i.Stackalloc_expression() != nil:
		return b.EmitMakeWithoutType(nil, nil)
	case i.Sizeof_expression() != nil:
		return b.EmitUndefined(i.GetText())
	}
	return b.EmitUndefined(i.GetText())
}

// visitPostfixPrimary handles alternatives of the form `primary_expression <suffix>`.
func (b *singleFileBuilder) visitPostfixPrimary(i *csharpparser.Primary_expressionContext, inner csharpparser.IPrimary_expressionContext) ssa.Value {
	switch {
	case i.TK_QMARK() != nil:
		return b.visitNullConditional(i, inner)
	case i.TK_LPAREN() != nil:
		return b.visitInvocation(inner, i.Argument_list())
	case i.TK_DOT() != nil && i.Identifier() != nil:
		return b.visitMemberAccess(i, inner)
	case i.TK_MINUS_GT() != nil && i.Identifier() != nil:
		obj := b.VisitPrimaryExpression(inner)
		return b.readMember(obj, identText(i.Identifier()), false)
	case i.TK_LBRACK() != nil:
		obj := b.VisitPrimaryExpression(inner)
		return b.elementValue(obj, b.visitElementArguments(i.Argument_list()))
	case i.TK_PLUS_PLUS() != nil:
		return b.visitIncDec(inner, true, false)
	case i.TK_MINUS_MINUS() != nil:
		return b.visitIncDec(inner, false, false)
	}
	// null forgiving `x!` and anything else: transparent
	return b.VisitPrimaryExpression(inner)
}

// ---------------------------------------------------------------- identifier chains

// identifierChain returns [a, b, c] for `a.b.c` when every segment is a plain identifier
// (no generics, no calls, no indexers); otherwise nil.
func (b *singleFileBuilder) identifierChain(raw csharpparser.IPrimary_expressionContext) []string {
	i, _ := raw.(*csharpparser.Primary_expressionContext)
	if i == nil {
		return nil
	}
	if sn, _ := i.Simple_name().(*csharpparser.Simple_nameContext); sn != nil {
		if sn.Type_argument_list() != nil {
			return nil
		}
		name := identText(sn.Identifier())
		if name == "" {
			return nil
		}
		return []string{name}
	}
	inner := i.Primary_expression()
	if inner == nil || i.TK_DOT() == nil || i.Identifier() == nil || i.TK_QMARK() != nil || i.Type_argument_list() != nil {
		return nil
	}
	prefix := b.identifierChain(inner)
	if prefix == nil {
		return nil
	}
	return append(prefix, identText(i.Identifier()))
}

// readLocalOrMember resolves a bare identifier only against variables, class members and imports;
// it never invents a type.
func (b *singleFileBuilder) readLocalOrMember(name string) ssa.Value {
	if v := b.PeekValue(name); !utils.IsNil(v) {
		return v
	}
	if v := b.readClassMemberValue(name); !utils.IsNil(v) {
		return v
	}
	if v, ok := b.constMap[name]; ok && !utils.IsNil(v) {
		return v
	}
	if prog := b.GetProgram(); prog != nil {
		if v, ok := prog.ReadImportValue(name); ok && !utils.IsNil(v) {
			return v
		}
	}
	return nil
}

// resolveIdentifierChain resolves `a.b.c` to a value.
func (b *singleFileBuilder) resolveIdentifierChain(chain []string, token ssa.CanStartStopToken) ssa.Value {
	if len(chain) == 0 {
		return nil
	}
	var obj ssa.Value
	rest := chain[1:]
	if v := b.readLocalOrMember(chain[0]); !utils.IsNil(v) {
		obj = v
	} else if bp, remain := b.resolveTypePath(chain, token); bp != nil {
		obj = bp.Container()
		rest = remain
	} else {
		obj = b.ReadIdentifierValue(chain[0], token)
	}
	for _, seg := range rest {
		obj = b.readMember(obj, seg, false)
	}
	return obj
}

// resolveTypePath splits an identifier chain into (type blueprint, remaining member segments).
func (b *singleFileBuilder) resolveTypePath(chain []string, token ssa.CanStartStopToken) (*ssa.Blueprint, []string) {
	if len(chain) == 0 {
		return nil, nil
	}
	if target, _, ok := b.lookupUsingAlias(chain[0]); ok {
		expanded := strings.Split(stripGenericSuffix(target), ".")
		chain = append(expanded, chain[1:]...)
	}
	// 1. 已存在的蓝图：最长前缀优先（Ns.Type / Outer.Inner / Type）
	for k := len(chain); k >= 1; k-- {
		if bp := b.lookupBlueprintByPathStrict(chain[:k]); bp != nil {
			return bp, chain[k:]
		}
	}
	// 2. 命名空间前缀 + 类型段
	if k := b.knownNamespacePrefixLen(chain); k > 0 && k < len(chain) {
		return b.blueprintByName(chain[:k+1], token), chain[k+1:]
	}
	if len(chain) > 1 && b.isNamespacePrefix(chain[0]) {
		return b.blueprintByName(chain, token), nil
	}
	// 3. 首段本身是类型名
	if looksLikeTypeName(chain[0]) {
		return b.blueprintByName(chain[:1], token), chain[1:]
	}
	return nil, chain
}

// knownNamespacePrefixLen returns the length of the longest chain prefix that is a known namespace.
func (b *singleFileBuilder) knownNamespacePrefixLen(chain []string) int {
	best := 0
	var app *ssa.Program
	if prog := b.GetProgram(); prog != nil {
		app = prog.GetApplication()
		if app == nil {
			app = prog
		}
	}
	for k := 1; k < len(chain); k++ {
		ns := strings.Join(chain[:k], ".")
		known := wellKnownNamespaces[ns] || utils.StringArrayContains(b.visibleUsings(), ns)
		if !known && app != nil {
			if lib, _ := app.GetLibrary(ns); lib != nil {
				known = true
			}
		}
		if known {
			best = k
		}
	}
	return best
}

func (b *singleFileBuilder) predefinedTypeContainer(keyword string, token ssa.CanStartStopToken) ssa.Value {
	clr, ok := csharpPredefinedClrNames[keyword]
	if !ok {
		clr = keyword
	}
	bp := b.blueprintByName([]string{"System", clr}, token)
	if bp == nil {
		return b.EmitUndefined(keyword)
	}
	return bp.Container()
}

func (b *singleFileBuilder) qualifiedAliasSegments(raw csharpparser.IQualified_alias_memberContext) []string {
	i, _ := raw.(*csharpparser.Qualified_alias_memberContext)
	if i == nil {
		return nil
	}
	ids := i.AllIdentifier()
	if len(ids) == 0 {
		return nil
	}
	alias := identText(ids[0])
	var segments []string
	if alias != "global" {
		if target, _, ok := b.lookupUsingAlias(alias); ok {
			segments = strings.Split(stripGenericSuffix(target), ".")
		} else {
			segments = []string{alias}
		}
	}
	for _, id := range ids[1:] {
		segments = append(segments, identText(id))
	}
	return segments
}

// ---------------------------------------------------------------- member / element access

// readMember reads `obj.name`; wantMethod prefers the method slot (call position).
func (b *singleFileBuilder) readMember(obj ssa.Value, name string, wantMethod bool) ssa.Value {
	if name == "" {
		return obj
	}
	if utils.IsNil(obj) {
		return b.EmitUndefined(name)
	}
	if !wantMethod {
		if tupleItem := b.readTupleItem(obj, name); !utils.IsNil(tupleItem) {
			return tupleItem
		}
		if getter, ok := b.emitDeclaredAccessor(obj, "get_"+name, nil); ok {
			return getter
		}
		// A method group (`var callback = instance.Method`) is still a read of
		// an instance method, even though invocation happens later. Preserve the
		// receiver in a fresh proxy now; returning the blueprint's shared method
		// slot would make two method groups collapse onto one receiver (or none).
		// Resolve through the declared-method table rather than the value's
		// function type so ordinary delegate/function-valued fields stay fields.
		if _, ok := b.declaredInstanceMethod(obj, name); ok {
			return b.readCallableMember(obj, name)
		}
	}
	key := b.EmitConstInstPlaceholder(name)
	// Writes performed after construction (including object initializers) belong
	// to the call value itself and must take precedence over constructor state.
	if !wantMethod {
		if member := b.ReadMemberCallValueByName(obj, name); !utils.IsNil(member) {
			return member
		}
	}
	// A C# constructor mutates the receiver supplied as its first argument and
	// returns that same instance.  The generic SSA call value is distinct from
	// the receiver, though, so looking a field up on the call directly would
	// skip constructor side effects and fall back to the class initializer.
	// Prefer the receiver's latest member for constructor results; if the
	// constructor did not write it, normal lookup below still exposes the
	// field/auto-property initializer registered on the blueprint.
	if !wantMethod {
		if member := b.readConstructorReceiverMemberThroughCast(obj, name, key); !utils.IsNil(member) {
			return member
		}
	}
	var v ssa.Value
	if wantMethod {
		v = b.ReadMemberCallMethod(obj, key)
	} else {
		v = b.ReadMemberCallMethodOrValue(obj, key)
	}
	if utils.IsNil(v) {
		return b.EmitUndefined(name)
	}
	return v
}

// readConstructorReceiverMemberThroughCast keeps a declaration cast as the
// receiver's compile-time type without making it an object-state boundary. A
// field visible on Base can therefore still read the state written by the
// Derived constructor held by the cast operand.
func (b *singleFileBuilder) readConstructorReceiverMemberThroughCast(obj ssa.Value, name string, key ssa.Value) ssa.Value {
	if member := b.readConstructorReceiverMember(obj, key); !utils.IsNil(member) {
		return member
	}
	cast, ok := ssa.ToTypeCast(obj)
	if !ok || cast == nil {
		return nil
	}
	declared, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || declared == nil || utils.IsNil(declared.GetNormalMember(name)) {
		return nil
	}
	operand, ok := cast.GetValueById(cast.Value)
	if !ok || utils.IsNil(operand) {
		return nil
	}
	member := b.readConstructorReceiverMemberThroughCast(operand, name, key)
	if utils.IsNil(member) {
		return nil
	}

	// A source-level cast selects the field declared on its static receiver.
	// `Base.Value` and `Derived.Value` are distinct slots even though generic SSA
	// member keys are both the string "Value". Only project constructor state
	// whose recorded declaring owner matches that selected slot. If the writer
	// cannot be recovered (for example through an opaque call), comparing the
	// operand's visible declaration still prevents a hidden derived field from
	// crossing the cast boundary.
	requested, _, hasRequested := b.declaredMemberSlotForReceiver(obj, name)
	if !hasRequested {
		return member
	}
	if writer, known := b.declaredMemberWriter(member); known {
		if writer != requested {
			return declared.GetNormalMember(name)
		}
		return member
	}
	if dynamic, _, known := b.declaredMemberSlotForReceiver(operand, name); known && dynamic != requested {
		return declared.GetNormalMember(name)
	}
	return member
}

func (b *singleFileBuilder) readConstructorReceiverMember(obj, key ssa.Value) ssa.Value {
	call, ok := ssa.ToCall(obj)
	if !ok || call == nil || len(call.Args) == 0 {
		return nil
	}
	class, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || class == nil {
		return nil
	}
	receiver, ok := call.GetValueById(call.Args[0])
	if !ok || utils.IsNil(receiver) {
		return nil
	}
	receiverClass, ok := ssa.ToBluePrintType(receiver.GetType())
	if !ok || receiverClass != class {
		return nil
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || utils.IsNil(method) {
		return nil
	}
	constructor, ok := ssa.ToFunction(method)
	if !ok || !b.isConstructorInHierarchy(class, constructor) {
		return nil
	}
	member, ok := ssa.GetLatestMemberByKey(receiver, key)
	if !ok {
		return nil
	}
	return member
}

func (b *singleFileBuilder) isConstructorInHierarchy(class *ssa.Blueprint, constructor *ssa.Function) bool {
	if class == nil || constructor == nil {
		return false
	}
	seen := make(map[*ssa.Blueprint]struct{})
	for current := class; current != nil; current = current.GetSuperBlueprint() {
		if _, ok := seen[current]; ok {
			return false
		}
		seen[current] = struct{}{}
		current.Build()
		if constructor.GetMethodName() == current.Name {
			return true
		}
	}
	return false
}

func (b *singleFileBuilder) declaredInstanceMethod(obj ssa.Value, name string) (ssa.Value, bool) {
	if utils.IsNil(obj) || name == "" {
		return nil, false
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || bp == nil {
		return nil, false
	}
	// The blueprint container denotes `Type.Member`, not an instance method
	// group. Static members must continue through the ordinary member reader.
	if container := bp.Container(); !utils.IsNil(container) && container.GetId() == obj.GetId() {
		return nil, false
	}
	method := bp.GetNormalMethod(name)
	return method, !utils.IsNil(method)
}

func (b *singleFileBuilder) hasDeclaredInstanceField(obj ssa.Value, name string) bool {
	if utils.IsNil(obj) || name == "" {
		return false
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || bp == nil {
		return false
	}
	if container := bp.Container(); !utils.IsNil(container) && container.GetId() == obj.GetId() {
		return false
	}
	return !utils.IsNil(bp.GetNormalMember(name))
}

// readCallableMember resolves an instance member in call position. An unknown
// member on an any-typed receiver is still a valid dynamic member, and its
// synthetic function type must be marked as a method so EmitCall supplies the
// receiver. Declared normal methods keep a member proxy (for readable member
// syntax) which points at the real function (for interprocedural data flow).
func (b *singleFileBuilder) readCallableMember(obj ssa.Value, name string) ssa.Value {
	if name == "" {
		return obj
	}
	if utils.IsNil(obj) {
		return b.EmitUndefined(name)
	}
	key := b.EmitConstInstPlaceholder(name)
	// Blueprint member slots are shared by every instance. Returning that shared
	// placeholder and attaching an object/key pair to it makes the first receiver
	// permanent: every later call then receives that first object as `this`.
	// Manufacture a fresh proxy for each declared instance-method access instead.
	// The proxy keeps the concrete receiver while Point preserves interprocedural
	// flow into the one shared method implementation.
	if method, ok := b.declaredInstanceMethod(obj, name); ok {
		return b.bindInstanceMethod(obj, name, method)
	}
	declaredField := b.hasDeclaredInstanceField(obj, name)
	v := b.ReadMemberCallMethodOrValue(obj, key)
	if utils.IsNil(v) {
		return b.EmitUndefined(name)
	}
	// A delegate stored in a declared field is already the callable value. It is
	// not a method member and therefore must not receive the containing object as
	// an implicit first argument, even when its lowered type is currently `any`.
	if declaredField {
		return v
	}
	if !v.IsMember() {
		obj.AddMember(key, v)
		ssa.AddObjectKeyPair(v, obj, key)
		if user, ok := obj.(ssa.User); ok {
			key.AddUser(user)
		}
	}
	if _, ok := ssa.ToFunctionType(v.GetType()); !ok {
		ft := ssa.NewFunctionTypeDefine(name, nil, nil, false)
		ft.SetIsMethod(true, obj.GetType())
		v.SetType(ft)
	}
	return v
}

// bindInstanceMethod creates a per-access method proxy for a specific method
// implementation.  Keeping this separate from lookup is important for
// `base.M()`: lookup must start at the direct parent, while the receiver must
// remain the current derived instance.
func (b *singleFileBuilder) bindInstanceMethod(obj ssa.Value, name string, method ssa.Value) ssa.Value {
	if utils.IsNil(obj) || name == "" || utils.IsNil(method) {
		return nil
	}
	key := b.EmitConstInstPlaceholder(name)
	proxy := b.EmitUndefined(name)
	proxy.Kind = ssa.UndefinedMemberValid
	proxy.SetType(method.GetType())
	obj.AddMember(key, proxy)
	ssa.AddObjectKeyPair(proxy, obj, key)
	if user, ok := obj.(ssa.User); ok {
		key.AddUser(user)
	}
	ssa.Point(proxy, method)
	b.restoreMemberVerboseName(proxy)
	return proxy
}

func (b *singleFileBuilder) directBaseBlueprint() *ssa.Blueprint {
	class := b.MarkedThisClassBlueprint
	if class == nil {
		if this := b.thisValue(); !utils.IsNil(this) {
			class, _ = ssa.ToBluePrintType(this.GetType())
		}
	}
	if class == nil {
		return nil
	}
	class.Build()
	parent := class.GetSuperBlueprint()
	if parent != nil {
		parent.Build()
	}
	return parent
}

func (b *singleFileBuilder) readBaseCallableMember(name string) ssa.Value {
	this := b.thisValue()
	parent := b.directBaseBlueprint()
	if utils.IsNil(this) || parent == nil {
		return b.readCallableMember(this, name)
	}
	if method := parent.GetNormalMethod(name); !utils.IsNil(method) {
		return b.markNonVirtualCallTarget(b.bindInstanceMethod(this, name, method))
	}

	// Preserve an unresolved metadata method as a method-shaped member of the
	// same receiver, without falling back through the derived method table.
	key := b.EmitConstInstPlaceholder(name)
	proxy := b.EmitUndefined(name)
	proxy.Kind = ssa.UndefinedMemberValid
	functionType := ssa.NewFunctionTypeDefine(name, nil, nil, false)
	functionType.SetIsMethod(true, parent)
	proxy.SetType(functionType)
	this.AddMember(key, proxy)
	ssa.AddObjectKeyPair(proxy, this, key)
	b.restoreMemberVerboseName(proxy)
	return b.markNonVirtualCallTarget(proxy)
}

func (b *singleFileBuilder) markNonVirtualCallTarget(target ssa.Value) ssa.Value {
	if utils.IsNil(target) {
		return target
	}
	if b.nonVirtualCallTargets == nil {
		b.nonVirtualCallTargets = make(map[int64]struct{})
	}
	b.nonVirtualCallTargets[target.GetId()] = struct{}{}
	return target
}

func (b *singleFileBuilder) isNonVirtualCallTarget(target ssa.Value) bool {
	if utils.IsNil(target) || b.nonVirtualCallTargets == nil {
		return false
	}
	_, ok := b.nonVirtualCallTargets[target.GetId()]
	return ok
}

func (b *singleFileBuilder) emitBaseAccessor(name string, args []ssa.Value) (ssa.Value, bool) {
	this := b.thisValue()
	parent := b.directBaseBlueprint()
	if utils.IsNil(this) || parent == nil {
		return nil, false
	}
	method := parent.GetNormalMethod(name)
	if utils.IsNil(method) {
		return nil, false
	}
	callee := b.bindInstanceMethod(this, name, method)
	if utils.IsNil(callee) {
		return nil, false
	}
	arguments := make([]csharpEvaluatedArgument, 0, len(args))
	for _, value := range args {
		arguments = append(arguments, csharpEvaluatedArgument{value: value})
	}
	return b.emitDetailedBaseCall(callee, arguments, name), true
}

func (b *singleFileBuilder) emitBaseCall(callee ssa.Value, args []ssa.Value, outs []outArgument, fallbackName string) ssa.Value {
	if utils.IsNil(callee) {
		callee = b.EmitUndefined(fallbackName)
	}
	invocation := b.NewCall(callee, args)
	invocation.IsNonVirtual = true
	call := b.EmitCall(invocation)
	if utils.IsNil(call) {
		return b.EmitUndefined(fallbackName)
	}
	b.projectReturnedConstructorState(call, callee)
	b.bindOutArguments(call, outs)
	return call
}

func (b *singleFileBuilder) emitDetailedBaseCall(callee ssa.Value, arguments []csharpEvaluatedArgument, fallbackName string) ssa.Value {
	if result, ok := b.emitAmbiguousDetailedMethodCall(callee, arguments, fallbackName, true); ok {
		return result
	}
	selected, args, outs := b.prepareDetailedCall(callee, arguments)
	result := b.emitBaseCall(selected, args, outs, fallbackName)
	b.projectMethodExplicitWrites(result, selected, nil)
	return result
}

func isBaseAccessPrimary(raw csharpparser.IPrimary_expressionContext) bool {
	i, _ := raw.(*csharpparser.Primary_expressionContext)
	return i != nil && i.Base_access() != nil
}

// declaredAccessorCallee resolves a C# property/indexer accessor without
// manufacturing one for fields or auto-properties. Instance accessors are read
// through a member proxy so EmitCall inserts the receiver exactly once; static
// accessors are ordinary functions and therefore receive no synthetic receiver.
func (b *singleFileBuilder) declaredAccessorCallee(obj ssa.Value, name string) (ssa.Value, bool) {
	if utils.IsNil(obj) || name == "" {
		return nil, false
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || bp == nil {
		return nil, false
	}
	if container := bp.Container(); !utils.IsNil(container) && container.GetId() == obj.GetId() {
		method := bp.GetStaticMethod(name)
		return method, !utils.IsNil(method)
	}
	if utils.IsNil(bp.GetNormalMethod(name)) {
		return nil, false
	}
	method := b.readCallableMember(obj, name)
	return method, !utils.IsNil(method)
}

func (b *singleFileBuilder) emitDeclaredAccessor(obj ssa.Value, name string, args []ssa.Value) (ssa.Value, bool) {
	callee, ok := b.declaredAccessorCallee(obj, name)
	if !ok {
		return nil, false
	}
	arguments := make([]csharpEvaluatedArgument, 0, len(args))
	for _, value := range args {
		arguments = append(arguments, csharpEvaluatedArgument{value: value})
	}
	return b.emitDetailedCall(callee, arguments, name), true
}

func assignmentAccessorName(raw csharpparser.IPrimary_expressionContext) (string, bool) {
	i, _ := raw.(*csharpparser.Primary_expressionContext)
	if i == nil {
		return "", false
	}
	if i.Primary_expression() != nil {
		if i.TK_LBRACK() != nil {
			return "Item", true
		}
		if i.Identifier() != nil && (i.TK_DOT() != nil || i.TK_MINUS_GT() != nil) {
			return identText(i.Identifier()), false
		}
	}
	if sn, _ := i.Simple_name().(*csharpparser.Simple_nameContext); sn != nil {
		return identText(sn.Identifier()), false
	}
	if ba, _ := i.Base_access().(*csharpparser.Base_accessContext); ba != nil {
		if ba.TK_LBRACK() != nil {
			return "Item", true
		}
		return identText(ba.Identifier()), false
	}
	if i.Predefined_type() != nil || i.Qualified_alias_member() != nil {
		return identText(i.Identifier()), false
	}
	return "", false
}

func (b *singleFileBuilder) assignmentAccessorCall(
	raw csharpparser.IPrimary_expressionContext,
	variable *ssa.Variable,
	prefix string,
	value ssa.Value,
) (ssa.Value, bool) {
	if variable == nil || !variable.IsMemberCall() {
		return nil, false
	}
	obj, key := variable.GetMemberCall()
	if utils.IsNil(obj) || utils.IsNil(key) {
		return nil, false
	}
	name, indexer := assignmentAccessorName(raw)
	if name == "" {
		return nil, false
	}
	var args []ssa.Value
	if indexer {
		args = append(args, b.unpackIndexerArguments(key)...)
	}
	if !utils.IsNil(value) {
		args = append(args, value)
	}
	if isBaseAccessPrimary(raw) {
		return b.emitBaseAccessor(prefix+name, args)
	}
	return b.emitDeclaredAccessor(obj, prefix+name, args)
}

func (b *singleFileBuilder) readAssignmentValue(raw csharpparser.IPrimary_expressionContext, variable *ssa.Variable) ssa.Value {
	if value, ok := b.assignmentAccessorCall(raw, variable, "get_", nil); ok {
		return value
	}
	return b.ReadValueByVariable(variable)
}

func (b *singleFileBuilder) emitAssignmentSetter(raw csharpparser.IPrimary_expressionContext, variable *ssa.Variable, value ssa.Value) {
	_, _ = b.assignmentAccessorCall(raw, variable, "set_", value)
}

func tupleItemOrdinal(name string) (int, bool) {
	if !strings.HasPrefix(name, "Item") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(name, "Item"))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n - 1, true
}

func (b *singleFileBuilder) readTupleItem(obj ssa.Value, name string) ssa.Value {
	idx, ok := tupleItemOrdinal(name)
	if !ok || utils.IsNil(obj) {
		return nil
	}
	key := b.EmitConstInst(idx)
	if _, exists := ssa.GetLatestMemberByKey(obj, key); !exists {
		return nil
	}
	return b.ReadMemberCallValue(obj, key)
}

func (b *singleFileBuilder) restoreMemberVerboseName(v ssa.Value) {
	if utils.IsNil(v) || !v.IsMember() {
		return
	}
	if _, ok := ssa.ToUndefined(v); !ok {
		return
	}
	obj := ssa.GetLatestObject(v)
	key := ssa.GetLatestKey(v)
	if utils.IsNil(obj) || utils.IsNil(key) {
		return
	}
	objName := obj.GetVerboseName()
	if objName == "" {
		if variable := obj.GetLastVariable(); variable != nil {
			objName = variable.GetName()
		}
	}
	if objName == "" {
		return
	}
	keyText := ssa.NewFullDisasmLiner(100).DisasmValue(key)
	if keyText == "" {
		keyText = ssa.GetKeyString(key)
	}
	if keyText == "" {
		return
	}
	if c, ok := ssa.ToConstInst(key); ok && c.IsString() {
		v.SetVerboseName(objName + "." + c.String())
		return
	}
	if _, ok := ssa.ToUnOp(key); ok {
		v.SetVerboseName(objName + "." + keyText)
		return
	}
	v.SetVerboseName(objName + "[" + keyText + "]")
}

func isRangeIndexCall(v ssa.Value) bool {
	call, ok := ssa.ToCall(v)
	if !ok {
		return false
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || utils.IsNil(method) {
		return false
	}
	un, ok := ssa.ToUndefined(method)
	return ok && un.GetName() == "range"
}

func findRangeExpression(node antlr.Tree) *csharpparser.Range_expressionContext {
	if node == nil {
		return nil
	}
	if r, ok := node.(*csharpparser.Range_expressionContext); ok {
		return r
	}
	for _, child := range node.GetChildren() {
		if r := findRangeExpression(child); r != nil {
			return r
		}
	}
	return nil
}

func (b *singleFileBuilder) visitRangeIndex(i *csharpparser.Range_expressionContext) ssa.Value {
	if i == nil {
		return nil
	}
	unis := i.AllUnary_expression()
	visitUnary := func(raw csharpparser.IUnary_expressionContext) ssa.Value {
		u, _ := raw.(*csharpparser.Unary_expressionContext)
		if u != nil && u.TK_XOR() != nil && u.Unary_expression() != nil {
			inner := b.VisitUnaryExpression(u.Unary_expression())
			if utils.IsNil(inner) {
				inner = b.EmitUndefined(u.Unary_expression().GetText())
			}
			neg := ssa.NewUnOp(ssa.OpNeg, inner)
			b.SetInstructionPosition(neg)
			b.EmitOnly(neg)
			return neg
		}
		return b.VisitUnaryExpression(raw)
	}
	if i.TK_DOT_DOT() == nil {
		if len(unis) == 0 {
			return nil
		}
		return visitUnary(unis[0])
	}
	args := make([]ssa.Value, 0, len(unis))
	for _, u := range unis {
		if v := visitUnary(u); !utils.IsNil(v) {
			args = append(args, v)
		}
	}
	return b.emitCall(b.EmitUndefined("range"), args, nil, "range")
}

// visitElementArguments preserves the C# index/range meaning which the generic
// expression visitor intentionally flattens (`^n` and `a..b`).
func (b *singleFileBuilder) visitElementArguments(raw csharpparser.IArgument_listContext) []ssa.Value {
	i, _ := raw.(*csharpparser.Argument_listContext)
	if i == nil {
		return nil
	}
	values := make([]ssa.Value, 0, len(i.AllArgument()))
	for _, a := range i.AllArgument() {
		ac, _ := a.(*csharpparser.ArgumentContext)
		if ac == nil {
			continue
		}
		av, _ := ac.Argument_value().(*csharpparser.Argument_valueContext)
		if av == nil || av.Expression() == nil {
			continue
		}
		var v ssa.Value
		if r := findRangeExpression(av.Expression()); r != nil {
			v = b.visitRangeIndex(r)
		} else {
			v = b.VisitExpression(av.Expression())
		}
		if utils.IsNil(v) {
			v = b.EmitUndefined(av.GetText())
		}
		values = append(values, v)
	}
	return values
}

func (b *singleFileBuilder) elementValue(obj ssa.Value, keys []ssa.Value) ssa.Value {
	if utils.IsNil(obj) {
		return b.EmitUndefined("element")
	}
	if len(keys) == 0 {
		keys = []ssa.Value{b.EmitConstInst(0)}
	}
	if callee, ok := b.declaredAccessorCallee(obj, "get_Item"); ok {
		for _, key := range keys {
			b.ensureIndexerElement(obj, key)
		}
		arguments := make([]csharpEvaluatedArgument, 0, len(keys))
		for _, key := range keys {
			arguments = append(arguments, csharpEvaluatedArgument{value: key})
		}
		return b.emitDetailedCall(callee, arguments, "get_Item")
	}
	for _, k := range keys {
		if utils.IsNil(k) {
			k = b.EmitConstInst(0)
		}
		b.ensureIndexerElement(obj, k)
		obj = b.ReadMemberCallValue(obj, k)
		if utils.IsNil(obj) {
			return b.EmitUndefined("element")
		}
		if isRangeIndexCall(k) {
			if un, ok := ssa.ToUndefined(obj); ok {
				un.Kind = ssa.UndefinedMemberInValid
			}
		}
		b.restoreMemberVerboseName(obj)
	}
	return obj
}

func (b *singleFileBuilder) elementVariable(obj ssa.Value, keys []ssa.Value) *ssa.Variable {
	if utils.IsNil(obj) {
		return nil
	}
	if len(keys) == 0 {
		keys = []ssa.Value{b.EmitConstInst(0)}
	}
	// A C# indexer can take multiple parameters. They are arguments to one
	// get_Item/set_Item invocation, not chained element accesses. Keep them in a
	// small internal bundle while the generic assignment path carries only an
	// object/key variable; assignmentAccessorCall expands the bundle again.
	if len(keys) > 1 && b.hasDeclaredIndexer(obj) {
		key := b.packIndexerArguments(keys)
		b.ensureIndexerElement(obj, key)
		return b.CreateMemberCallVariable(obj, key)
	}
	for idx, k := range keys {
		if utils.IsNil(k) {
			k = b.EmitConstInst(0)
		}
		if idx == len(keys)-1 {
			b.ensureIndexerElement(obj, k)
			return b.CreateMemberCallVariable(obj, k)
		}
		b.ensureIndexerElement(obj, k)
		obj = b.ReadMemberCallValue(obj, k)
		if utils.IsNil(obj) {
			return nil
		}
	}
	return nil
}

const csharpIndexerArgumentsName = "$csharp_indexer_arguments"

func (b *singleFileBuilder) hasDeclaredIndexer(obj ssa.Value) bool {
	if utils.IsNil(obj) {
		return false
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	return ok && bp != nil && (!utils.IsNil(bp.GetNormalMethod("get_Item")) || !utils.IsNil(bp.GetNormalMethod("set_Item")))
}

func (b *singleFileBuilder) packIndexerArguments(keys []ssa.Value) ssa.Value {
	if len(keys) == 1 {
		return keys[0]
	}
	packed := b.EmitEmptyContainer()
	packed.SetName(csharpIndexerArgumentsName)
	packed.SetVerboseName(csharpIndexerArgumentsName)
	for index, key := range keys {
		if utils.IsNil(key) {
			key = b.EmitUndefined("index")
		}
		packed.AddMember(b.EmitConstInst(index), key)
	}
	return packed
}

func (b *singleFileBuilder) unpackIndexerArguments(key ssa.Value) []ssa.Value {
	if utils.IsNil(key) {
		return nil
	}
	if key.GetName() != csharpIndexerArgumentsName {
		return []ssa.Value{key}
	}
	values := make([]ssa.Value, 0, len(key.GetAllMember()))
	for index := 0; index < len(key.GetAllMember()); index++ {
		value, ok := ssa.GetLatestMemberByKey(key, b.EmitConstInst(index))
		if !ok || utils.IsNil(value) {
			value = b.EmitUndefined("index")
		}
		values = append(values, value)
	}
	return values
}

// ensureIndexerElement marks an arbitrary key as valid when the receiver type
// declares a C# indexer (compiled as get_Item/set_Item). The SSA object model
// otherwise validates blueprint members by their literal key and would report
// `counter[0]` as a missing field even though the indexer accepts that key.
func (b *singleFileBuilder) ensureIndexerElement(obj, key ssa.Value) {
	if utils.IsNil(obj) || utils.IsNil(key) {
		return
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || bp == nil || (utils.IsNil(bp.GetNormalMethod("get_Item")) && utils.IsNil(bp.GetNormalMethod("set_Item"))) {
		return
	}
	if _, exists := ssa.GetLatestMemberByKey(obj, key); exists {
		return
	}
	placeholder := b.EmitUndefined("item")
	if un, ok := ssa.ToUndefined(placeholder); ok {
		un.Kind = ssa.UndefinedMemberValid
	}
	obj.AddMember(key, placeholder)
	ssa.AddObjectKeyPair(placeholder, obj, key)
}

func (b *singleFileBuilder) visitMemberAccess(i *csharpparser.Primary_expressionContext, inner csharpparser.IPrimary_expressionContext) ssa.Value {
	if chain := b.identifierChain(i); chain != nil {
		return b.resolveIdentifierChain(chain, i)
	}
	obj := b.VisitPrimaryExpression(inner)
	return b.readMember(obj, identText(i.Identifier()), false)
}

func (b *singleFileBuilder) isStaticMemberCallee(raw csharpparser.IPrimary_expressionContext) bool {
	pe, _ := raw.(*csharpparser.Primary_expressionContext)
	if pe == nil {
		return false
	}
	if pe.Predefined_type() != nil || pe.Qualified_alias_member() != nil {
		return true
	}
	inner := pe.Primary_expression()
	if inner == nil || pe.Identifier() == nil || (pe.TK_DOT() == nil && pe.TK_MINUS_GT() == nil) {
		return false
	}
	chain := b.identifierChain(inner)
	if len(chain) == 0 {
		return false
	}
	// Classification must not evaluate the receiver. In particular,
	// readClassMemberValue emits a declared property getter; calling it here and
	// then resolving the real callee would execute `Current` twice in
	// `Current.Execute(...)`.
	if v, found := b.peekStaticClassificationValue(chain[0]); found {
		if bp, ok := ssa.ToBluePrintType(v.GetType()); ok && bp != nil {
			if container := bp.Container(); !utils.IsNil(container) && container.GetId() == v.GetId() {
				return true
			}
		}
		return false
	}
	if b.hasUsingAlias(chain[0]) {
		return true
	}
	for k := len(chain); k >= 1; k-- {
		if b.lookupBlueprintByPathStrict(chain[:k]) != nil {
			return true
		}
	}
	return looksLikeTypeName(chain[0]) || b.knownNamespacePrefixLen(chain) > 0 || b.isNamespacePrefix(chain[0])
}

// peekStaticClassificationValue mirrors the existence/precedence portion of
// bare-identifier lookup without invoking accessors or manufacturing values.
func (b *singleFileBuilder) peekStaticClassificationValue(name string) (ssa.Value, bool) {
	if v := b.PeekValue(name); !utils.IsNil(v) {
		return v, true
	}
	if class := b.MarkedThisClassBlueprint; class != nil {
		for bp := class; bp != nil; bp = b.outerBlueprintOf(bp) {
			if method := bp.GetStaticMethod(name); !utils.IsNil(method) {
				return method, true
			}
			if member := bp.GetNormalMember(name); !utils.IsNil(member) {
				return member, true
			}
			if method := bp.GetNormalMethod(name); !utils.IsNil(method) {
				return method, true
			}
			if member := bp.GetStaticMember(name); !utils.IsNil(member) {
				return member, true
			}
			if constant := bp.GetConstMember(name); !utils.IsNil(constant) {
				return constant, true
			}
		}
	}
	if v, ok := b.constMap[name]; ok && !utils.IsNil(v) {
		return v, true
	}
	if prog := b.GetProgram(); prog != nil {
		if v, ok := prog.ReadImportValue(name); ok && !utils.IsNil(v) {
			return v, true
		}
	}
	return nil, false
}

// unknownStaticTypeReceiver keeps an unresolved source type recognizable as an
// undefined value. Declared/imported types continue to use their blueprint
// container so real static methods and members can be resolved normally.
func (b *singleFileBuilder) unknownStaticTypeReceiver(obj ssa.Value) ssa.Value {
	if utils.IsNil(obj) {
		return obj
	}
	bp, ok := ssa.ToBluePrintType(obj.GetType())
	if !ok || bp == nil {
		return obj
	}
	if b.constructors.isDeclared(bp) {
		return obj
	}
	container := bp.Container()
	if utils.IsNil(container) || container.GetId() != obj.GetId() {
		return obj
	}
	// Keep the user-facing/static-member receiver at its C# type segment while
	// retaining the fully-qualified blueprint on the value's type. This preserves
	// queries such as `File.ReadAllText` without collapsing `A.File` and `B.File`.
	unknown := b.EmitUndefined(bp.Name)
	shortName := lastDotSegment(bp.Name)
	unknown.SetVerboseName(shortName)
	unknown.SetType(bp)
	if prog := b.GetProgram(); prog != nil && shortName != "" {
		// The blueprint remains keyed by its fully-qualified identity, but the
		// source-level terminal segment must stay searchable as a value name.
		prog.SetInstructionWithName(shortName, unknown)
	}
	return unknown
}

// visitCallee resolves the callee of `callee(args)`; member callees prefer method slots.
func (b *singleFileBuilder) visitCallee(raw csharpparser.IPrimary_expressionContext) ssa.Value {
	pe, _ := raw.(*csharpparser.Primary_expressionContext)
	if pe == nil {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	if sn, _ := pe.Simple_name().(*csharpparser.Simple_nameContext); sn != nil {
		name := identText(sn.Identifier())
		if v := b.PeekValue(name); !utils.IsNil(v) {
			return v
		}
		if class := b.MarkedThisClassBlueprint; class != nil {
			if this := b.PeekValue("this"); !utils.IsNil(this) {
				if utils.IsNil(class.GetStaticMethod(name)) && !utils.IsNil(class.GetNormalMethod(name)) {
					return b.ReadMemberCallMethod(this, b.EmitConstInstPlaceholder(name))
				}
			}
		}
		return b.ReadIdentifierValue(name, sn)
	}
	if inner := pe.Primary_expression(); inner != nil && pe.TK_QMARK() == nil && pe.Identifier() != nil && (pe.TK_DOT() != nil || pe.TK_MINUS_GT() != nil) {
		name := identText(pe.Identifier())
		staticCall := b.isStaticMemberCallee(pe)
		var obj ssa.Value
		if chain := b.identifierChain(inner); chain != nil {
			obj = b.resolveIdentifierChain(chain, inner)
		} else {
			obj = b.VisitPrimaryExpression(inner)
		}
		if staticCall {
			obj = b.unknownStaticTypeReceiver(obj)
			return b.readMember(obj, name, true)
		}
		return b.readCallableMember(obj, name)
	}
	if pe.Predefined_type() != nil && pe.Identifier() != nil {
		obj := b.predefinedTypeContainer(pe.Predefined_type().GetText(), pe.Predefined_type())
		return b.readMember(obj, identText(pe.Identifier()), true)
	}
	if pe.Qualified_alias_member() != nil && pe.Identifier() != nil {
		segments := b.qualifiedAliasSegments(pe.Qualified_alias_member())
		obj := b.resolveIdentifierChain(segments, pe)
		return b.readMember(obj, identText(pe.Identifier()), true)
	}
	if ba, _ := pe.Base_access().(*csharpparser.Base_accessContext); ba != nil && ba.Identifier() != nil {
		return b.readBaseCallableMember(identText(ba.Identifier()))
	}
	return b.VisitPrimaryExpression(raw)
}

func (b *singleFileBuilder) visitInvocation(callee csharpparser.IPrimary_expressionContext, argList csharpparser.IArgument_listContext) ssa.Value {
	simpleName := b.simpleCalleeName(callee)
	if simpleName == "nameof" {
		return b.EmitConstInst(nameofArgument(argList))
	}
	localCallee := simpleName != "" && !utils.IsNil(b.PeekValue(simpleName))
	staticCall := b.isStaticMemberCallee(callee)
	target := b.visitCallee(callee)
	arguments := b.visitArgumentDetails(argList)
	// A synthetic missing member is method-shaped by the generic member reader.
	// For a static call with explicit arguments, correct that shape before the
	// call is emitted so call-side use chains and side effects see the final args.
	if staticCall && len(arguments) > 0 {
		if un, ok := ssa.ToUndefined(target); ok && un.Kind == ssa.UndefinedMemberInValid {
			if ft, ok := ssa.ToFunctionType(target.GetType()); ok && ft.IsMethod {
				target.SetType(ssa.NewFunctionTypeDefine(target.GetName(), nil, nil, false))
			}
		}
	}
	if isBaseAccessPrimary(callee) || b.isNonVirtualCallTarget(target) {
		return b.emitDetailedBaseCall(target, arguments, callee.GetText())
	}
	if simpleName != "" && !localCallee && b.MarkedThisClassBlueprint != nil {
		return b.emitDetailedBareCall(target, b.MarkedThisClassBlueprint, simpleName, b.PeekValue("this"), arguments)
	}
	return b.emitDetailedCall(target, arguments, callee.GetText())
}

func (b *singleFileBuilder) simpleCalleeName(raw csharpparser.IPrimary_expressionContext) string {
	pe, _ := raw.(*csharpparser.Primary_expressionContext)
	if pe == nil {
		return ""
	}
	sn, _ := pe.Simple_name().(*csharpparser.Simple_nameContext)
	if sn == nil || sn.Type_argument_list() != nil {
		return ""
	}
	return identText(sn.Identifier())
}

func nameofArgument(raw csharpparser.IArgument_listContext) string {
	i, _ := raw.(*csharpparser.Argument_listContext)
	if i == nil || len(i.AllArgument()) == 0 {
		return ""
	}
	ac, _ := i.AllArgument()[0].(*csharpparser.ArgumentContext)
	if ac == nil {
		return ""
	}
	av, _ := ac.Argument_value().(*csharpparser.Argument_valueContext)
	if av == nil || av.Expression() == nil {
		return ""
	}
	name := av.Expression().GetText()
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, "::"); idx >= 0 {
		name = name[idx+2:]
	}
	return name
}

// visitNullConditional handles `a?.b...` / `a?[i]...` with trailing dependent accesses.
func (b *singleFileBuilder) visitNullConditional(i *csharpparser.Primary_expressionContext, inner csharpparser.IPrimary_expressionContext) ssa.Value {
	obj := b.VisitPrimaryExpression(inner)
	cond := b.EmitBinOp(ssa.OpNotEq, obj, b.EmitConstInstNil())
	var accessed ssa.Value
	value := b.emitTernary(cond, func() ssa.Value {
		deps := i.AllDependent_access()
		var cur ssa.Value
		if i.TK_DOT() != nil && i.Identifier() != nil {
			name := identText(i.Identifier())
			if dependentIsInvocation(deps, 0) {
				cur = b.readCallableMember(obj, name)
			} else {
				cur = b.readMember(obj, name, false)
			}
		} else if i.TK_LBRACK() != nil {
			cur = b.elementValue(obj, b.visitElementArguments(i.Argument_list()))
		} else {
			cur = obj
		}
		accessed = b.applyDependentAccesses(cur, deps)
		return accessed
	}, func() ssa.Value { return b.EmitConstInstNil() })
	b.restoreMemberVerboseName(accessed)
	return value
}

func dependentIsInvocation(deps []csharpparser.IDependent_accessContext, idx int) bool {
	if idx >= len(deps) {
		return false
	}
	dc, _ := deps[idx].(*csharpparser.Dependent_accessContext)
	return dc != nil && dc.TK_LPAREN() != nil
}

func (b *singleFileBuilder) applyDependentAccesses(cur ssa.Value, deps []csharpparser.IDependent_accessContext) ssa.Value {
	for idx, d := range deps {
		dc, _ := d.(*csharpparser.Dependent_accessContext)
		if dc == nil {
			continue
		}
		switch {
		case dc.TK_DOT() != nil:
			name := identText(dc.Identifier())
			if dependentIsInvocation(deps, idx+1) {
				cur = b.readCallableMember(cur, name)
			} else {
				cur = b.readMember(cur, name, false)
			}
		case dc.TK_LBRACK() != nil:
			cur = b.elementValue(cur, b.visitElementArguments(dc.Argument_list()))
		case dc.TK_LPAREN() != nil:
			arguments := b.visitArgumentDetails(dc.Argument_list())
			cur = b.emitDetailedCall(cur, arguments, dc.GetText())
		}
	}
	return cur
}

// VisitNullConditionalInvocationExpression handles `a?.b(args)` / `a?[i](args)` in statement / body position.
func (b *singleFileBuilder) VisitNullConditionalInvocationExpression(raw csharpparser.INull_conditional_invocation_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Null_conditional_invocation_expressionContext)
	if !ok || i == nil {
		return nil
	}
	var obj ssa.Value
	var invoke func() ssa.Value
	if m, _ := i.Null_conditional_member_access().(*csharpparser.Null_conditional_member_accessContext); m != nil {
		obj = b.VisitPrimaryExpression(m.Primary_expression())
		invoke = func() ssa.Value {
			deps := m.AllDependent_access()
			cur := b.readCallableMember(obj, identText(m.Identifier()))
			cur = b.applyDependentAccesses(cur, deps)
			arguments := b.visitArgumentDetails(i.Argument_list())
			return b.emitDetailedCall(cur, arguments, i.GetText())
		}
	} else if e, _ := i.Null_conditional_element_access().(*csharpparser.Null_conditional_element_accessContext); e != nil {
		obj = b.VisitPrimaryExpression(e.Primary_expression())
		invoke = func() ssa.Value {
			cur := b.elementValue(obj, b.visitElementArguments(e.Argument_list()))
			cur = b.applyDependentAccesses(cur, e.AllDependent_access())
			arguments := b.visitArgumentDetails(i.Argument_list())
			return b.emitDetailedCall(cur, arguments, i.GetText())
		}
	}
	if utils.IsNil(obj) || invoke == nil {
		return b.EmitUndefined(i.GetText())
	}
	cond := b.EmitBinOp(ssa.OpNotEq, obj, b.EmitConstInstNil())
	return b.emitTernary(cond, invoke, func() ssa.Value { return b.EmitConstInstNil() })
}

// ---------------------------------------------------------------- this / base

func (b *singleFileBuilder) thisValue() ssa.Value {
	if v := b.PeekValue("this"); !utils.IsNil(v) {
		return v
	}
	return b.ReadValue("this")
}

func (b *singleFileBuilder) VisitBaseAccess(raw csharpparser.IBase_accessContext) ssa.Value {
	i, _ := raw.(*csharpparser.Base_accessContext)
	if i == nil {
		return nil
	}
	this := b.thisValue()
	if i.Identifier() != nil {
		name := identText(i.Identifier())
		if value, ok := b.emitBaseAccessor("get_"+name, nil); ok {
			return value
		}
		parent := b.directBaseBlueprint()
		if parent != nil {
			if method := parent.GetNormalMethod(name); !utils.IsNil(method) {
				return b.markNonVirtualCallTarget(b.bindInstanceMethod(this, name, method))
			}
		}
		if !utils.IsNil(this) {
			if value := b.ReadMemberCallValueByName(this, name); !utils.IsNil(value) {
				return value
			}
		}
		if parent != nil {
			if member := parent.GetNormalMember(name); !utils.IsNil(member) {
				key := b.EmitConstInstPlaceholder(name)
				if value := b.ReadMemberCallValue(this, key); !utils.IsNil(value) {
					return value
				}
				return member
			}
		}
		return b.readMember(this, name, false)
	}
	args := b.visitElementArguments(i.Argument_list())
	if value, ok := b.emitBaseAccessor("get_Item", args); ok {
		return value
	}
	return b.elementValue(this, args)
}

// ---------------------------------------------------------------- left values

// VisitPrimaryLeftValue resolves a primary_expression used as an assignment target.
func (b *singleFileBuilder) VisitPrimaryLeftValue(raw csharpparser.IPrimary_expressionContext) *ssa.Variable {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Primary_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if inner := i.Primary_expression(); inner != nil {
		switch {
		case i.TK_QMARK() != nil:
			// `a?.b = x` 不是合法左值，退化为普通成员
			return b.CreateVariable(i.GetText())
		case i.Identifier() != nil && (i.TK_DOT() != nil || i.TK_MINUS_GT() != nil):
			name := identText(i.Identifier())
			var obj ssa.Value
			if chain := b.identifierChain(inner); chain != nil {
				obj = b.resolveIdentifierChain(chain, inner)
			} else {
				obj = b.VisitPrimaryExpression(inner)
			}
			if utils.IsNil(obj) {
				return b.CreateVariable(name)
			}
			return b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder(name))
		case i.TK_LBRACK() != nil:
			obj := b.VisitPrimaryExpression(inner)
			if v := b.elementVariable(obj, b.visitElementArguments(i.Argument_list())); v != nil {
				return v
			}
			return b.CreateVariable(i.GetText())
		}
		return b.CreateVariable(i.GetText())
	}
	if sn, _ := i.Simple_name().(*csharpparser.Simple_nameContext); sn != nil {
		return b.CreateIdentifierVariable(identText(sn.Identifier()), sn)
	}
	if i.This_access() != nil {
		return b.CreateVariable("this")
	}
	if ba, _ := i.Base_access().(*csharpparser.Base_accessContext); ba != nil {
		this := b.thisValue()
		if ba.Identifier() != nil {
			return b.CreateMemberCallVariable(this, b.EmitConstInstPlaceholder(identText(ba.Identifier())))
		}
		if v := b.elementVariable(this, b.visitElementArguments(ba.Argument_list())); v != nil {
			return v
		}
	}
	if pe, _ := i.Parenthesized_expression().(*csharpparser.Parenthesized_expressionContext); pe != nil {
		if p := b.unwrapToPrimary(pe.Expression()); p != nil {
			return b.VisitPrimaryLeftValue(p)
		}
	}
	if i.Predefined_type() != nil && i.Identifier() != nil {
		obj := b.predefinedTypeContainer(i.Predefined_type().GetText(), i.Predefined_type())
		return b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder(identText(i.Identifier())))
	}
	if i.Qualified_alias_member() != nil && i.Identifier() != nil {
		obj := b.resolveIdentifierChain(b.qualifiedAliasSegments(i.Qualified_alias_member()), i)
		if !utils.IsNil(obj) {
			return b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder(identText(i.Identifier())))
		}
	}
	return b.CreateVariable(i.GetText())
}

// ---------------------------------------------------------------- tuple / deconstruction

func (b *singleFileBuilder) VisitTupleExpression(raw csharpparser.ITuple_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Tuple_expressionContext)
	if i == nil {
		return nil
	}
	if d, _ := i.Deconstruction_expression().(*csharpparser.Deconstruction_expressionContext); d != nil {
		// `var (a, b)` in value position: declare placeholders
		tuple := b.EmitEmptyContainer()
		b.deconstructTuple(d.Deconstruction_tuple(), tuple)
		return tuple
	}
	tuple := b.EmitEmptyContainer()
	for idx, e := range i.AllTuple_element() {
		te, _ := e.(*csharpparser.Tuple_elementContext)
		if te == nil {
			continue
		}
		v := b.VisitExpression(te.Expression())
		if utils.IsNil(v) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(tuple, b.EmitConstInst(idx)), v)
		if name := identText(te.Identifier()); name != "" {
			b.AssignVariable(b.CreateMemberCallVariable(tuple, b.EmitConstInst(name)), v)
		}
	}
	return tuple
}

// deconstructAssign handles `(a, b) = value` and `var (a, b) = value`.
func (b *singleFileBuilder) deconstructAssign(raw csharpparser.ITuple_expressionContext, value ssa.Value) {
	i, _ := raw.(*csharpparser.Tuple_expressionContext)
	if i == nil {
		return
	}
	if d, _ := i.Deconstruction_expression().(*csharpparser.Deconstruction_expressionContext); d != nil {
		b.deconstructTuple(d.Deconstruction_tuple(), value)
		return
	}
	for idx, e := range i.AllTuple_element() {
		te, _ := e.(*csharpparser.Tuple_elementContext)
		if te == nil || te.Expression() == nil {
			continue
		}
		element := b.ReadMemberCallValue(value, b.EmitConstInst(idx))
		if utils.IsNil(element) {
			element = b.EmitUndefined("tuple")
		}
		primary := b.unwrapToPrimary(te.Expression())
		if pe, _ := primary.(*csharpparser.Primary_expressionContext); pe != nil && pe.Tuple_expression() != nil {
			b.deconstructAssign(pe.Tuple_expression(), element)
			continue
		}
		if te.Expression().GetText() == "_" {
			continue
		}
		if variable := b.leftValueVariable(te.Expression(), primary); variable != nil {
			element = b.applyVariableDeclaredType(variable, element)
			b.AssignVariable(variable, element)
		}
	}
}

// deconstructTuple binds `var (a, (b, c))` designations to value[idx].
func (b *singleFileBuilder) deconstructTuple(raw csharpparser.IDeconstruction_tupleContext, value ssa.Value) {
	i, _ := raw.(*csharpparser.Deconstruction_tupleContext)
	if i == nil {
		return
	}
	for idx, e := range i.AllDeconstruction_element() {
		de, _ := e.(*csharpparser.Deconstruction_elementContext)
		if de == nil {
			continue
		}
		element := b.ReadMemberCallValue(value, b.EmitConstInst(idx))
		if utils.IsNil(element) {
			element = b.EmitUndefined("tuple")
		}
		if de.Deconstruction_tuple() != nil {
			b.deconstructTuple(de.Deconstruction_tuple(), element)
			continue
		}
		name := identText(de.Identifier())
		if name == "" || name == "_" {
			continue
		}
		b.AssignVariable(b.CreateLocalVariable(name), element)
	}
}

// ---------------------------------------------------------------- object / array creation

func (b *singleFileBuilder) VisitObjectCreation(raw csharpparser.IObject_creation_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Object_creation_expressionContext)
	if !ok || i == nil {
		return nil
	}
	typ := b.VisitType(i.Type_())
	arguments := b.visitArgumentDetails(i.Argument_list())
	var obj ssa.Value
	if bp, ok := ssa.ToBluePrintType(typ); ok && bp != nil {
		b.ensureBlueprintConstructorSlot(bp)
		self := b.EmitUndefined(bp.Name)
		self.SetType(bp)
		call := b.emitConstructorCall(bp, self, arguments, nil, true)
		if utils.IsNil(call) {
			obj = self
		} else {
			call.SetType(bp)
			obj = call
		}
	} else {
		args, _ := flattenEvaluatedArguments(arguments)
		var make_ ssa.Value
		if typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
			make_ = b.EmitMakeWithoutType(nil, nil)
		} else {
			make_ = b.EmitMakeBuildWithType(typ, b.EmitConstInst(0), b.EmitConstInst(0))
		}
		obj = make_
		// 构造实参对 List<T>(collection) 等仍有数据流意义
		for idx, a := range args {
			if utils.IsNil(a) {
				continue
			}
			b.AssignVariable(b.CreateMemberCallVariable(obj, b.EmitConstInst(idx)), a)
		}
	}
	b.applyObjectOrCollectionInitializer(obj, i.Object_or_collection_initializer())
	return obj
}

func (b *singleFileBuilder) applyObjectOrCollectionInitializer(obj ssa.Value, raw csharpparser.IObject_or_collection_initializerContext) {
	i, _ := raw.(*csharpparser.Object_or_collection_initializerContext)
	if i == nil || utils.IsNil(obj) {
		return
	}
	if oi, _ := i.Object_initializer().(*csharpparser.Object_initializerContext); oi != nil {
		b.applyObjectInitializer(obj, oi)
		return
	}
	if ci, _ := i.Collection_initializer().(*csharpparser.Collection_initializerContext); ci != nil {
		b.applyCollectionInitializer(obj, ci)
	}
}

func (b *singleFileBuilder) applyObjectInitializer(obj ssa.Value, oi *csharpparser.Object_initializerContext) {
	list, _ := oi.Member_initializer_list().(*csharpparser.Member_initializer_listContext)
	if list == nil {
		return
	}
	for _, m := range list.AllMember_initializer() {
		mi, _ := m.(*csharpparser.Member_initializerContext)
		if mi == nil {
			continue
		}
		target, _ := mi.Initializer_target().(*csharpparser.Initializer_targetContext)
		if target == nil {
			continue
		}
		var key ssa.Value
		var keys []ssa.Value
		accessorName := ""
		if target.Identifier() != nil {
			accessorName = identText(target.Identifier())
			key = b.EmitConstInstPlaceholder(accessorName)
		} else {
			keys = b.VisitArgumentList(target.Argument_list())
			if len(keys) > 0 {
				key = b.packIndexerArguments(keys)
			}
		}
		if utils.IsNil(key) {
			continue
		}
		value := b.initializerValue(mi.Initializer_value())
		if utils.IsNil(value) {
			continue
		}
		if accessorName == "" {
			b.ensureIndexerElement(obj, key)
		}
		b.AssignVariable(b.CreateMemberCallVariable(obj, key), value)
		if accessorName != "" {
			_, _ = b.emitDeclaredAccessor(obj, "set_"+accessorName, []ssa.Value{value})
			continue
		}
		if len(keys) > 0 {
			args := append(append([]ssa.Value(nil), keys...), value)
			_, _ = b.emitDeclaredAccessor(obj, "set_Item", args)
		}
	}
}

func (b *singleFileBuilder) initializerValue(raw csharpparser.IInitializer_valueContext) ssa.Value {
	iv, _ := raw.(*csharpparser.Initializer_valueContext)
	if iv == nil {
		return nil
	}
	if iv.Expression() != nil {
		return b.VisitExpression(iv.Expression())
	}
	if iv.Object_or_collection_initializer() != nil {
		nested := b.EmitEmptyContainer()
		b.applyObjectOrCollectionInitializer(nested, iv.Object_or_collection_initializer())
		return nested
	}
	return nil
}

func (b *singleFileBuilder) applyCollectionInitializer(obj ssa.Value, ci *csharpparser.Collection_initializerContext) {
	list, _ := ci.Element_initializer_list().(*csharpparser.Element_initializer_listContext)
	if list == nil {
		return
	}
	for idx, e := range list.AllElement_initializer() {
		ei, _ := e.(*csharpparser.Element_initializerContext)
		if ei == nil {
			continue
		}
		if ei.Non_assignment_expression() != nil {
			v := b.VisitNonAssignmentExpression(ei.Non_assignment_expression())
			if !utils.IsNil(v) {
				b.emitCollectionAdd(obj, []ssa.Value{v})
				key := b.EmitConstInst(idx)
				b.ensureCollectionElement(obj, key)
				b.AssignVariable(b.CreateMemberCallVariable(obj, key), v)
			}
			continue
		}
		el, _ := ei.Expression_list().(*csharpparser.Expression_listContext)
		if el == nil {
			continue
		}
		exprs := el.AllExpression()
		values := make([]ssa.Value, len(exprs))
		for j, ex := range exprs {
			values[j] = b.VisitExpression(ex)
		}
		b.emitCollectionAdd(obj, values)
		if len(values) == 2 {
			// { key, value } for dictionaries
			k := values[0]
			v := values[1]
			if !utils.IsNil(k) && !utils.IsNil(v) {
				b.ensureCollectionElement(obj, k)
				b.AssignVariable(b.CreateMemberCallVariable(obj, k), v)
			}
			continue
		}
		tuple := b.EmitEmptyContainer()
		for j, v := range values {
			if utils.IsNil(v) {
				continue
			}
			b.AssignVariable(b.CreateMemberCallVariable(tuple, b.EmitConstInst(j)), v)
		}
		key := b.EmitConstInst(idx)
		b.ensureCollectionElement(obj, key)
		b.AssignVariable(b.CreateMemberCallVariable(obj, key), tuple)
	}
}

// ensureCollectionElement creates the synthetic storage slot used to model
// collection reads. A user-defined collection only needs Add(...) in C#; it
// need not expose an indexer, so collection-initializer storage cannot require
// get_Item/set_Item to be declared.
func (b *singleFileBuilder) ensureCollectionElement(obj, key ssa.Value) {
	if utils.IsNil(obj) || utils.IsNil(key) {
		return
	}
	if _, exists := ssa.GetLatestMemberByKey(obj, key); exists {
		return
	}
	placeholder := b.EmitUndefined("item")
	if un, ok := ssa.ToUndefined(placeholder); ok {
		un.Kind = ssa.UndefinedMemberValid
	}
	obj.AddMember(key, placeholder)
	ssa.AddObjectKeyPair(placeholder, obj, key)
}

// emitCollectionAdd models the invocation mandated by C# collection-initializer
// semantics. The already-evaluated values are reused by both this call and the
// synthetic index/map writes above, so each source expression executes once.
func (b *singleFileBuilder) emitCollectionAdd(obj ssa.Value, args []ssa.Value) {
	if utils.IsNil(obj) || len(args) == 0 {
		return
	}
	callee := b.readCallableMember(obj, "Add")
	_ = b.emitCall(callee, args, nil, "Add")
}

func (b *singleFileBuilder) VisitAnonymousObjectCreation(raw csharpparser.IAnonymous_object_creation_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Anonymous_object_creation_expressionContext)
	if i == nil {
		return nil
	}
	obj := b.EmitEmptyContainer()
	init, _ := i.Anonymous_object_initializer().(*csharpparser.Anonymous_object_initializerContext)
	if init == nil {
		return obj
	}
	list, _ := init.Member_declarator_list().(*csharpparser.Member_declarator_listContext)
	if list == nil {
		return obj
	}
	for _, d := range list.AllMember_declarator() {
		md, _ := d.(*csharpparser.Member_declaratorContext)
		if md == nil {
			continue
		}
		var key string
		var value ssa.Value
		switch {
		case md.Identifier() != nil && md.TK_EQ() != nil:
			key = identText(md.Identifier())
			value = b.VisitExpression(md.Expression())
		case md.Simple_name() != nil:
			sn, _ := md.Simple_name().(*csharpparser.Simple_nameContext)
			if sn != nil {
				key = identText(sn.Identifier())
				value = b.ReadIdentifierValue(key, sn)
			}
		case md.Member_access() != nil:
			ma, _ := md.Member_access().(*csharpparser.Member_accessContext)
			if ma != nil {
				key = identText(ma.Identifier())
				value = b.VisitMemberAccessContext(ma)
			}
		case md.Base_access() != nil:
			ba, _ := md.Base_access().(*csharpparser.Base_accessContext)
			if ba != nil {
				key = identText(ba.Identifier())
				value = b.VisitBaseAccess(ba)
			}
		case md.Null_conditional_projection_initializer() != nil:
			np, _ := md.Null_conditional_projection_initializer().(*csharpparser.Null_conditional_projection_initializerContext)
			if np != nil {
				key = identText(np.Identifier())
				value = b.readMember(b.VisitPrimaryExpression(np.Primary_expression()), key, false)
			}
		}
		if key == "" || utils.IsNil(value) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder(key)), value)
	}
	return obj
}

// VisitMemberAccessContext handles the standalone `member_access` rule (used by anonymous objects).
func (b *singleFileBuilder) VisitMemberAccessContext(i *csharpparser.Member_accessContext) ssa.Value {
	if i == nil {
		return nil
	}
	name := identText(i.Identifier())
	switch {
	case i.Primary_expression() != nil:
		if chain := b.identifierChain(i.Primary_expression()); chain != nil {
			return b.resolveIdentifierChain(append(chain, name), i)
		}
		return b.readMember(b.VisitPrimaryExpression(i.Primary_expression()), name, false)
	case i.Predefined_type() != nil:
		return b.readMember(b.predefinedTypeContainer(i.Predefined_type().GetText(), i.Predefined_type()), name, false)
	case i.Qualified_alias_member() != nil:
		return b.resolveIdentifierChain(append(b.qualifiedAliasSegments(i.Qualified_alias_member()), name), i)
	}
	return b.EmitUndefined(name)
}

func (b *singleFileBuilder) VisitArrayCreation(raw csharpparser.IArray_creation_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Array_creation_expressionContext)
	if !ok || i == nil {
		return nil
	}
	switch {
	case i.Non_array_type() != nil:
		elem := b.visitNonArrayType(i.Non_array_type())
		sliceTyp := ssa.NewSliceType(elem)
		if i.Array_initializer() != nil {
			return b.VisitArrayInitializer(i.Array_initializer(), sliceTyp)
		}
		var dims []ssa.Value
		if el, _ := i.Expression_list().(*csharpparser.Expression_listContext); el != nil {
			for _, e := range el.AllExpression() {
				dims = append(dims, b.VisitExpression(e))
			}
		}
		var size ssa.Value
		if len(dims) > 0 && !utils.IsNil(dims[0]) {
			size = dims[0]
		} else {
			size = b.EmitConstInst(0)
		}
		return b.EmitMakeBuildWithType(sliceTyp, size, size)
	case i.Array_type() != nil:
		return b.VisitArrayInitializer(i.Array_initializer(), b.visitArrayType(i.Array_type()))
	default:
		return b.VisitArrayInitializer(i.Array_initializer(), nil)
	}
}

// VisitArrayInitializer builds `{ a, b, c }` as a slice with indexed members.
func (b *singleFileBuilder) VisitArrayInitializer(raw csharpparser.IArray_initializerContext, typ ssa.Type) ssa.Value {
	if b == nil || b.IsStop() {
		return nil
	}
	if typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
		typ = ssa.NewSliceType(ssa.CreateAnyType())
	}
	i, _ := raw.(*csharpparser.Array_initializerContext)
	var items []csharpparser.IVariable_initializerContext
	if i != nil {
		if list, _ := i.Variable_initializer_list().(*csharpparser.Variable_initializer_listContext); list != nil {
			items = list.AllVariable_initializer()
		}
	}
	size := b.EmitConstInst(len(items))
	arr := b.EmitMakeBuildWithType(typ, size, size)
	if utils.IsNil(arr) {
		return b.EmitUndefined("array")
	}
	var elemTyp ssa.Type
	if ot, ok := ssa.ToObjectType(typ); ok && ot != nil {
		elemTyp = ot.FieldType
	}
	for idx, it := range items {
		var v ssa.Value
		vi, _ := it.(*csharpparser.Variable_initializerContext)
		if vi != nil && vi.Array_initializer() != nil {
			v = b.VisitArrayInitializer(vi.Array_initializer(), elemTyp)
		} else {
			v = b.VisitVariableInitializer(it)
		}
		if utils.IsNil(v) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(arr, b.EmitConstInst(idx)), v)
	}
	return arr
}

// ---------------------------------------------------------------- typeof / default / nameof

func (b *singleFileBuilder) VisitTypeofExpression(raw csharpparser.ITypeof_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Typeof_expressionContext)
	if i == nil {
		return nil
	}
	switch {
	case i.Type_() != nil:
		return b.emitCall(b.EmitUndefined("typeof"), []ssa.Value{b.EmitConstInst(i.Type_().GetText())}, nil, "typeof")
	case i.Unbound_type_name() != nil:
		return b.emitCall(b.EmitUndefined("typeof"), []ssa.Value{b.EmitConstInst(i.Unbound_type_name().GetText())}, nil, "typeof")
	}
	return b.EmitUndefined(i.GetText())
}

func (b *singleFileBuilder) VisitDefaultValueExpression(raw csharpparser.IDefault_value_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Default_value_expressionContext)
	if i == nil {
		return nil
	}
	if i.Explicitly_typed_default() == nil {
		return b.EmitConstInst(0)
	}
	value := b.EmitConstInstNil()
	if et, _ := i.Explicitly_typed_default().(*csharpparser.Explicitly_typed_defaultContext); et != nil && et.Type_() != nil {
		if typ := b.VisitType(et.Type_()); typ != nil && typ.GetTypeKind() != ssa.AnyTypeKind {
			switch typ.GetTypeKind() {
			case ssa.NumberTypeKind:
				return b.EmitConstInst(0)
			case ssa.BooleanTypeKind:
				return b.EmitConstInst(false)
			case ssa.StringTypeKind:
				return value
			}
			value.SetType(typ)
		}
	}
	return value
}

func (b *singleFileBuilder) VisitNameofExpression(raw csharpparser.INameof_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Nameof_expressionContext)
	if i == nil {
		return nil
	}
	ne, _ := i.Named_entity().(*csharpparser.Named_entityContext)
	if ne == nil {
		return b.EmitConstInst("")
	}
	if ids := ne.AllIdentifier(); len(ids) > 0 {
		return b.EmitConstInst(identText(ids[len(ids)-1]))
	}
	if t, _ := ne.Named_entity_target().(*csharpparser.Named_entity_targetContext); t != nil {
		if sn, _ := t.Simple_name().(*csharpparser.Simple_nameContext); sn != nil {
			return b.EmitConstInst(identText(sn.Identifier()))
		}
		return b.EmitConstInst(t.GetText())
	}
	return b.EmitConstInst(ne.GetText())
}
