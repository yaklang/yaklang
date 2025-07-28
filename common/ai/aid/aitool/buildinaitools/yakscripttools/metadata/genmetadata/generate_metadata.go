package genmetadata

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed prompt/generate_keyword.txt
var aitool_generate_key_word_prompt string

// 定义结构体来存储结果
type GenerateResult struct {
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

// 生成元数据（关键词和描述）从代码
func GenerateMetadataFromCodeContent(name string, code string) (*metadata.YakScriptMetadata, error) {
	// 首先尝试从现有代码中解析元数据
	existingMetadata, err := metadata.ParseYakScriptMetadata(name, code)
	if err == nil && existingMetadata != nil {
		// 如果已经有完整的元数据信息，使用AI进行增强
		log.Infof("Found existing metadata for %s, enhancing with AI", name)

		// 如果既有描述又有关键词，使用AI增强
		if existingMetadata.Description != "" && len(existingMetadata.Keywords) > 0 {
			// 使用AI生成增强的元数据
			aiResult, aiErr := generateMetadata(code, aitool_generate_key_word_prompt, false)
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

				return &metadata.YakScriptMetadata{
					Name:        name,
					Description: description,
					Keywords:    mergedKeywords,
				}, nil
			}
		}
	}

	// 如果没有现有元数据或只有部分元数据，使用AI生成完整元数据
	result, err := generateMetadata(code, aitool_generate_key_word_prompt, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metadata from code: %v", err)
	}

	return &metadata.YakScriptMetadata{
		Name:        name,
		Description: result.Description,
		Keywords:    result.Keywords,
	}, nil
}

func newForge() (*aiforge.LiteForge, error) {
	lf, err := aiforge.NewLiteForge("generate_metadata", aiforge.WithLiteForge_Prompt(aitool_generate_key_word_prompt),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithStringParam("language", aitool.WithParam_Required(true), aitool.WithParam_Description("语言，固定为chinese")),
			aitool.WithStringParam("description", aitool.WithParam_Required(true), aitool.WithParam_Description("ai工具功能描述")),
			aitool.WithStringArrayParam("keywords", aitool.WithParam_Required(true), aitool.WithParam_Description("关键词数组")),
		))
	if err != nil {
		return nil, err
	}
	return lf, nil
}

func generateMetadata(code string, promptFormat string, debug bool) (*GenerateResult, error) {
	promptTemplate := template.Must(template.New("generate_keywords").Parse(promptFormat))

	// Create a buffer to store the executed template
	var promptBuffer bytes.Buffer

	// Execute the template with the code and existing description
	templateData := map[string]interface{}{
		"Code": code,
	}

	err := promptTemplate.Execute(&promptBuffer, templateData)
	if err != nil {
		log.Errorf("failed to execute prompt template: %v", err)
		return nil, fmt.Errorf("failed to execute prompt template: %v", err)
	}

	lf, err := newForge()
	if err != nil {
		return nil, err
	}

	forgetResult, err := lf.Execute(context.Background(), []*ypb.ExecParamItem{
		{
			Key:   "query",
			Value: promptBuffer.String(),
		},
	})
	if err != nil {
		return nil, err
	}

	if forgetResult.Action == nil {
		return nil, fmt.Errorf("extract action failed")
	}
	params := forgetResult.Action.GetInvokeParams("params")
	language := params.GetString("language")
	description := params.GetString("description")
	keywords := params.GetStringSlice("keywords")

	return &GenerateResult{
		Language:    language,
		Description: description,
		Keywords:    keywords,
	}, nil
}

func UpdateYakScriptMetaData(name string, content string, forceUpdate bool) (string, *metadata.YakScriptMetadata, error) {
	metadataIns, err := metadata.ParseYakScriptMetadata(name, content)
	if err != nil {
		log.Errorf("Failed to parse metadata for %s: %v", name, err)
		return content, nil, err
	}

	// 检查是否需要生成元数据
	needUpdate := forceUpdate || len(metadataIns.Keywords) == 0 || metadataIns.Description == ""
	if needUpdate { // 从代码中生成描述和关键词
		generatedMetadata, err := GenerateMetadataFromCodeContent(name, string(content))
		if err != nil {
			log.Errorf("Failed to generate metadata for tool: %s error: %v", metadataIns.Name, err)
			return content, nil, err
		}

		// 如果原元数据缺失，使用生成的元数据
		if metadataIns.Description == "" || forceUpdate {
			metadataIns.Description = generatedMetadata.Description
			log.Infof("Generated description for %s: %s", name, metadataIns.Description)
		}

		if len(metadataIns.Keywords) == 0 || forceUpdate {
			metadataIns.Keywords = generatedMetadata.Keywords
			log.Infof("Generated keywords for %s: %v", name, metadataIns.Keywords)
		}

		// 生成带有新Description和Keywords的脚本内容
		newContent := metadata.GenerateScriptWithMetadata(string(content), metadataIns.Description, metadataIns.Keywords)
		content = newContent
	}
	return content, metadataIns, err
}
