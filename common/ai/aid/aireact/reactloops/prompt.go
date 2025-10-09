package reactloops

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed loop_template.tpl
var coreTemplate string

type basicPromptGetter interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)
}

func (r *ReActLoop) generateSchemaString(disallowExit bool) (string, error) {
	// loop
	// build in code
	values := r.actions.Values()
	if disallowExit {
		var filteredValues []*LoopAction
		for _, v := range values {
			if v.ActionType != loopAction_Finish.ActionType {
				filteredValues = append(filteredValues, v)
			} else {
				log.Warnf("action[%s] is removed from schema because loop exit is disallowed", v.ActionType)
			}
		}
		values = filteredValues
	}
	schema := buildSchema(values...)
	return schema, nil
}

func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
	operator *LoopActionHandlerOperator,
) (string, error) {
	background, extraInfos, err := r.GetInvoker().GetBasicPromptInfo(nil)
	schema, err := r.generateSchemaString(operator.disallowLoopExit)
	if err != nil {
		return "", err
	}

	var persistent string
	if r.persistentInstructionProvider != nil {
		persistent, err = r.persistentInstructionProvider(r, nonce)
		if err != nil {
			return "", utils.Wrap(err, "build persistent context failed")
		}
	}

	var outputExample string
	if r.reflectionOutputExampleProvider != nil {
		outputExample, err = r.reflectionOutputExampleProvider(r, nonce)
		if err != nil {
			return "", utils.Wrap(err, "build output example failed")
		}
	}

	var reactiveData string
	if r.reactiveDataBuilder != nil {
		reactiveData, err = r.reactiveDataBuilder(r, operator.GetFeedback(), nonce)
		if err != nil {
			return "", utils.Wrap(err, "build reactive data failed")
		}
	}

	_ = extraInfos
	prompt, err := utils.RenderTemplate(
		coreTemplate,
		map[string]any{
			"ReactiveData":      reactiveData,
			"Background":        background,
			"PersistentContext": persistent,
			"OutputExample":     outputExample,
			"Nonce":             nonce,
			"UserQuery":         userInput,
			"Schema":            schema,
		},
	)
	if err != nil {
		return "", utils.Wrap(err, "render loop prompt template failed")
	}
	return prompt, nil
}
