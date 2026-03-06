package loop_syntaxflow_rule

import (
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

// checkSyntaxFlowAndFormatErrors 复用 static_analyzer 做语法检查与富格式输出
// 返回: errorMessages string, hasBlockingErrors bool
func checkSyntaxFlowAndFormatErrors(content string) (string, bool) {
	res := static_analyzer.SyntaxFlowRuleCheckingWithSample(content, "", "", "")
	if len(res.SyntaxErrors) > 0 {
		errMsg := res.FormattedErrors
		if errMsg != "" {
			// 与旧实现一致，便于识别错误来源
			errMsg = "SyntaxFlow 编译错误: \n" + errMsg
		}
		return errMsg, true
	}
	return "", false
}
