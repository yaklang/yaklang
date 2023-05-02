package yak

import (
	"fmt"
	"regexp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
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
	code := utils.InterfaceToBytes(r)
	engine := yaklang.New()

	newEngine, ok := engine.(*antlr4yak.Engine)
	if !ok {
		return nil
	}
	opcodes, err := newEngine.Compile(string(code))
	if err != nil {
		switch ret := err.(type) {
		case yakast.YakMergeError:
			var results []*StaticAnalyzeResult
			for _, e := range ret {
				results = append(results, &StaticAnalyzeResult{
					Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
					Severity:        "error",
					StartLineNumber: e.StartPos.LineNumber,
					StartColumn:     e.StartPos.ColumnNumber,
					EndLineNumber:   e.EndPos.LineNumber,
					EndColumn:       e.EndPos.ColumnNumber,
					RawMessage:      e.Error(),
				})
			}
			return results
		default:
			log.Error("静态分析失败：Yaklang 返回错误不标准")
			return nil
		}
	}
	_ = opcodes
	return nil
}
