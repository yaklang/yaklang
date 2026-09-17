package yakast

import (
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestCoreCompilerUnknownPanicFailsClosed(t *testing.T) {
	y := NewYakCompilerWithSymbolTable(yakvm.NewSymbolTable())
	lexer := yak.NewYaklangLexer(antlr.NewInputStream("value = 1"))
	parser := yak.NewYaklangParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	// A broken formatter is an internal failure, not a registered semantic error.
	y.formatted = nil
	y.VisitProgram(parser.Program())
	if len(y.GetErrors()) == 0 || !strings.Contains(y.GetErrors().Error(), "internal compiler error") {
		t.Fatal("internal panic was swallowed")
	}
	if len(y.GetOpcodes()) != 0 {
		t.Fatal("partial opcodes escaped")
	}
	if !y.Compiler("value = 2") {
		t.Fatal(y.GetErrors())
	}
	if y.Compiler("1=2") {
		t.Fatal("invalid lvalue accepted")
	}
	if len(y.GetOpcodes()) != 0 {
		t.Fatal("failed compile published code")
	}
	if !y.Compiler("value = 3") {
		t.Fatal(y.GetErrors())
	}
}
