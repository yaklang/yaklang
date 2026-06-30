package antlr4util

import (
	"reflect"
	"unsafe"

	"github.com/yaklang/antlr/v4"
)

const antlrRuntimeVersion = "github.com/yaklang/antlr/v4 v4.13.1"

type tokenSourcePairer interface {
	GetTokenSourceCharStreamPair() *antlr.TokenSourceCharStreamPair
}

var (
	antlrTokenType       = reflect.TypeOf((*antlr.Token)(nil)).Elem()
	emptyTokenSourcePair = &antlr.TokenSourceCharStreamPair{}
)

// DetachLexerTokenSource clears the tokenSource reference stored in the
// TokenSourceCharStreamPair used by lexer-emitted tokens.
func DetachLexerTokenSource(lexer any) {
	pairer, ok := lexer.(tokenSourcePairer)
	if !ok {
		return
	}
	pair := pairer.GetTokenSourceCharStreamPair()
	if pair == nil {
		return
	}
	zeroUnexportedField(pair, "tokenSource")
}

// DetachParserATNSimulatorCaches clears BaseATNSimulator's DFA and
// PredictionContextCache references from the parser's interpreter.
func DetachParserATNSimulatorCaches(parser antlr.Parser) {
	if parser == nil {
		return
	}
	interpreter := parser.GetInterpreter()
	if interpreter == nil {
		return
	}
	zeroUnexportedField(&interpreter.BaseATNSimulator, "decisionToDFA")
	zeroUnexportedField(&interpreter.BaseATNSimulator, "sharedContextCache")
}

// SlimParserTree trims a retained ANTLR parse subtree in place.
func SlimParserTree[T antlr.Tree](node T) T {
	slimParserTree(node)
	return node
}

// SlimParserNode trims only the given ANTLR node, without descending into children.
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

// DetachParserNode cuts only the direct upward/parser references of a parse tree node.
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

// DetachParserTreeChildren applies DetachParserNode to a root's direct children.
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
	for i := 0; i < elem.NumField(); i++ {
		fieldType := elem.Type().Field(i).Type
		if !fieldType.Implements(antlrTokenType) {
			continue
		}
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

func baseToken(token antlr.Token) *antlr.BaseToken {
	if token == nil || isTypedNil(token) {
		return nil
	}
	if common, ok := token.(*antlr.CommonToken); ok {
		return &common.BaseToken
	}
	v := reflect.ValueOf(token)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}
	field, ok := structField(elem, "BaseToken")
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
	field, ok := structField(elem, "BaseParserRuleContext")
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
	field, ok := structField(elem, fieldName)
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

func structField(elem reflect.Value, fieldName string) (reflect.Value, bool) {
	if elem.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	field, ok := elem.Type().FieldByName(fieldName)
	if !ok {
		return reflect.Value{}, false
	}
	return elem.FieldByIndex(field.Index), true
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
