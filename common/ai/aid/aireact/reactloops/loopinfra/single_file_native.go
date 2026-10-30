package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Native edits carry their body on the individual call. Never read a previous
// AITAG's loop variable: a native response can contain several different edits.
func (f *SingleFileModificationSuiteFactory) actionContent(loop *reactloops.ReActLoop, action *aicommon.Action) string {
	if loop.FunctionCallModeEnabled() {
		return action.GetString("content")
	}
	return loop.Get(f.GetCodeVariableName())
}

func (f *SingleFileModificationSuiteFactory) withNativeContent(register reactloops.ReActLoopOption, verb string) reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		register(loop)
		original, err := loop.GetActionHandler(f.GetActionName(verb))
		if err != nil {
			return
		}
		action := *original
		action.NativeDescription = map[string]string{
			"write":  "Create the file only when it is empty. Supply the complete initial body in content. Use modify or insert for an existing file.",
			"modify": "Modify the existing file using content: an Apply Patch block (*** Begin Patch ... *** End Patch), or a replacement snippet with old_snippet or a 1-based inclusive line range. Do not replace the whole file without an explicit range.",
			"insert": "Insert content before insert_line (1-based), preserving existing file content.",
		}[verb]
		action.NativeOptions = []aitool.ToolOption{
			aitool.WithStringParam("content", aitool.WithParam_Required(true),
				aitool.WithParam_Description("Exact text to write, replace or insert, including all Markdown/code and newlines. Supply it entirely in this argument.")),
		}
		switch verb {
		case "modify":
			action.NativeOptions = append(action.NativeOptions,
				aitool.WithIntegerParam("modify_start_line", aitool.WithParam_Description("1-based inclusive start; omit when content is a patch or old_snippet is provided.")),
				aitool.WithIntegerParam("modify_end_line", aitool.WithParam_Description("1-based inclusive end.")),
				aitool.WithStringParam("old_snippet", aitool.WithParam_Description("Exact existing text to replace with content.")),
				aitool.WithBoolParam("replace_all"),
				aitool.WithStringParam("modify_code_reason"),
			)
		case "insert":
			action.NativeOptions = append(action.NativeOptions, aitool.WithIntegerParam("insert_line", aitool.WithParam_Required(true)))
		}
		action.StreamFields = append(append([]*reactloops.LoopStreamField(nil), original.StreamFields...), &reactloops.LoopStreamField{
			FieldName: "content", AINodeId: f.aiNodeId, ContentType: f.contentType,
		})
		action.ActionVerifier = func(l *reactloops.ReActLoop, call *aicommon.Action) error {
			if l.FunctionCallModeEnabled() && call.GetString("content") == "" {
				return fmt.Errorf("%s requires non-empty arguments.content", action.ActionType)
			}
			if original.ActionVerifier != nil {
				return original.ActionVerifier(l, call)
			}
			return nil
		}
		action.ActionHandler = func(l *reactloops.ReActLoop, call *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			// A preceding call in the same native batch may have changed the file.
			if l.FunctionCallModeEnabled() {
				if err := action.ActionVerifier(l, call); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
			}
			original.ActionHandler(l, call, op)
		}
		reactloops.WithOverrideLoopAction(&action)(loop)
	}
}
