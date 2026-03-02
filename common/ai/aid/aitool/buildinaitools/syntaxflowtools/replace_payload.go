package syntaxflowtools

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// ReplacePayloadWithFileContent 当有生成文件路径时，以磁盘文件内容为准展示，忽略 AI payload。
// label 为简短说明，如 "规则"、"脚本"、"报告"
func ReplacePayloadWithFileContent(payload string, filepath string, label string) string {
	if filepath == "" || label == "" {
		return payload
	}
	fileContent, err := os.ReadFile(filepath)
	if err != nil {
		log.Warnf("directly_answer: failed to read file %s: %v, using original payload", filepath, err)
		return payload
	}
	content := strings.TrimSpace(string(fileContent))
	if content == "" {
		return payload
	}
	return label + "已生成，路径：`" + filepath + "`\n\n" + content
}

// ReplacePayloadRuleWithFileContent 当有 sf 规则文件路径时，由系统读取磁盘文件并展示，确保与生成文件一致。
func ReplacePayloadRuleWithFileContent(payload string, filepath string) string {
	return ReplacePayloadWithFileContent(payload, filepath, "规则")
}
