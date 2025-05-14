package metadata

import (
	"embed"
	"fmt"
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

type YakToolMetadata struct {
	Name        string
	Description string
	Keywords    []string
}

func ParseYakScriptMetadata(name string, code string) (*YakToolMetadata, error) {
	prog, err := static_analyzer.SSAParse(code, "yak")
	if err != nil {
		return nil, fmt.Errorf("static_analyzer.SSAParse(string(content), \"yak\") error: %v", err)
	}

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
	return &YakToolMetadata{
		Name:        name,
		Description: strings.Join(desc, "; "),
		Keywords:    keywords,
	}, nil
}

// 生成元数据（关键词和描述）从代码
func GenerateMetadataFromCodeContent(name string, code string) (*YakToolMetadata, error) {
	// 首先尝试从现有代码中解析元数据
	existingMetadata, err := ParseYakScriptMetadata(name, code)
	if err == nil && existingMetadata != nil {
		// 如果已经有完整的元数据信息，使用AI进行增强
		log.Infof("Found existing metadata for %s, enhancing with AI", name)

		// 如果既有描述又有关键词，使用AI增强
		if existingMetadata.Description != "" && len(existingMetadata.Keywords) > 0 {
			// 使用AI生成增强的元数据
			aiResult, aiErr := GenerateMetadataFromCode(code)
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

				return &YakToolMetadata{
					Name:        name,
					Description: description,
					Keywords:    mergedKeywords,
				}, nil
			}
		}
	}

	// 如果没有现有元数据或只有部分元数据，使用AI生成完整元数据
	result, err := GenerateMetadataFromCode(code)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metadata from code: %v", err)
	}

	return &YakToolMetadata{
		Name:        name,
		Description: result.Description,
		Keywords:    result.Keywords,
	}, nil
}
