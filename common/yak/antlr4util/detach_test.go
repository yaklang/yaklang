package antlr4util

import (
	"reflect"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

func TestDetachLexerTokenSource(t *testing.T) {
	assertUnexportedFieldExists(t, &antlr.TokenSourceCharStreamPair{}, "tokenSource")

	lexer := javaparser.NewJavaLexer(antlr.NewInputStream("class A{}"))
	tok := lexer.NextToken()
	if tok == nil || tok.GetTokenType() == antlr.TokenEOF {
		t.Fatalf("expected a non-EOF token, got %v", tok)
	}
	if tok.GetTokenSource() == nil {
		t.Fatalf("expected token source to be non-nil before detach")
	}
	beforeText := tok.GetText()

	DetachLexerTokenSource(lexer)

	if tok.GetTokenSource() != nil {
		t.Fatalf("expected token source to be nil after detach")
	}
	afterText := tok.GetText()
	if beforeText != afterText {
		t.Fatalf("expected token text to remain available after detach, before=%q after=%q", beforeText, afterText)
	}
}

func TestDetachParserATNSimulatorCaches(t *testing.T) {
	assertUnexportedFieldExists(t, antlr.NewParserATNSimulator(nil, nil, nil, nil).BaseATNSimulator, "decisionToDFA")
	assertUnexportedFieldExists(t, antlr.NewParserATNSimulator(nil, nil, nil, nil).BaseATNSimulator, "sharedContextCache")

	lexer := javaparser.NewJavaLexer(antlr.NewInputStream("class A{}"))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)

	interp := parser.GetInterpreter()
	if interp == nil || interp.BaseATNSimulator == nil {
		t.Fatalf("expected parser interpreter to be non-nil")
	}
	if interp.DecisionToDFA() == nil || len(interp.DecisionToDFA()) == 0 {
		t.Fatalf("expected decisionToDFA to be initialized before detach")
	}
	if interp.SharedContextCache() == nil {
		t.Fatalf("expected sharedContextCache to be initialized before detach")
	}

	DetachParserATNSimulatorCaches(parser)

	if interp.DecisionToDFA() != nil {
		t.Fatalf("expected decisionToDFA to be nil after detach")
	}
	if interp.SharedContextCache() != nil {
		t.Fatalf("expected sharedContextCache to be nil after detach")
	}
}

func assertUnexportedFieldExists(t *testing.T, target any, fieldName string) {
	t.Helper()

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		t.Fatalf("target must be a non-nil pointer, got %T", target)
	}
	if value.Elem().FieldByName(fieldName).IsValid() {
		return
	}
	t.Fatalf("antlr runtime field %q not found on %T; update detach.go for %s", fieldName, target, antlrRuntimeVersion)
}
