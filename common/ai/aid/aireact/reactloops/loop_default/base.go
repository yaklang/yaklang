package loop_default

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed prompts/instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(true),
				reactloops.WithAllowPlanAndExec(true),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithActionFactoryFromLoop(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG),
			}
			preset = append(preset, opts...)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r, preset...)
			return loop, err
		},
	)
	if err != nil {
		log.Errorf("build default react loop failed: %v", err)
	}
}
