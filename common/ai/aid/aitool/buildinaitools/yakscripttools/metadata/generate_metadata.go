package metadata

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed prompt/generate_keyword.txt
var aitool_generate_key_word_prompt string

//go:embed prompt/generate_keyword_forge.txt
var aiforge_generate_key_word_prompt string

//go:embed prompt/generate_keyword_yakscript.txt
var aiyakscript_generate_key_word_prompt string

// 定义结构体来存储结果
type GenerateResult struct {
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

func GenerateYakScriptMetadata(script string) (*GenerateResult, error) {
	return generateMetadata(script, aiyakscript_generate_key_word_prompt, false)
}

func GenerateYakScriptAIToolMetadata(code string) (*GenerateResult, error) {
	return generateMetadata(code, aitool_generate_key_word_prompt, true)
}

func GenerateForgeMetadata(forgeContent string) (*GenerateResult, error) {
	return generateMetadata(forgeContent, aiforge_generate_key_word_prompt, true)
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

	// Get response from AI
	response, err := ai.Chat(promptBuffer.String(), aispec.WithDebugStream(debug))
	if err != nil {
		log.Errorf("failed to get AI response: %v", err)
		return nil, fmt.Errorf("failed to get AI response: %v", err)
	}

	// Extract JSON object from the response
	var result GenerateResult
	index := jsonextractor.ExtractObjectIndexes(response)
	for _, pair := range index {
		err = json.Unmarshal([]byte(response[pair[0]:pair[1]]), &result)
		if err == nil && len(result.Keywords) > 0 && result.Description != "" {
			// 如果描述包含引号或反斜杠，可能需要特殊处理
			result.Description = strings.ReplaceAll(result.Description, "\\", "\\\\")
			result.Description = strings.ReplaceAll(result.Description, "\"", "\\\"")

			// 确保所有关键词都是小写的
			for i, keyword := range result.Keywords {
				result.Keywords[i] = strings.ToLower(keyword)
			}

			return &result, nil
		}
	}
	return nil, fmt.Errorf("failed to extract valid metadata from AI response")
}
