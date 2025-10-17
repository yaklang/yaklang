package reactloops

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func ConvertReActLoopFactoryToActionFactory(
	name string,
	factory LoopFactory,
) func(r aicommon.AIInvokeRuntime) (*LoopAction, error) {
	return func(r aicommon.AIInvokeRuntime) (*LoopAction, error) {
		if utils.IsNil(r) {
			return nil, utils.Errorf("runtime is nil when creating loop action: %s", name)
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
				var err error
				loop, err := factory(r, WithOnPostIteraction(oldLoop.onPostIteration))
				if err != nil {
					operator.Fail(err.Error())
					return
				}
				emitter := oldLoop.GetEmitter()
				invoker := oldLoop.GetInvoker()
				msg := fmt.Sprintf(
					"AI decided to focus on the loop: %v", name,
				)
				invoker.AddToTimeline("focus-on", msg)

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
				err = loop.ExecuteWithExistedTask(current)
				if err != nil {
					operator.Fail(err.Error())
					return
				}
				// exit with no error
				invoker.AddToTimeline("lose-focus", "AI finished focus on the loop["+name+"]")
				operator.Exit()
			},
		}
		return action, nil
	}
}
