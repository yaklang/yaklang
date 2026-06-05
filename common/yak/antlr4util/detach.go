package antlr4util

import (
	"reflect"
	"sync"
	"unsafe"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

const antlrRuntimeVersion = "github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20220911224424-aa1f1f12a846"

type tokenSourcePairer interface {
	GetTokenSourceCharStreamPair() *antlr.TokenSourceCharStreamPair
}

type structFieldKey struct {
	typ  reflect.Type
	name string
}

type structFieldIndex struct {
	index []int
	ok    bool
}

var (
	antlrTokenType       = reflect.TypeOf((*antlr.Token)(nil)).Elem()
	emptyTokenSourcePair = &antlr.TokenSourceCharStreamPair{}
	tokenFieldIndexCache = sync.Map{}
	structFieldCache     = sync.Map{}
)

// DetachLexerTokenSource clears the tokenSource reference stored in the
// TokenSourceCharStreamPair used by lexer-emitted tokens.
//
// Motivation: ANTLR CommonToken keeps a pointer to TokenSourceCharStreamPair,
// which by default references the lexer instance as tokenSource. When an AST
// (parse tree) is retained, those tokens can keep the lexer (and its ATN/DFA
// caches) alive, leading to huge memory retention in large projects.
//
// After detaching, tokens can still read text via charStream, but no longer
// retain the lexer through tokenSource.
//
// This depends on unexported fields in the pinned ANTLR Go runtime above.
// Keep `common/yak/antlr4util/detach_test.go` passing when upgrading ANTLR.
func DetachLexerTokenSource(lexer any) {
	pairer, ok := lexer.(tokenSourcePairer)
	if !ok {
		return
	}
	pair := pairer.GetTokenSourceCharStreamPair()
	if pair == nil {
		return
	}
	detachTokenSource(pair)
}

// DetachParserATNSimulatorCaches clears BaseATNSimulator's DFA and
// PredictionContextCache references from the parser's interpreter.
//
// Motivation: generated ParserRuleContext structs keep a reference to the parser.
// If we later reset/replace worker caches, that old cache can still be retained
// through ctx.parser -> parser.Interpreter -> BaseATNSimulator, preventing GC
// and causing huge memory retention on large projects.
//
// This depends on unexported fields in the pinned ANTLR Go runtime above.
// Keep `common/yak/antlr4util/detach_test.go` passing when upgrading ANTLR.
func DetachParserATNSimulatorCaches(parser antlr.Parser) {
	if parser == nil {
		return
	}
	interpreter := parser.GetInterpreter()
	if interpreter == nil || interpreter.BaseATNSimulator == nil {
		return
	}
	zeroUnexportedField(interpreter.BaseATNSimulator, "decisionToDFA")
	zeroUnexportedField(interpreter.BaseATNSimulator, "sharedContextCache")
}

// SlimParserTree trims a retained ANTLR parse subtree in place.
//
// It preserves the generated parser context types and the downward children
// shape, so existing visitors/builders can continue to use methods like
// MethodBody(), Block(), GetText(), GetStart(), and token label accessors.
//
// It removes the references that make a small lazy-build subtree pin a whole
// file parse:
//   - rule/terminal parent links are cleared;
//   - generated context parser fields are cleared;
//   - parser context exception objects are cleared;
//   - parser ATN simulator caches reachable through ctx.parser are cleared;
//   - every token reachable from the subtree is given stable text and a private
//     empty source pair, so it no longer retains the lexer/input stream.
//
// This depends on the pinned ANTLR Go runtime above. Keep detach_test.go
// passing when upgrading ANTLR.
func SlimParserTree[T antlr.Tree](node T) T {
	slimParserTree(node)
	return node
}

// SlimParserNode trims only the given ANTLR node, without descending into
// children. Use it on very hot paths such as SetRange after the range has been
// computed; recursive subtree slimming belongs at lazy-capture boundaries.
func SlimParserNode[T antlr.Tree](node T) T {
	if isTypedNil(node) {
		return node
	}
	switch n := any(node).(type) {
	case antlr.ParserRuleContext:
		slimParserRuleContext(n)
	case antlr.TerminalNode:
		SlimToken(n.GetSymbol())
	}
	return node
}

// DetachParserNode cuts only the direct upward/parser references of a parse
// tree node. It intentionally does not descend and does not copy token text.
//
// Use this for file-root cleanup after all lazy-retained subtrees have already
// been captured with SlimParserTree. It avoids turning the whole file AST into a
// retained-text copy while still breaking root parent chains.
func DetachParserNode[T antlr.Tree](node T) T {
	if isTypedNil(node) {
		return node
	}
	switch n := any(node).(type) {
	case antlr.ParserRuleContext:
		n.SetParent(nil)
		detachParserRuleContext(n)
	case antlr.TerminalNode:
		zeroUnexportedField(n, "parentCtx")
	}
	return node
}

// DetachParserTreeChildren applies DetachParserNode to a root's direct
// children. This is the cheap file-level cleanup counterpart of SlimParserTree.
func DetachParserTreeChildren(root antlr.Tree) {
	if root == nil || isTypedNil(root) {
		return
	}
	for _, child := range root.GetChildren() {
		DetachParserNode(child)
	}
}

func slimParserTree(node antlr.Tree) {
	if node == nil || isTypedNil(node) {
		return
	}

	switch n := node.(type) {
	case antlr.ParserRuleContext:
		n.SetParent(nil)
		slimParserRuleContext(n)
	case antlr.TerminalNode:
		slimTerminalNode(n)
	}

	for _, child := range node.GetChildren() {
		slimParserTree(child)
	}
}

func slimParserRuleContext(ctx antlr.ParserRuleContext) {
	if ctx == nil || isTypedNil(ctx) {
		return
	}
	ctx.SetStart(SlimToken(ctx.GetStart()))
	ctx.SetStop(SlimToken(ctx.GetStop()))

	detachParserRuleContext(ctx)
	slimTokenFields(ctx)
}

func detachParserRuleContext(ctx antlr.ParserRuleContext) {
	if ctx == nil || isTypedNil(ctx) {
		return
	}
	if parserProvider, ok := ctx.(interface{ GetParser() antlr.Parser }); ok {
		DetachParserATNSimulatorCaches(parserProvider.GetParser())
	}
	zeroUnexportedField(ctx, "parser")
	if base := baseParserRuleContext(ctx); base != nil {
		zeroUnexportedField(base, "exception")
	}
}

func slimTerminalNode(node antlr.TerminalNode) {
	if node == nil || isTypedNil(node) {
		return
	}
	SlimToken(node.GetSymbol())
	zeroUnexportedField(node, "parentCtx")
}

// SlimToken keeps token text/range metadata but drops lexer/input references.
func SlimToken[T antlr.Token](token T) T {
	if isTypedNil(token) {
		return token
	}
	token.SetText(token.GetText())
	if base := baseToken(token); base != nil {
		setUnexportedField(base, "source", emptyTokenSourcePair)
	}
	return token
}

func slimTokenFields(ctx antlr.ParserRuleContext) {
	v := reflect.ValueOf(ctx)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	for _, i := range tokenFieldIndexes(elem.Type()) {
		field := elem.Field(i)
		value, ok := fieldInterface(field)
		if !ok || value == nil {
			continue
		}
		if token, ok := value.(antlr.Token); ok {
			SlimToken(token)
		}
	}
}

func tokenFieldIndexes(ctxType reflect.Type) []int {
	if cached, ok := tokenFieldIndexCache.Load(ctxType); ok {
		return cached.([]int)
	}
	indexes := make([]int, 0)
	for i := 0; i < ctxType.NumField(); i++ {
		if ctxType.Field(i).Type.Implements(antlrTokenType) {
			indexes = append(indexes, i)
		}
	}
	actual, _ := tokenFieldIndexCache.LoadOrStore(ctxType, indexes)
	return actual.([]int)
}

func baseToken(token antlr.Token) *antlr.BaseToken {
	if token == nil || isTypedNil(token) {
		return nil
	}
	if common, ok := token.(*antlr.CommonToken); ok {
		return common.BaseToken
	}
	v := reflect.ValueOf(token)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}
	field, ok := cachedStructField(elem, "BaseToken")
	if !ok {
		return nil
	}
	value, ok := fieldInterface(field)
	if !ok {
		return nil
	}
	base, _ := value.(*antlr.BaseToken)
	return base
}

func baseParserRuleContext(ctx antlr.ParserRuleContext) *antlr.BaseParserRuleContext {
	if ctx == nil || isTypedNil(ctx) {
		return nil
	}
	if base, ok := ctx.(*antlr.BaseParserRuleContext); ok {
		return base
	}
	v := reflect.ValueOf(ctx)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}
	field, ok := cachedStructField(elem, "BaseParserRuleContext")
	if !ok {
		return nil
	}
	value, ok := fieldInterface(field)
	if !ok {
		return nil
	}
	base, _ := value.(*antlr.BaseParserRuleContext)
	return base
}

func detachTokenSource(pair *antlr.TokenSourceCharStreamPair) {
	zeroUnexportedField(pair, "tokenSource")
}

func zeroUnexportedField(structPtr any, fieldName string) {
	setUnexportedField(structPtr, fieldName, nil)
}

func setUnexportedField(structPtr any, fieldName string, value any) {
	v := reflect.ValueOf(structPtr)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field, ok := cachedStructField(elem, fieldName)
	if !ok || !field.CanAddr() {
		return
	}
	target := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	if value == nil {
		target.Set(reflect.Zero(field.Type()))
		return
	}
	source := reflect.ValueOf(value)
	if source.Type().AssignableTo(field.Type()) {
		target.Set(source)
		return
	}
	if source.Type().ConvertibleTo(field.Type()) {
		target.Set(source.Convert(field.Type()))
	}
}

func cachedStructField(elem reflect.Value, fieldName string) (reflect.Value, bool) {
	if elem.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	index, ok := cachedStructFieldIndex(elem.Type(), fieldName)
	if !ok {
		return reflect.Value{}, false
	}
	return elem.FieldByIndex(index), true
}

func cachedStructFieldIndex(typ reflect.Type, fieldName string) ([]int, bool) {
	key := structFieldKey{typ: typ, name: fieldName}
	if cached, ok := structFieldCache.Load(key); ok {
		result := cached.(structFieldIndex)
		return result.index, result.ok
	}
	field, ok := typ.FieldByName(fieldName)
	result := structFieldIndex{ok: ok}
	if ok {
		result.index = field.Index
	}
	actual, _ := structFieldCache.LoadOrStore(key, result)
	result = actual.(structFieldIndex)
	return result.index, result.ok
}

func fieldInterface(field reflect.Value) (any, bool) {
	if !field.IsValid() {
		return nil, false
	}
	if field.CanInterface() {
		return field.Interface(), true
	}
	if !field.CanAddr() {
		return nil, false
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface(), true
}

func isTypedNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
