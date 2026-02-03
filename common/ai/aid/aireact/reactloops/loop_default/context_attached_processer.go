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
	originOptions := r.GetConfig().OriginOptions()

	var opts []any
	for _, option := range originOptions {
		opts = append(opts, option)
	}

	haveKnowledgeBase := false
	attachedResult := task.GetAttachedDatas()
	for _, data := range attachedResult {
		if data.Type == aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE {
			haveKnowledgeBase = true
			break
		}
	}

	if !haveKnowledgeBase {
		return nil
	}

	var knowledgeEnhanceLoop *reactloops.ReActLoop
	opts = append(opts, reactloops.WithActionFactoryFromLoop(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE), reactloops.WithOnLoopInstanceCreated(func(loop *reactloops.ReActLoop) {
		knowledgeEnhanceLoop = loop
	}))

	ok, err := r.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, newTask, opts...)

	var searchResultsSummary string

	finalSummary := knowledgeEnhanceLoop.Get("final_summary")
	if finalSummary != "" {
		searchResultsSummary = finalSummary
	} else {
		searchResultsSummary = loop.Get("search_results_summary")
	}

	if searchResultsSummary != "" {
		loop.GetInvoker().AddToTimeline("knowledge_search_result_summary", searchResultsSummary)
		loop.GetInvoker().AddToTimeline("import notice", "knowledge_search_result_summary has been set, no need to search the knowledge base again as it has already been queried")
	}

	if err != nil {
		return utils.Wrap(err, "failed to execute loop task")
	}
	if !ok {
		return utils.Errorf("failed to execute loop task")
	}
	return nil
}
