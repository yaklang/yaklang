package antlr4util

import (
	"reflect"
	"unsafe"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

const antlrRuntimeVersion = "github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20220911224424-aa1f1f12a846"

type tokenSourcePairer interface {
	GetTokenSourceCharStreamPair() *antlr.TokenSourceCharStreamPair
}

// SetParserBuildParseTrees toggles BaseParser.BuildParseTrees on generated
// parsers via the embedded *antlr.BaseParser field.
func SetParserBuildParseTrees(parser antlr.Parser, enabled bool) {
	if parser == nil {
		return
	}

	v := reflect.ValueOf(parser)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}

	baseParserField := elem.FieldByName("BaseParser")
	if !baseParserField.IsValid() || baseParserField.Kind() != reflect.Ptr || baseParserField.IsNil() {
		return
	}

	baseParserElem := baseParserField.Elem()
	if baseParserElem.Kind() != reflect.Struct {
		return
	}

	buildParseTreesField := baseParserElem.FieldByName("BuildParseTrees")
	if !buildParseTreesField.IsValid() || !buildParseTreesField.CanAddr() || buildParseTreesField.Kind() != reflect.Bool {
		return
	}

	reflect.NewAt(buildParseTreesField.Type(), unsafe.Pointer(buildParseTreesField.UnsafeAddr())).Elem().SetBool(enabled)
}

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

func detachTokenSource(pair *antlr.TokenSourceCharStreamPair) {
	zeroUnexportedField(pair, "tokenSource")
}

func zeroUnexportedField(structPtr any, fieldName string) {
	v := reflect.ValueOf(structPtr)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field := elem.FieldByName(fieldName)
	if !field.IsValid() || !field.CanAddr() {
		return
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.Zero(field.Type()))
}
