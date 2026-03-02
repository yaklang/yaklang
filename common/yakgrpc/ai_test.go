package yakgrpc

import (
	"testing"
)

func TestAcc(t *testing.T) {
	// r, err := aireact.NewReAct(aicommon.WithDebug(true))
	// require.NoError(t, err)

	// currentTask := aicommon.NewStatefulTaskBase(
	// 	"plan",
	// 	"对www.a.com进行web漏洞扫描，这是我的内网域名，不要问我其他问题了",
	// 	r.GetConfig().GetContext(),
	// 	r.Emitter,
	// )

	// task, err := r.ExecuteLoopTask(schema.AI_REACT_LOOP_NAME_PLAN, currentTask, reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
	// 	if isDone {
	// 		fmt.Println(loop.Get("plan_data"))
	// 	}

	// }))
	// if err != nil {
	// 	return
	// }
	// require.NotNil(t, task)
	// require.NoError(t, err)
}
