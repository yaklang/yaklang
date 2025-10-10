package reactloops

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed loop_template.tpl
var coreTemplate string

func (r *ReActLoop) generateSchemaString(disallowExit bool) (string, error) {
	// loop
	// build in code
	values := r.GetAllActions()
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
	background, infos, err := r.getRenderInfo()
	if err != nil {
		return "", utils.Wrap(err, "get basic prompt info failed")
	}
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

	infos["ReactiveData"] = reactiveData
	infos["Background"] = background
	infos["PersistentContext"] = persistent
	infos["OutputExample"] = outputExample
	infos["Nonce"] = nonce
	infos["UserQuery"] = userInput
	infos["Schema"] = schema
	prompt, err := utils.RenderTemplate(coreTemplate, infos)
	if err != nil {
		return "", utils.Wrap(err, "render loop prompt template failed")
	}
	return prompt, nil
}
