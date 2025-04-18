package aiforge

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"text/template"
)

//go:embed forgeprompts/forge-arguments.txt
var _promptArguments string

type ForgeArgument struct {
	Help  string
	Value string
	Name  string
}

type ForgeArguments []*ForgeArgument

func (a ForgeArguments) String() string {
	tmpl, err := template.New("arguments").Parse(_promptArguments)
	if err != nil {
		log.Error(err)
		return ""
	}
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, map[string]any{
		"Arguments": a,
	})
	if err != nil {
		log.Errorf("error executing suggestion template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (f *ForgeBlueprint) AnalyzeCliParameter(input []*ypb.ExecParamItem) (ForgeArguments, error) {
	var args []*ForgeArgument
	inputMap := make(map[string]any)
	for _, item := range input {
		inputMap[item.Key] = item.Value
	}

	cliParamInfo := f.GenerateParameter()
	for _, param := range cliParamInfo.CliParameter {
		var arg = &ForgeArgument{
			Help: param.Help,
			Name: param.Field,
		}
		userInput, ok := inputMap[param.Field]
		if !ok {
			if param.Required {
				return nil, utils.Errorf("Required %s field not set", param.Field)
			}
			arg.Value = param.DefaultValue
		} else {
			arg.Value = codec.AnyToString(userInput)
		}
		args = append(args, arg)
	}
	return args, nil
}
