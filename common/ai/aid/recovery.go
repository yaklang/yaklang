package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func NewRecoveredCoordinator(
	ctx context.Context,
	pt *schema.AIAgentRuntime,
	options ...aicommon.ConfigOption,
) (*Coordinator, error) {
	if pt.Uuid == "" {
		return nil, utils.Error("cannot recover coordinator at this time, no coordinator uuid")
	}
	return NewCoordinatorContext(ctx, pt.GetUserInput(), append(options, aicommon.WithID(pt.Uuid),aicommon.WithSequence(pt.Seq))...)
}

func NewFastRecoverCoordinatorContext(ctx context.Context, uuid string, opt ...aicommon.ConfigOption,) (*Coordinator, error) {
	rt, err := yakit.GetAgentRuntime(consts.GetGormProjectDatabase(), uuid)
	if err != nil {
		return nil, utils.Errorf("coordinator: get runtime failed: %v", err)
	}
	return NewRecoveredCoordinator(ctx, rt, opt...)
}

func NewFastRecoverCoordinator(uuid string, opt ...aicommon.ConfigOption,) (*Coordinator, error) {
	return NewFastRecoverCoordinatorContext(context.Background(), uuid, opt...)
}
