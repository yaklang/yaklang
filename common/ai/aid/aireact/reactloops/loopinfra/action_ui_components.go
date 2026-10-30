package loopinfra

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
)

var uiComponentNamesSchema = map[string]any{
	"type": "array", "maxItems": interaction.UILoadMaxNames,
	"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
}

var uiComponentQuerySchema = map[string]any{"type": "string", "maxLength": 128}
var uiComponentOffsetSchema = map[string]any{"type": "integer", "minimum": 0, "maximum": interaction.MaxCatalogComponents, "default": 0}

var loadUIComponentsAction = &reactloops.LoopAction{
	ActionType:  "load_ui_components",
	Description: fmt.Sprintf("Load real UI component definitions before render_ui. names replaces the entire active batch (max %d, %d tokens); [] clears it. Reuse visible definitions. Task-local, no rendering or waiting; oversized batches are rejected unchanged.", interaction.UILoadMaxNames, interaction.UISchemaTokenBudget),
	Options:     []aitool.ToolOption{aitool.WithRawParam("names", uiComponentNamesSchema, aitool.WithParam_Required(true))},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		defer op.Continue()
		if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
			op.Feedback("Invalid load_ui_components request")
			return
		}
		raw, _ := lookupCanonicalActionParam(action, "names")
		if err := interaction.ValidateSchema(uiComponentNamesSchema, raw); err != nil {
			op.Feedback(fmt.Sprintf("names must be an array of up to %d nonempty component names; [] clears the batch", interaction.UILoadMaxNames))
			return
		}
		data, _ := json.Marshal(raw)
		var names []string
		_ = json.Unmarshal(data, &names)
		result, err := loop.LoadUIComponents(names)
		if err != nil {
			op.Feedback(err.Error())
			return
		}
		op.Feedback(result) // Small receipt only; schemas live in the dynamic view.
	},
}

var searchUIComponentsAction = &reactloops.LoopAction{
	ActionType:  "search_ui_components",
	Description: "Discover UI components by literal name/description substring (case-insensitive); query empty lists a page. Returns bounded summaries, not schemas. Follow nextOffset, then load_ui_components with selected names.",
	Options: []aitool.ToolOption{
		aitool.WithRawParam("query", uiComponentQuerySchema, aitool.WithParam_Required(true)),
		aitool.WithRawParam("offset", uiComponentOffsetSchema),
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		defer op.Continue()
		if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
			op.Feedback("Invalid search_ui_components request")
			return
		}
		query, _ := lookupCanonicalActionParam(action, "query")
		offset, exists := lookupCanonicalActionParam(action, "offset")
		if !exists {
			offset = float64(0)
		}
		if interaction.ValidateSchema(uiComponentQuerySchema, query) != nil || interaction.ValidateSchema(uiComponentOffsetSchema, offset) != nil {
			op.Feedback(fmt.Sprintf("query must be a string of at most 128 characters and offset an integer from 0 to %d", interaction.MaxCatalogComponents))
			return
		}
		data, _ := json.Marshal(offset)
		var start int
		_ = json.Unmarshal(data, &start)
		result, err := loop.SearchUIComponents(query.(string), start)
		if err != nil {
			op.Feedback(err.Error())
			return
		}
		data, _ = json.Marshal(result)
		op.Feedback(string(data))
	},
}
