package reactloops

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

var loops = new(sync.Map)
var actions = new(sync.Map)

func RegisterAction(name string, action *LoopAction) {
	actions.Store(name, action)
}

func GetLoopAction(name string) (*LoopAction, bool) {
	action, ok := actions.Load(name)
	if !ok {
		return nil, false
	}
	actionObj, ok := action.(*LoopAction)
	if !ok {
		return nil, false
	}
	return actionObj, true
}

type LoopFactory func(r aicommon.AIInvokeRuntime) (*ReActLoop, error)

func RegisterLoopFactory(
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
