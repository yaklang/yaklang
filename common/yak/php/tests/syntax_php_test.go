package tests

import (
	"embed"
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed syntax/***
var syntaxFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		lex := phpparser.NewPHPLexer(antlr.NewInputStream(src))
		tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
		parser := phpparser.NewPHPParser(tokenStream)
		parser.SetErrorHandler(antlr.NewBailErrorStrategy())
		if parser.HtmlDocument() == nil {
			t.Errorf("file: %v 's syntax parsing failed, no html document entry", filename)
		}
	})
}

func TestAllSyntaxForPHP_G4(t *testing.T) {
	entry, err := syntaxFs.ReadDir("syntax")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		path := filepath.Join("syntax", f.Name())
		if !strings.HasSuffix(path, ".php") {
			continue
		}
		raw, err := syntaxFs.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", path)
		}
		validateSource(t, path, string(raw))
	}
}

func TestSyntax_(t *testing.T) {
	validateSource(t, "class member access", `<?php $c->fn = 1; ?>`)
	validateSource(t, `string as class identifier`, `
<?php 
class foo { static $bar = 'baz'; }
var_dump('foo'::$bar);`)
}
