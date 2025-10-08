package aireact

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_yaklangcode"
)

func (r *ReAct) invokeWriteYaklangCode(task aicommon.AIStatefulTask, approach string) (string, error) {
	loop, err := reactloops.CreateLoopByName(loop_yaklangcode.LOOP_NAME_WRITE_YAKLANG_CODE, r)
	if err != nil {
		return "", err
	}
	task.SetUserInput(approach)
	err = loop.ExecuteWithExistedTask(task)
	if err != nil {
		return "", err
	}
	return loop.Get("filename"), nil
}
