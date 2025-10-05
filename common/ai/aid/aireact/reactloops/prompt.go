package reactloops

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed loop_template.tpl
var coreTemplate string

type basicPromptGetter interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)
}

func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
) (string, error) {
	getter, ok := r.config.(basicPromptGetter)
	if ok {
		return "", utils.Errorf("config does not implement GetBasicPromptInfo")
	}
	background, extraInfos, err := getter.GetBasicPromptInfo(nil)
	schema, err := r.generateSchemaString()
	if err != nil {
		return "", err
	}

	_ = extraInfos
	return utils.RenderTemplate(
		coreTemplate,
		map[string]any{
			"Background": background,
			"Nonce":      nonce,
			"UserQuery":  userInput,
			"Schema":     schema,
		},
	)
}
