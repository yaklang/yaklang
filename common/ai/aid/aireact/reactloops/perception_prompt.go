package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

	"github.com/yaklang/yaklang/common/utils"
)

var perceptionPromptTemplate = promptloader.MustLoad("ai/aid/aireact/reactloops/prompts/perception.txt")

func buildPerceptionPrompt(input string, extra map[string]string) (string, error) {
	data := map[string]any{
		"Input": input,
	}
	for k, v := range extra {
		data[k] = v
	}
	return utils.RenderTemplate(perceptionPromptTemplate, data)
}
