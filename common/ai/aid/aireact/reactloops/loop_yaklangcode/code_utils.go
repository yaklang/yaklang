package loop_yaklangcode

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// lineNumberRegex 匹配行号格式: (数字)\s+(|)\s{1}(代码)
var lineNumberRegex = regexp.MustCompile(`^(\d+)\s+\|\s`)

func prettifyAITagCode(i string) (start, end int, result string, fixed bool) {
	lines := utils.ParseStringToRawLines(i)
	if len(lines) == 0 {
		return 0, 0, i, false
	}

	// 跳过前面的空行，找到第一行有内容的行
	startIdx := 0
	for startIdx < len(lines) && strings.TrimSpace(lines[startIdx]) == "" {
		startIdx++
	}

	// 跳过后面的空行，找到最后一行有内容的行
	endIdx := len(lines) - 1
	for endIdx >= startIdx && strings.TrimSpace(lines[endIdx]) == "" {
		endIdx--
	}

	// 如果全是空行
	if startIdx > endIdx {
		return 0, 0, i, false
	}

	// 尝试识别第一行的行号
	firstLine := lines[startIdx]
	match := lineNumberRegex.FindStringSubmatch(firstLine)
	if match == nil {
		// 第一行没有行号，快速失败
		return 0, 0, i, false
	}

	// 解析第一行的行号
	firstLineNum, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, i, false
	}

	// 验证所有行并收集结果
	var processedLines []string
	expectedLineNum := firstLineNum

	for j := startIdx; j <= endIdx; j++ {
		line := lines[j]

		// 空行允许存在
		if strings.TrimSpace(line) == "" {
			processedLines = append(processedLines, "")
			continue
		}

		// 尝试匹配行号
		match := lineNumberRegex.FindStringSubmatch(line)
		if match == nil {
			// 这一行没有行号格式，快速失败
			return 0, 0, i, false
		}

		// 解析行号
		lineNum, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, 0, i, false
		}

		// 检查行号是否连续（只检验有行号的行）
		if lineNum != expectedLineNum {
			// 行号不连续，快速失败
			return 0, 0, i, false
		}

		expectedLineNum++

		// 提取代码部分（移除行号前缀）
		// 找到 | 后面的空格位置，然后取后面的所有内容
		indexOfPipe := strings.Index(line, "|")
		if indexOfPipe == -1 {
			return 0, 0, i, false
		}
		// 跳过 | 和后面的空格
		codeStart := indexOfPipe + 2 // 1 for |, 1 for the required space
		var code string
		if codeStart < len(line) {
			code = line[codeStart:]
		}
		processedLines = append(processedLines, code)
	}

	// 构建修复后的结果
	var buf bytes.Buffer
	for _, line := range processedLines {
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	finalResult := strings.TrimSuffix(buf.String(), "\n")

	return firstLineNum, expectedLineNum - 1, finalResult, true
}
