package loopinfra

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
)

var renderUIAction = &reactloops.LoopAction{
	ActionType:  "render_ui",
	Description: "Present a small independent UI block using ONLY the negotiated component catalog. Never output HTML/CSS/JS, functions or expressions. Give properties in catalog order and declared types. Use stable document/node IDs and explicit children IDs. Supply meaningful plain-text fallback. Local input stays on the client until submit_to_chat. This call does not wait. Diagnostics permit ONE correction with the SAME IDs; invalid properties will not take effect.",
	Options: []aitool.ToolOption{aitool.WithRawParam("ui", map[string]any{
		"type": "object", "required": []string{"id", "catalogVersion", "fallback", "root", "nodes"},
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "catalogVersion": map[string]any{"type": "string"},
			"fallback": map[string]any{"type": "string"}, "root": map[string]any{"type": "string"},
			"nodes": map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{
				"type": "object", "required": []string{"id", "component", "props"}, "properties": map[string]any{
					"id": map[string]any{"type": "string"}, "component": map[string]any{"type": "string"},
					"props":    map[string]any{"type": "object"},
					"children": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"bindings": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}},
					"action": map[string]any{"type": "object", "required": []string{"name", "args"}, "properties": map[string]any{
						"name": map[string]any{"type": "string", "enum": []string{"submit_to_chat"}}, "args": map[string]any{"type": "object"},
					}},
				},
			}},
		},
	}, aitool.WithParam_Required(true))},
	StreamFields: []*reactloops.LoopStreamField{{FieldName: "ui", AINodeId: "ui_component", ContentType: interaction.ComponentContentType, RawJSON: true}},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		defer op.Continue()
		cfg, ok := loop.GetConfig().(*aicommon.Config)
		if !ok {
			op.Feedback("UI is not available")
			return
		}
		params := action.GetInvokeParams("ui")
		if len(params) == 0 {
			params = action.GetInvokeParams("next_action").GetObject("ui")
		}
		raw, err := json.Marshal(params)
		if err != nil || len(raw) > 256*1024 {
			op.Feedback("UI document exceeds 256 KiB or is invalid")
			return
		}
		var doc interaction.ComponentDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			op.Feedback(err.Error())
			return
		}
		ctx, taskID := cfg.GetContext(), ""
		if task := loop.GetCurrentTask(); task != nil {
			ctx, taskID = task.GetContext(), task.GetId()
		}
		result, err := cfg.RenderUI(ctx, taskID, doc)
		if err != nil {
			op.Feedback(err.Error())
		} else {
			op.Feedback(result)
			if feedback, ok := result.(map[string]any); ok && feedback["correctionAllowed"] == true {
				op.NextAction("render_ui")
			}
		}
	},
}
