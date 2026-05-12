package aid

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

type deepthinkDynamicData struct {
	Progress        string
	CurrentTaskGoal string
	UserInput       string
	PlanHelp        string
}

type dynamicPlanDynamicData struct {
	CurrentTaskInfo   string
	UserInput         string
	PlanHelp          string
	StableInstruction string
}

type planReviewDynamicData struct {
	CurrentPlanDetail string
	UserSuggestion    string
	ExtraPrompt       string
	PlanHelp          string
	OriginalUserInput string
}

type freedomReviewDynamicData struct {
	CurrentPlanDetail string
	UserReviewPlan    string
	PlanHelp          string
	OriginalUserInput string
}

type createSubtaskDynamicData struct {
	CurrentPlanDetail string
	TargetPlansDetail string
	ExtraPrompt       string
	PlanHelp          string
	OriginalUserInput string
}

func newAidPromptMaterials(instruction string, schema string) *aicommon.PromptMaterials {
	return &aicommon.PromptMaterials{
		TaskInstruction: strings.TrimSpace(instruction),
		Schema:          schema,
	}
}

func newAidPlanReviewPromptMaterials(pr *planRequest, instruction string, schemaKey string) *aicommon.PromptMaterials {
	return newAidPromptMaterials(instruction, pr.cod.ContextProvider.Schema()[schemaKey])
}

func (pr *planRequest) assemblePlanReviewPrompt(
	instruction string,
	schemaKey string,
	dynamicTemplateName string,
	dynamicTemplate string,
	dynamicData any,
) (string, string, error) {
	nonce := utils.RandStringBytes(6)
	prompt, err := aicommon.NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		newAidPlanReviewPromptMaterials(pr, instruction, schemaKey),
		dynamicTemplateName,
		dynamicTemplate,
		dynamicData,
		nonce,
	)
	return prompt, nonce, err
}

func buildCreateSubtaskTargetPlansDetail(c *Coordinator, targetPlans []string) string {
	if c == nil || len(targetPlans) == 0 {
		return "用户未显式指定 target plans，需要你结合当前任务树自行判断哪些叶子任务更适合继续拆分。"
	}

	lines := make([]string, 0, len(targetPlans)+1)
	lines = append(lines, "用户指定需要重点拆分的任务如下：")
	for _, index := range targetPlans {
		index = strings.TrimSpace(index)
		if index == "" {
			continue
		}
		task := c.FindSubtaskByIndex(index)
		if task == nil {
			lines = append(lines, fmt.Sprintf("- %s: 当前任务树中未找到该索引，请结合计划上下文自行判断。", index))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s；目标：%s", index, task.Name, task.Goal))
	}
	return strings.Join(lines, "\n")
}

func (t *AiTask) buildDeepthinkPrompt(userInput string) (string, error) {
	builder := aicommon.NewDefaultPromptPrefixBuilder()
	nonce := utils.RandStringBytes(6)
	materials := newAidPromptMaterials(__prompt_deepthinkInstruction, t.ContextProvider.Schema()["PlanJsonSchema"])
	return builder.AssemblePromptWithDynamicSection(
		materials,
		"aid-deepthink-dynamic",
		__prompt_deepthinkDynamic,
		deepthinkDynamicData{
			Progress:        t.ContextProvider.Progress(),
			CurrentTaskGoal: t.ContextProvider.CurrentTask.Goal,
			UserInput:       strings.TrimSpace(userInput),
			PlanHelp:        t.ContextProvider.PlanHelp(),
		},
		nonce,
	)
}

func (t *AiTask) buildDynamicPlanPrompt(userInput string) (string, error) {
	frozen, open := t.ContextProvider.TimelineDumpFrozenOpen()
	builder := aicommon.NewDefaultPromptPrefixBuilder()
	nonce := utils.RandStringBytes(6)
	materials := newAidPromptMaterials(__prompt_dynamicPlanInstruction, t.ContextProvider.Schema()["RePlanJsonSchema"])
	materials.TimelineFrozen = frozen
	materials.TimelineOpen = open
	return builder.AssemblePromptWithDynamicSection(
		materials,
		"aid-dynamic-plan-dynamic",
		__prompt_dynamicPlanDynamic,
		dynamicPlanDynamicData{
			CurrentTaskInfo:   t.ContextProvider.CurrentTaskInfoDynamic(),
			UserInput:         strings.TrimSpace(userInput),
			PlanHelp:          t.ContextProvider.PlanHelp(),
			StableInstruction: t.ContextProvider.CurrentTaskInfoStable(),
		},
		nonce,
	)
}

func (pr *planRequest) buildPlanIncompletePrompt(suggestion string, extraPrompt string, rsp *PlanResponse) (string, string, error) {
	return pr.assemblePlanReviewPrompt(
		__prompt_planIncompleteInstruction,
		"PlanJsonSchema",
		"aid-plan-review-incomplete-dynamic",
		__prompt_planIncompleteDynamic,
		planReviewDynamicData{
			CurrentPlanDetail: rsp.RootTask.ProgressWithDetail(),
			UserSuggestion:    suggestion,
			ExtraPrompt:       extraPrompt,
			PlanHelp:          pr.cod.ContextProvider.PlanHelp(),
			OriginalUserInput: pr.cod.ContextProvider.Query,
		},
	)
}

func (pr *planRequest) buildFreedomReviewPrompt(extraPrompt string, rsp *PlanResponse) (string, string, error) {
	return pr.assemblePlanReviewPrompt(
		__prompt_planFreedomReviewInstruction,
		"PlanJsonSchema",
		"aid-plan-review-freedom-dynamic",
		__prompt_planFreedomReviewDynamic,
		freedomReviewDynamicData{
			CurrentPlanDetail: rsp.RootTask.ProgressWithDetail(),
			UserReviewPlan:    extraPrompt,
			PlanHelp:          pr.cod.ContextProvider.PlanHelp(),
			OriginalUserInput: pr.cod.ContextProvider.Query,
		},
	)
}

func (pr *planRequest) buildCreateSubtaskPrompt(extraPrompt string, targetPlans []string, rsp *PlanResponse) (string, string, error) {
	return pr.assemblePlanReviewPrompt(
		__prompt_planCreateSubtaskInstruction,
		"PlanCreateSubtaskSchema",
		"aid-plan-review-create-subtask-dynamic",
		__prompt_planCreateSubtaskDynamic,
		createSubtaskDynamicData{
			CurrentPlanDetail: rsp.RootTask.ProgressWithDetail(),
			TargetPlansDetail: buildCreateSubtaskTargetPlansDetail(pr.cod, targetPlans),
			ExtraPrompt:       extraPrompt,
			PlanHelp:          pr.cod.ContextProvider.PlanHelp(),
			OriginalUserInput: pr.cod.ContextProvider.Query,
		},
	)
}
