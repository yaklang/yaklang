package tests

import (
	_ "embed"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed bad_doc.php
var badDocPHP string

func TestBadDoc(t *testing.T) {
	validateSource(t, "bad_doc.php", badDocPHP)
}

//go:embed syntax/bad_qrcode.php
var qrcode string

// todo: 待修复
//func TestBadQrcode(t *testing.T) {
//	ssatest.Check(t, qrcode, func(prog *ssaapi.Program) error {
//		prog.Show()
//		return nil
//	}, ssaapi.WithLanguage(ssaapi.PHP))
//}

//go:embed bad/badFile_panic1.php
var bad_panic string

func TestBadPanic(t *testing.T) {
	ssatest.Check(t, bad_panic, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}

//go:embed bad/badFile_panic2.php
var bad_panic2 string

func TestBadPanic2(t *testing.T) {
	ssatest.Check(t, bad_panic2, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}

//go:embed bad/bad_panic3.php
var bad_panic3 string

func TestBadPanic3(t *testing.T) {
	ssatest.NonStrictMockSSA(t, bad_panic3)
	//lexer := phpparser.NewPHPLexer(antlr.NewInputStream(bad_panic3))
	//lexer.RemoveErrorListeners()
	//tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	//source := tokenStream.GetTokenSource()
	//for {
	//	token := source.NextToken()
	//	fmt.Printf("%d:%s\n", token.GetTokenType(), token.GetText())
	//	if token == nil || token.GetTokenType() == antlr.TokenEOF {
	//		break
	//	}
	//}
}

func TestPHPHtmlComment(t *testing.T) {
	testCode := `/*
<?php die(); ?>
*/`
	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(testCode))
	lexer.RemoveErrorListeners()
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	source := tokenStream.GetTokenSource()
	hasPHPstartToken := false
	for {
		token := source.NextToken()
		if token == nil || token.GetTokenType() == antlr.TokenEOF {
			break
		}
		if token.GetTokenType() == phpparser.PHPLexerPHPStart {
			hasPHPstartToken = true
		}
	}
	require.True(t, hasPHPstartToken, "PHPStart token not found")

}

func TestBadCode(t *testing.T) {
	code := `/*
/*

*/

set a =1;
*/`
	ssatest.NonStrictMockSSA(t, code)
}
