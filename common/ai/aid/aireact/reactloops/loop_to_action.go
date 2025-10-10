package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func ConvertReActLoopFactoryToActionFactory(
	name string,
	factory func(r aicommon.AIInvokeRuntime) (*ReActLoop, error),
) func(r aicommon.AIInvokeRuntime) (*LoopAction, error) {
	return func(r aicommon.AIInvokeRuntime) (*LoopAction, error) {
		if utils.IsNil(r) {
			return nil, utils.Errorf("runtime is nil when creating loop action: %s", name)
		}

		loop, err := factory(r)
		if err != nil {
			return nil, err
		}
		action := &LoopAction{
			ActionType:   name,
			Description:  "focus on solving the problem using [" + name + "] loop",
			StreamFields: []*LoopStreamField{},
			ActionVerifier: func(oldLoop *ReActLoop, action *aicommon.Action) error {
				_, ok := oldLoop.actions.Get(name)
				if ok {
					return utils.Errorf("action type %s already exists in the loop", name)
				}
				return nil
			},
			ActionHandler: func(oldLoop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
				emitter := oldLoop.GetEmitter()
				emitter.EmitJSON(schema.EVENT_TYPE_FOCUS_ON_LOOP, "focus-on", map[string]any{
					"loop": name,
				})
				defer func() {
					emitter.EmitJSON(schema.EVENT_TYPE_LOSE_FOCUS_LOOP, "lose-focus", map[string]any{
						"from_loop": name,
						"to_loop":   oldLoop.loopName,
					})
				}()
				current := oldLoop.GetCurrentTask()
				err := loop.ExecuteWithExistedTask(current)
				if err != nil {
					operator.Fail(err.Error())
					return
				}
				// exit with no error
				operator.Exit()
			},
		}
		return action, nil
	}
}
