package reactloops

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/perception.txt
var perceptionPromptTemplate string

func buildPerceptionPrompt(input string, extra map[string]string) (string, error) {
	data := map[string]any{
		"Input": input,
	}
	for k, v := range extra {
		data[k] = v
	}
	return utils.RenderTemplate(perceptionPromptTemplate, data)
}
