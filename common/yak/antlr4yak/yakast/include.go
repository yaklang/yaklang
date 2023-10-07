package yakast

import (
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	"github.com/yaklang/yaklang/common/utils"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
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
	if _, err := utils.GetFirstExistedFileE(fpath); err != nil {
		y.panicCompilerError(includePathNotFoundError, fpath)
	}

	code, err := ioutil.ReadFile(fpath)
	if err != nil {
		y.panicCompilerError(readFileError, fpath, err)
	}
	codeStr := string(code)
	fileHash := utils.CalcSha1(codeStr)
	if _, ok := y.importCycleHash[fileHash]; ok {
		y.panicCompilerError(includeCycleError, fpath)
		return nil
	}
	y.importCycleHash[fileHash] = struct{}{}

	y.writeString(`"` + fpath + `"`)

	// parse
	inputStream := antlr.NewInputStream(string(code))
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)

	// compile, 忽略formatter
	recoverFormatBufferFunc := y.switchFormatBuffer()
	recoverSource := y.switchSource(&fpath, &codeStr)
	defer func() {
		recoverFormatBufferFunc()
		recoverSource()
	}()

	y.VisitProgramWithoutSymbolTable(p.Program().(*yak.ProgramContext))

	return nil
}
