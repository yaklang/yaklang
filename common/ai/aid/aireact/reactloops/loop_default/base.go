package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	LOOP_NAME_DEFAULT = "default"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		LOOP_NAME_DEFAULT,
		func(r aicommon.AIInvokeRuntime) (*reactloops.ReActLoop, error) {
			loop, err := reactloops.NewReActLoop(
				LOOP_NAME_DEFAULT,
				r,
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithUserInteract(r.GetConfig().GetAllowUserInteraction()),
			)
			return loop, err
		},
	)
	if err != nil {
		log.Errorf("build default react loop failed: %v", err)
	}
}
