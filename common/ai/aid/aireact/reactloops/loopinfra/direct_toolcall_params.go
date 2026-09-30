package loopinfra

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func readDirectToolCallParams(loop *reactloops.ReActLoop, action *aicommon.Action, tool *aitool.Tool) (aitool.InvokeParams, []string) {
	raw, object := getDirectlyCallToolParamPayload(action)
	params, _ := normalizeDirectlyCallToolParams(raw, object)
	if params == nil {
		params = make(aitool.InvokeParams)
	}
	blocks := aicommon.MergeActionAITagParams(action, params, getDirectlyCallToolParamNames(loop, tool.Name))
	return params, blocks
}

func PrepareDirectToolCallParams(loop *reactloops.ReActLoop, action *aicommon.Action, tool *aitool.Tool) (aitool.InvokeParams, error) {
	params, _ := readDirectToolCallParams(loop, action, tool)
	if valid, validationErrors := tool.ValidateParams(params); !valid {
		return nil, utils.Errorf("invalid params for %q: %s", tool.Name, strings.Join(validationErrors, "; "))
	}
	identifier := action.GetString("directly_call_identifier")
	if identifier == "" {
		identifier = action.GetString("identifier")
	}
	if identifier != "" {
		params[aicommon.ReservedKeyIdentifier] = identifier
	}
	if expectations := action.GetString("directly_call_expectations"); expectations != "" {
		params[aicommon.ReservedKeyCallExpectations] = expectations
	}
	return params, nil
}
