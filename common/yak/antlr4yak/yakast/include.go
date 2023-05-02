package yakast

import (
	"io/ioutil"
	"strconv"
	"strings"
	yak "yaklang/common/yak/antlr4yak/parser"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/go-rod/rod/lib/utils"
)

func (y *YakCompiler) VisitIncludeStmt(raw yak.IIncludeStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.IncludeStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("include ")

	// include 语句的参数是文件路径，直接读取并判断是否存在
	fpath := i.StringLiteral().GetText()
	fpath = strings.ReplaceAll(fpath, "\\", "\\\\")
	fpath, err := strconv.Unquote(fpath)
	if err != nil {
		y.panicCompilerError(includeUnquoteError, fpath, err)
	}
	if !utils.FileExists(fpath) {
		y.panicCompilerError(includePathNotFoundError, fpath)
	}

	code, err := ioutil.ReadFile(fpath)
	if err != nil {
		y.panicCompilerError(readFileError, fpath, err)
	}

	y.writeString(`"` + fpath + `"`)
	// parse
	inputStream := antlr.NewInputStream(string(code))
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)

	// compile, 忽略formatter
	recoverFormatBufferFunc := y.switchFormatBuffer()
	y.VisitProgramWithoutSymbolTable(p.Program().(*yak.ProgramContext))
	recoverFormatBufferFunc()
	return nil
}
