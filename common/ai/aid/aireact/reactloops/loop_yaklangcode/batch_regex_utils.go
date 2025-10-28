package loop_yaklangcode

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// BatchRegexReplaceResult 批量正则替换结果
type BatchRegexReplaceResult struct {
	// 修改后的完整代码
	ModifiedCode string
	// 替换的行数
	ReplacementCount int
	// 修改的行详情
	ModifiedLines []ModifiedLineInfo
	// 是否有任何修改
	HasModifications bool
}

// ModifiedLineInfo 修改行信息
type ModifiedLineInfo struct {
	LineNumber   int    // 行号（从1开始）
	OriginalLine string // 原始行内容
	ModifiedLine string // 修改后行内容
}

// BatchRegexReplaceOptions 批量正则替换选项
type BatchRegexReplaceOptions struct {
	// 正则表达式模式
	Pattern string
	// 替换字符串
	Replacement string
	// 捕获组编号（0表示替换整个匹配，>0表示替换指定捕获组）
	Group int
	// 是否启用详细日志
	VerboseLog bool
}

// ValidateBatchRegexReplaceOptions 验证批量正则替换选项
func ValidateBatchRegexReplaceOptions(opts *BatchRegexReplaceOptions) error {
	if opts == nil {
		return utils.Error("options cannot be nil")
	}

	if opts.Pattern == "" {
		return utils.Error("pattern cannot be empty")
	}

	// 验证正则表达式语法
	_, err := regexp.Compile(opts.Pattern)
	if err != nil {
		return utils.Errorf("invalid regexp pattern '%s': %v", opts.Pattern, err)
	}

	if opts.Group < 0 {
		return utils.Error("group must be >= 0")
	}

	return nil
}

// BatchRegexReplace 执行批量正则替换
func BatchRegexReplace(code string, opts *BatchRegexReplaceOptions) (*BatchRegexReplaceResult, error) {
	// 验证参数
	if err := ValidateBatchRegexReplaceOptions(opts); err != nil {
		return nil, err
	}

	if code == "" {
		return &BatchRegexReplaceResult{
			ModifiedCode:     "",
			ReplacementCount: 0,
			ModifiedLines:    []ModifiedLineInfo{},
			HasModifications: false,
		}, nil
	}

	// 编译正则表达式
	re, err := regexp.Compile(opts.Pattern)
	if err != nil {
		return nil, utils.Errorf("failed to compile regexp pattern '%s': %v", opts.Pattern, err)
	}

	// 按行分割代码
	lines := strings.Split(code, "\n")
	var finalLines []string
	var modifiedLineInfos []ModifiedLineInfo
	replacementCount := 0
	deletedCount := 0

	// 逐行进行正则替换
	for i, line := range lines {
		// 首先检查是否是删除整行的情况（替换为空字符串且原行完全匹配）
		if opts.Replacement == "" && re.MatchString(line) && re.FindString(line) == line {
			// 删除整行 - 不添加到结果中
			replacementCount++
			modifiedLineInfos = append(modifiedLineInfos, ModifiedLineInfo{
				LineNumber:   i + 1,
				OriginalLine: line,
				ModifiedLine: "[DELETED]", // 标记为已删除
			})

			if opts.VerboseLog {
				log.Infof("line %d deleted: %s", i+1, utils.ShrinkTextBlock(line, 50))
			}
			deletedCount++
			// 不添加到 finalLines，实现真正的删除
			continue
		}

		// 普通替换逻辑
		newLine, modified := replaceLineWithRegex(line, re, opts)

		if modified {
			replacementCount++
			// 普通替换或部分替换
			finalLines = append(finalLines, newLine)
			modifiedLineInfos = append(modifiedLineInfos, ModifiedLineInfo{
				LineNumber:   i + 1,
				OriginalLine: line,
				ModifiedLine: newLine,
			})

			if opts.VerboseLog {
				log.Infof("line %d replaced: %s -> %s", i+1,
					utils.ShrinkTextBlock(line, 50),
					utils.ShrinkTextBlock(newLine, 50))
			}
		} else {
			// 未修改的行
			finalLines = append(finalLines, line)
		}
	}

	// 重新组合代码
	newCode := strings.Join(finalLines, "\n")

	return &BatchRegexReplaceResult{
		ModifiedCode:     newCode,
		ReplacementCount: replacementCount,
		ModifiedLines:    modifiedLineInfos,
		HasModifications: replacementCount > 0,
	}, nil
}

// replaceLineWithRegex 对单行执行正则替换
func replaceLineWithRegex(line string, re *regexp.Regexp, opts *BatchRegexReplaceOptions) (string, bool) {
	if !re.MatchString(line) {
		return line, false
	}

	var newLine string

	if opts.Group == 0 {
		// 替换整个匹配 - 使用我们自己的引用展开函数
		newLine = re.ReplaceAllStringFunc(line, func(match string) string {
			matches := re.FindStringSubmatch(match)
			if len(matches) > 0 {
				return expandReplacementReferences(opts.Replacement, matches)
			}
			return match
		})
	} else {
		// 替换指定的捕获组
		newLine = replaceSpecificGroup(line, re, opts.Group, opts.Replacement)
	}

	return newLine, newLine != line
}

// replaceSpecificGroup 替换指定的捕获组
func replaceSpecificGroup(line string, re *regexp.Regexp, group int, replacement string) string {
	// 查找所有匹配和子匹配
	matches := re.FindStringSubmatch(line)
	if len(matches) <= group {
		// 捕获组不存在，返回原行
		return line
	}

	// 获取所有子匹配的位置信息
	submatches := re.FindStringSubmatchIndex(line)
	if len(submatches) <= group*2+1 || submatches[group*2] < 0 {
		// 捕获组位置信息不存在，返回原行
		return line
	}

	// 获取捕获组的开始和结束位置
	start := submatches[group*2]
	end := submatches[group*2+1]

	// 构建替换字符串，支持 $1, $2 等引用
	finalReplacement := expandReplacementReferences(replacement, matches)

	// 执行替换：只替换捕获组部分
	return line[:start] + finalReplacement + line[end:]
}

// expandReplacementReferences 展开替换字符串中的引用（如 $1, $2）
func expandReplacementReferences(replacement string, matches []string) string {
	result := replacement

	// 使用正则表达式精确匹配 $数字 模式，避免 $2_v2 被误替换
	for i := len(matches) - 1; i >= 0; i-- {
		// 使用正则表达式匹配 $数字，确保数字后面不是数字或字母
		pattern := fmt.Sprintf(`\$%d(?:[^0-9a-zA-Z]|$)`, i)
		re := regexp.MustCompile(pattern)

		// 替换时保留非数字字母字符
		result = re.ReplaceAllStringFunc(result, func(match string) string {
			if len(match) > len(fmt.Sprintf("$%d", i)) {
				// 保留后面的字符
				suffix := match[len(fmt.Sprintf("$%d", i)):]
				return matches[i] + suffix
			}
			return matches[i]
		})
	}

	return result
}

// BatchRegexReplaceMultiPattern 支持多个模式的批量替换
func BatchRegexReplaceMultiPattern(code string, patterns []BatchRegexReplaceOptions) (*BatchRegexReplaceResult, error) {
	if len(patterns) == 0 {
		return &BatchRegexReplaceResult{
			ModifiedCode:     code,
			ReplacementCount: 0,
			ModifiedLines:    []ModifiedLineInfo{},
			HasModifications: false,
		}, nil
	}

	currentCode := code
	totalReplacements := 0
	var allModifiedLines []ModifiedLineInfo

	// 依次应用每个模式
	for i, pattern := range patterns {
		result, err := BatchRegexReplace(currentCode, &pattern)
		if err != nil {
			return nil, utils.Errorf("pattern %d failed: %v", i+1, err)
		}

		currentCode = result.ModifiedCode
		totalReplacements += result.ReplacementCount

		// 调整行号（因为前面的替换可能改变了行数）
		allModifiedLines = append(allModifiedLines, result.ModifiedLines...)
	}

	return &BatchRegexReplaceResult{
		ModifiedCode:     currentCode,
		ReplacementCount: totalReplacements,
		ModifiedLines:    allModifiedLines,
		HasModifications: totalReplacements > 0,
	}, nil
}
