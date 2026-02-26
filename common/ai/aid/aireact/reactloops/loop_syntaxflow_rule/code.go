package loop_syntaxflow_rule

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
				r,
				loopinfra.WithLoopVarsPrefix("sf"),
				loopinfra.WithActionSuffix("rule"), // write_rule, modify_rule, insert_rule, delete_rule
				loopinfra.WithAITagConfig("GEN_RULE", "sf_rule", "syntaxflow-rule", "text/syntaxflow"),
				loopinfra.WithFileExtension(".sf"),
				loopinfra.WithFileChanged(func(content string, op *reactloops.LoopActionHandlerOperator) (string, bool) {
					return checkSyntaxFlowAndFormatErrors(content)
				}),
				loopinfra.WithEventType("syntaxflow_rule_editor"),
			)

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				modSuite.GetAITagOption(),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					sfCode := loop.Get("full_sf_code")
					codeWithLine := utils.PrefixLinesWithLineNumbers(sfCode)
					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)
					renderMap := map[string]any{
						"Code":                      sfCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
			}
			preset = append(preset, modSuite.GetActions()...)
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW, r, preset...)
		},
		reactloops.WithLoopDescription("Enter focused mode for SyntaxFlow rule generation and modification with real-time syntax validation"),
		reactloops.WithLoopUsagePrompt("Use when user requests to write, modify, or debug SyntaxFlow vulnerability detection rules. Provides tools: write_rule, modify_rule, insert_rule, delete_rule with real-time SyntaxFlow compile validation"),
		reactloops.WithLoopOutputExample(`
* When user requests to write SyntaxFlow rule:
  {"@action": "write_syntaxflow_rule", "human_readable_thought": "I need to write a SyntaxFlow rule for vulnerability detection"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW)
	}
}
