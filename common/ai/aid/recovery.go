package aid

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func NewRecoveredCoordinator(
	ctx context.Context,
	pt *schema.AiCoordinatorRuntime,
	options ...Option,
) (*Coordinator, error) {
	if pt.Uuid == "" {
		return nil, utils.Error("cannot recover coordinator at this time, no coordinator uuid")
	}
	config := newConfigEx(ctx, pt.Uuid, pt.Seq)
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return nil, utils.Errorf("coordinator: apply option failed: %v", err)
		}
	}
	config.startEventLoop(ctx)

	if config.aiToolManager == nil {
		config.aiToolManager = buildinaitools.NewToolManager(config.tools)
	}
	c := &Coordinator{
		config:    config,
		userInput: pt.GetUserInput(),
	}
	config.memory.StoreQuery(c.userInput)
	config.memory.StoreTools(func() []*aitool.Tool {
		alltools, err := config.aiToolManager.GetAllTools()
		if err != nil {
			log.Errorf("coordinator: get all tools failed: %v", err)
			return nil
		}
		return alltools
	})
	return c, nil
}

func NewFastRecoverCoordinatorContext(ctx context.Context, uuid string, opt ...Option) (*Coordinator, error) {
	rt, err := aiddb.GetCoordinatorRuntime(consts.GetGormProjectDatabase(), uuid)
	if err != nil {
		return nil, utils.Errorf("coordinator: get runtime failed: %v", err)
	}
	return NewRecoveredCoordinator(ctx, rt, opt...)
}

func NewFastRecoverCoordinator(uuid string, opt ...Option) (*Coordinator, error) {
	return NewFastRecoverCoordinatorContext(context.Background(), uuid, opt...)
}
