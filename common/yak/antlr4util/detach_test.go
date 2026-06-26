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

func TestSlimParserTree_ClearsParentParserAndTokenInput(t *testing.T) {
	assertUnexportedFieldExists(t, &antlr.TokenSourceCharStreamPair{}, "charStream")
	assertUnexportedFieldExists(t, antlr.NewTerminalNodeImpl(nil), "parentCtx")
	assertUnexportedFieldExists(t, antlr.NewBaseParserRuleContext(nil, -1), "exception")

	lexer := javaparser.NewJavaLexer(antlr.NewInputStream("class A { void run(){ int x = 1 + 2; } }"))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	cu := parser.CompilationUnit().(*javaparser.CompilationUnitContext)
	method := cu.TypeDeclaration(0).(*javaparser.TypeDeclarationContext).ClassDeclaration().(*javaparser.ClassDeclarationContext).ClassBody().(*javaparser.ClassBodyContext).ClassBodyDeclaration(0).(*javaparser.ClassBodyDeclarationContext).MemberDeclaration().(*javaparser.MemberDeclarationContext).MethodDeclaration().(*javaparser.MethodDeclarationContext)
	body := method.MethodBody().(*javaparser.MethodBodyContext)

	start := body.GetStart()
	if start == nil || start.GetInputStream() == nil || start.GetTokenSource() == nil {
		t.Fatalf("expected method body token to retain input/source before slimming")
	}
	beforeText := body.GetText()
	if body.GetParent() == nil {
		t.Fatalf("expected method body to have parent before slimming")
	}
	if body.GetParser() == nil {
		t.Fatalf("expected generated context parser before slimming")
	}

	SlimParserTree(body)

	if body.GetParent() != nil {
		t.Fatalf("expected root parent to be nil after slimming")
	}
	if body.GetParser() != nil {
		t.Fatalf("expected generated context parser to be nil after slimming")
	}
	if body.GetText() != beforeText {
		t.Fatalf("expected text to remain stable after slimming, before=%q after=%q", beforeText, body.GetText())
	}
	if got := body.GetStart(); got == nil || got.GetText() == "" || got.GetInputStream() != nil || got.GetTokenSource() != nil {
		t.Fatalf("expected start token text without input/source after slimming, token=%#v input=%v source=%v", got, got.GetInputStream(), got.GetTokenSource())
	}
	block := body.Block().(*javaparser.BlockContext)
	if block.GetParser() != nil {
		t.Fatalf("expected child generated context parser to be nil after slimming")
	}
	blockStart := block.GetChild(0)
	if blockStart == nil || blockStart.GetParent() != nil {
		t.Fatalf("expected terminal child parent to be nil after slimming, child=%#v parent=%v", blockStart, blockStart.GetParent())
	}
}

func TestDetachParserTreeChildren_CutsRootChildrenWithoutRecursiveTokenCopy(t *testing.T) {
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream("class A { void run(){ int x = 1 + 2; } }"))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	cu := parser.CompilationUnit().(*javaparser.CompilationUnitContext)
	typeDecl := cu.TypeDeclaration(0).(*javaparser.TypeDeclarationContext)
	method := typeDecl.ClassDeclaration().(*javaparser.ClassDeclarationContext).ClassBody().(*javaparser.ClassBodyContext).ClassBodyDeclaration(0).(*javaparser.ClassBodyDeclarationContext).MemberDeclaration().(*javaparser.MemberDeclarationContext).MethodDeclaration().(*javaparser.MethodDeclarationContext)
	body := method.MethodBody().(*javaparser.MethodBodyContext)

	start := body.GetStart()
	if start == nil || start.GetInputStream() == nil || start.GetTokenSource() == nil {
		t.Fatalf("expected descendant token to retain input/source before lightweight detach")
	}
	if typeDecl.GetParent() == nil {
		t.Fatalf("expected direct child to have parent before lightweight detach")
	}
	if typeDecl.GetParser() == nil || body.GetParser() == nil {
		t.Fatalf("expected generated context parsers before lightweight detach")
	}

	DetachParserTreeChildren(cu)

	if typeDecl.GetParent() != nil {
		t.Fatalf("expected direct child parent to be nil after lightweight detach")
	}
	if typeDecl.GetParser() != nil {
		t.Fatalf("expected direct child parser to be nil after lightweight detach")
	}
	if body.GetParser() == nil {
		t.Fatalf("expected descendant parser to remain untouched by lightweight detach")
	}
	if start.GetInputStream() == nil || start.GetTokenSource() == nil {
		t.Fatalf("expected descendant token input/source to remain untouched by lightweight detach")
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
