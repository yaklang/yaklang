package reactloops

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

var loops = new(sync.Map)

type LoopFactory func(r aicommon.AIInvokeRuntime) (*ReActLoop, error)

func Register(
	name string,
	creator LoopFactory,
) error {
	_, ok := loops.Load(name)
	if ok {
		return utils.Errorf("reactloop[%v] already exists", name)
	}
	loops.Store(name, creator)
	return nil
}

func CreateLoopByName(name string, invoker aicommon.AIInvokeRuntime) (*ReActLoop, error) {
	factory, ok := loops.Load(name)
	if !ok {
		return nil, utils.Errorf("reactloop[%v] not found", name)
	}
	factoryCreator, ok := factory.(LoopFactory)
	if !ok {
		return nil, utils.Errorf("reactloop[%v] type assert error", name)
	}
	return factoryCreator(invoker)
}
