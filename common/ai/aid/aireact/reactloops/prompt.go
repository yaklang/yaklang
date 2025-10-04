package reactloops

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed loop_template.tpl
var coreTemplate string

func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
) (string, error) {
	schema, err := r.generateSchemaString()
	if err != nil {
		return "", err
	}

	return utils.RenderTemplate(
		coreTemplate,
		map[string]any{
			"Nonce":     nonce,
			"UserQuery": userInput,
			"Schema":    schema,
		},
	)
}
