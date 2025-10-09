package loop_default

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	LOOP_NAME_DEFAULT = "default"
)

//go:embed prompts/instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		LOOP_NAME_DEFAULT,
		func(r aicommon.AIInvokeRuntime) (*reactloops.ReActLoop, error) {
			loop, err := reactloops.NewReActLoop(
				LOOP_NAME_DEFAULT,
				r,
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(true),
				reactloops.WithAllowPlanAndExec(true),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
			)
			return loop, err
		},
	)
	if err != nil {
		log.Errorf("build default react loop failed: %v", err)
	}
}
