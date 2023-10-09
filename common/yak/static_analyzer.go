package yak

import (
	"fmt"
	"os"
	"regexp"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

type StaticAnalyzeResult struct {
	Message         string `json:"message"`
	Severity        string `json:"severity"` // Error / Warning
	StartLineNumber int    `json:"startLineNumber"`
	StartColumn     int    `json:"startColumn"`
	EndLineNumber   int    `json:"endLineNumber"`
	EndColumn       int    `json:"endColumn"`
	RawMessage      string `json:"rawMessage"`
}

var (
	extractLineFromSyntax = regexp.MustCompile(`(?i)^line (\d+): ((Syntax Error)|(runtime error:)|(Tokenize Error))`)
)

func AnalyzeStaticYaklang(r interface{}) []*StaticAnalyzeResult {
	_ = extractLineFromSyntax
	return AnalyzeStaticYaklangEx(r, os.Getenv("STATIC_CHECK") == "strict")
}

func AnalyzeStaticYaklangEx(r interface{}, strictMode bool) []*StaticAnalyzeResult {
	code := string(utils.InterfaceToBytes(r))
	var results []*StaticAnalyzeResult

	// compiler
	newEngine := yaklang.New()
	newEngine.SetStrictMode(strictMode)
	_, err := newEngine.Compile(code)
	if err != nil {
		switch ret := err.(type) {
		case yakast.YakMergeError:
			for _, e := range ret {
				results = append(results, &StaticAnalyzeResult{
					Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
					Severity:        "error",
					StartLineNumber: e.StartPos.LineNumber,
					StartColumn:     e.StartPos.ColumnNumber + 1,
					EndLineNumber:   e.EndPos.LineNumber,
					EndColumn:       e.EndPos.ColumnNumber + 2,
					RawMessage:      e.Error(),
				})
			}
		default:
			log.Error("静态分析失败：Yaklang 返回错误不标准")
		}
	}

	// yak function table
	functionTable := yaklang.New().GetFntable()
	// yak-main
	functionTable["YAK_DIR"] = ""
	functionTable["YAK_FILENAME"] = ""
	functionTable["YAK_MAIN"] = false
	functionTable["id"] = ""
	// param
	getParam := func(key string) interface{} {
		return nil
	}
	functionTable["getParam"] = getParam
	functionTable["getParams"] = getParam
	functionTable["param"] = getParam

	// mitm
	functionTable["MITM_PLUGIN"] = ""
	functionTable["MITM_PARAMS"] = make(map[string]string)
	// ssa
	prog := yak2ssa.ParseSSA(
		code,
		yak2ssa.WithSymbolTable(functionTable),
	)
	errs := prog.GetErrors()
	for _, err := range errs {
		var severity string
		switch err.Kind {
		case ssa.Warn:
			severity = "warning"
		case ssa.Error:
			severity = "error"
		}
		results = append(results, &StaticAnalyzeResult{
			Message:         err.Message,
			Severity:        severity,
			StartLineNumber: err.Pos.StartLine,
			StartColumn:     err.Pos.StartColumn + 1,
			EndLineNumber:   err.Pos.EndLine,
			EndColumn:       err.Pos.EndColumn + 2,
			RawMessage:      err.String(),
		})
	}
	// debug printf json
	// prog.Show()
	// for _, result := range results {
	// 	fmt.Printf("%+v\n", result)
	// }

	return results
}
