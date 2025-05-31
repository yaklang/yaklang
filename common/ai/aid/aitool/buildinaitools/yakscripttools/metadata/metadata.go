package metadata

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

func GetYakScript(fs embed.FS, name string) (string, error) {
	content, err := fs.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type YakScriptMetadata struct {
	Name        string
	Description string
	Keywords    []string
}

func ParseYakScriptMetadataProg(name string, prog *ssaapi.Program) (*YakScriptMetadata, error) {
	var desc []string
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		data, err := strconv.Unquote(value.String())
		if err != nil {
			data = value.String()
		}
		desc = append(desc, data)
	})

	var keywords []string
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		data, err := strconv.Unquote(value.String())
		if err != nil {
			data = value.String()
		}
		keywords = append(keywords, strings.Split(data, ",")...)
	})
	return &YakScriptMetadata{
		Name:        name,
		Description: strings.Join(desc, "; "),
		Keywords:    keywords,
	}, nil
}

func ParseYakScriptMetadata(name string, code string) (*YakScriptMetadata, error) {
	prog, err := static_analyzer.SSAParse(code, "yak")
	if err != nil {
		return nil, fmt.Errorf("static_analyzer.SSAParse(string(content), \"yak\") error: %v", err)
	}
	return ParseYakScriptMetadataProg(name, prog)
}

// 生成元数据（关键词和描述）从代码
func GenerateMetadataFromCodeContent(name string, code string) (*YakScriptMetadata, error) {
	// 首先尝试从现有代码中解析元数据
	existingMetadata, err := ParseYakScriptMetadata(name, code)
	if err == nil && existingMetadata != nil {
		// 如果已经有完整的元数据信息，使用AI进行增强
		log.Infof("Found existing metadata for %s, enhancing with AI", name)

		// 如果既有描述又有关键词，使用AI增强
		if existingMetadata.Description != "" && len(existingMetadata.Keywords) > 0 {
			// 使用AI生成增强的元数据
			aiResult, aiErr := GenerateYakScriptAIToolMetadata(code)
			if aiErr == nil && aiResult != nil {
				// 将AI生成的关键词与现有关键词合并，去重
				keywordMap := make(map[string]bool)
				for _, kw := range existingMetadata.Keywords {
					keywordMap[strings.ToLower(strings.TrimSpace(kw))] = true
				}

				for _, kw := range aiResult.Keywords {
					keywordMap[strings.ToLower(strings.TrimSpace(kw))] = true
				}

				// 重建关键词列表
				var mergedKeywords []string
				for kw := range keywordMap {
					if kw != "" {
						mergedKeywords = append(mergedKeywords, kw)
					}
				}

				// 优先使用现有描述，并在AI描述有实质性不同时添加它
				description := existingMetadata.Description
				if !strings.Contains(strings.ToLower(description), strings.ToLower(aiResult.Description)) &&
					!strings.Contains(strings.ToLower(aiResult.Description), strings.ToLower(description)) {
					description = aiResult.Description
				}

				return &YakScriptMetadata{
					Name:        name,
					Description: description,
					Keywords:    mergedKeywords,
				}, nil
			}
		}
	}

	// 如果没有现有元数据或只有部分元数据，使用AI生成完整元数据
	result, err := GenerateYakScriptAIToolMetadata(code)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metadata from code: %v", err)
	}

	return &YakScriptMetadata{
		Name:        name,
		Description: result.Description,
		Keywords:    result.Keywords,
	}, nil
}

// GenerateScriptWithMetadata 生成带有描述和关键词的脚本内容
func GenerateScriptWithMetadata(content string, description string, keywords []string) string {
	prog, err := static_analyzer.SSAParse(content, "yak")
	if err != nil {
		log.Errorf("Failed to parse metadata: %v", err)
		return content
	}

	contentLines := strings.Split(content, "\n")
	descRanges := make([]struct{ typ, start, end int }, 0)
	keywordsRanges := make([]struct{ typ, start, end int }, 0)

	// Find __DESC__ variables and their ranges
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		descRange := value.GetRange()
		if descRange != nil {
			start := descRange.GetStart().GetLine()
			end := descRange.GetEnd().GetLine()
			descRanges = append(descRanges, struct{ typ, start, end int }{typ: 0, start: start, end: end})
		}
	})

	// Find __KEYWORDS__ variables and their ranges
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		keywordsRange := value.GetRange()
		if keywordsRange != nil {
			start := keywordsRange.GetStart().GetLine()
			end := keywordsRange.GetEnd().GetLine()
			keywordsRanges = append(keywordsRanges, struct{ typ, start, end int }{typ: 1, start: start, end: end})
		}
	})

	allRange := append(descRanges, keywordsRanges...)
	// Sort ranges in reverse order to avoid index shifts when modifying the content
	sort.Slice(allRange, func(i, j int) bool {
		return allRange[i].start > allRange[j].start
	})

	// Replace or remove all __DESC__ variables
	for _, r := range allRange {
		// 确保索引在有效范围内
		if r.start <= 0 || r.end >= len(contentLines) {
			log.Warnf("Invalid range: start=%d, end=%d, content length=%d", r.start, r.end, len(contentLines))
			continue
		}

		switch r.typ {
		case 0:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		case 1:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		}
	}

	// Generate new declarations
	newDesc := ""
	if strings.Contains(description, "\n") {
		// Use heredoc format for multiline descriptions
		newDesc = fmt.Sprintf("__DESC__ = <<<EOF\n%s\nEOF\n\n", description)
	} else {
		newDesc = fmt.Sprintf("__DESC__ = %q\n\n", description)
	}
	newKeywords := fmt.Sprintf("__KEYWORDS__ = %q\n\n", strings.Join(keywords, ","))

	newContent := strings.TrimSpace(strings.Join(contentLines, "\n"))
	// Add new declarations at the beginning of the file
	newContent = newDesc + newKeywords + newContent
	return newContent
}
