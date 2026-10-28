package aive

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func init() {
	aicommon.RegisterTrajectoryEvaluator(submitTrajectoryEvaluation)
}
