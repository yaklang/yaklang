package loop_default

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func ProcessAttachedData(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	// 新建任务 让 ai 根据用户输入和提及信息进行增强知识回答

	newTask := aicommon.NewStatefulTaskBase(
		task.GetId(),
		fmt.Sprintf("Please answer the user's question based on the attached data, user input: %s", task.GetUserInput()),
		r.GetConfig().GetContext(),
		r.GetConfig().GetEmitter(),
	)

	newTask.SetAttachedDatas(task.GetAttachedDatas())

	ok, err := r.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, newTask, reactloops.WithActionFactoryFromLoop(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE))

	if err != nil {
		return utils.Wrap(err, "failed to execute loop task")
	}
	if !ok {
		return utils.Errorf("failed to execute loop task")
	}
	return nil
}
