package yak

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	pta "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	_ "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
	From            string `json: "from"`
}

func AnalyzeStaticYaklang(i interface{}) []*StaticAnalyzeResult {
	code := utils.UnsafeBytesToString(utils.InterfaceToBytes(i))

	return AnalyzeStaticYaklangWithType(code, "yak")
}

func AnalyzeStaticYaklangWithType(code, codeTyp string) []*StaticAnalyzeResult {
	var results []*StaticAnalyzeResult

	// compiler
	newEngine := yaklang.New()
	newEngine.SetStrictMode(false)
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
					From:            "compiler",
				})
			}
		default:
			log.Error("静态分析失败：Yaklang 返回错误不标准")
		}
	}

	prog := ssaapi.Parse(code, pta.GetPluginSSAOpt(codeTyp)...)
	if prog == nil {
		return results
	}
	pta.CheckPluginType(codeTyp, prog)

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
			From:            "SSA",
		})
	}
	// debug printf json
	// prog.ShowWithSource()
	// for _, result := range results {
	// 	fmt.Printf("%+v\n", result)
	// }

	return results
}
