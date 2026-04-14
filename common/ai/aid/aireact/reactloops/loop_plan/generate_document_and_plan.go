package loop_plan

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	PlanDocumentFieldName = "document"
	PlanDocumentAINodeID  = "plan-document"

	PlanTasksAINodeID = "plan-tasks"
)

func generateGuidanceDocument(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) string {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	facts := loop.Get(PLAN_FACTS_KEY)
	evidence := getLoopTaskEvidenceDocument(loop)
	loopContext := getLoopTaskContext(loop)

	templateData := loop.GetBaseFrameContext()
	templateData["UserInput"] = userInput
	templateData["Facts"] = facts
	templateData["Evidence"] = evidence
	templateData["Context"] = loopContext

	prompt, err := utils.RenderTemplate(guidanceDocumentPrompt, templateData)
	if err != nil {
		log.Warnf("plan loop: render guidance document prompt failed: %v", err)
		return ""
	}

	loop.LoadingStatus("正在生成任务执行指导文档... / Generating guidance document...")

	taskIndex := ""
	if task != nil {
		taskIndex = task.GetId()
	}
	emitter := loop.GetEmitter()

	action, err := invoker.InvokeQualityPriorityLiteForge(
		ctx,
		"plan_guidance_document",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam(PlanDocumentFieldName,
				aitool.WithParam_Description("基于控制论与科学方法论框架组织的任务执行指导性文档，Markdown 格式，包含目标定义、系统现状分析、差距分析、控制策略、执行路径、验证与反馈六个章节"),
				aitool.WithParam_Required(true),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldCallback(
			[]string{PlanDocumentFieldName},
			func(key string, r io.Reader) {
				r = utils.JSONStringReader(r)
				if emitter == nil {
					io.Copy(io.Discard, r)
					return
				}
				emitter.EmitTextMarkdownStreamEvent(PlanDocumentAINodeID, r, taskIndex)
			},
		),
	)
	if err != nil {
		log.Warnf("plan loop: generate guidance document failed: %v", err)
		return ""
	}
	if action == nil {
		return ""
	}
	document := strings.TrimSpace(action.GetString(PlanDocumentFieldName))
	if document == "" {
		log.Warnf("plan loop: guidance document is empty")
		return ""
	}
	log.Infof("plan loop: guidance document generated successfully (%d chars)", len(document))
	return document
}

func emitDocumentMarkdown(loop *reactloops.ReActLoop, document string) {
	document = strings.TrimSpace(document)
	if document == "" {
		return
	}

	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(PlanDocumentAINodeID, strings.NewReader(document), taskIndex, func() {}); err != nil {
			log.Warnf("plan loop: emit guidance document markdown failed: %v", err)
		}
	}
}

func generatePlanFromDocument(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) string {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	templateData := loop.GetBaseFrameContext()
	templateData["UserInput"] = userInput
	templateData["Document"] = loop.Get(PLAN_DOCUMENT_KEY)
	templateData["Facts"] = loop.Get(PLAN_FACTS_KEY)
	templateData["Context"] = getLoopTaskContext(loop)

	prompt, err := utils.RenderTemplate(planFromDocumentPrompt, templateData)
	if err != nil {
		log.Warnf("plan loop: render plan_from_document prompt failed: %v", err)
		return ""
	}

	loop.LoadingStatus("正在生成任务计划... / Generating execution plan...")

	taskIndex := ""
	if task != nil {
		taskIndex = task.GetId()
	}
	emitter := loop.GetEmitter()

	action, err := invoker.InvokeQualityPriorityLiteForge(
		ctx,
		"plan_from_document",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("main_task", aitool.WithParam_Required(true)),
			aitool.WithStringParam("main_task_identifier"),
			aitool.WithStringParam("main_task_goal", aitool.WithParam_Required(true)),
			aitool.WithStructArrayParam(
				"tasks",
				nil,
				nil,
				aitool.WithStringParam("subtask_name", aitool.WithParam_Required(true)),
				aitool.WithStringParam("subtask_identifier"),
				aitool.WithStringParam("subtask_goal", aitool.WithParam_Required(true)),
				aitool.WithStringArrayParam("depends_on"),
				aitool.WithStructArrayParam(
					"sub_subtasks",
					nil,
					nil,
					aitool.WithStringParam("subtask_name", aitool.WithParam_Required(true)),
					aitool.WithStringParam("subtask_identifier"),
					aitool.WithStringParam("subtask_goal", aitool.WithParam_Required(true)),
					aitool.WithStringArrayParam("depends_on"),
				),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldCallback(
			[]string{"tasks"},
			func(key string, r io.Reader) {
				if emitter == nil {
					io.Copy(io.Discard, r)
					return
				}
				pr, pw := io.Pipe()
				go func() {
					defer pw.Close()
					planTasksStreamHandler(r, pw)
				}()
				emitter.EmitTextMarkdownStreamEvent(PlanTasksAINodeID, pr, taskIndex)
			},
		),
	)
	if err != nil {
		log.Warnf("plan loop: generate plan from document failed: %v", err)
		return ""
	}
	if action == nil {
		return ""
	}

	tasks := action.GetInvokeParamsArray("tasks")
	if action.GetString("main_task") == "" || action.GetString("main_task_goal") == "" || len(tasks) == 0 {
		log.Warnf("plan loop: plan from document has missing required fields")
		return ""
	}

	taskPayload := serializeTaskParams(tasks)

	payload := map[string]any{
		"@action":        "plan",
		"main_task":      action.GetString("main_task"),
		"main_task_goal": action.GetString("main_task_goal"),
		"tasks":          taskPayload,
	}
	if identifier := action.GetString("main_task_identifier"); identifier != "" {
		payload["main_task_identifier"] = identifier
	}
	log.Infof("plan loop: plan generated from document successfully (%d subtasks)", len(taskPayload))
	return string(utils.Jsonify(payload))
}

func serializeTaskParams(tasks []aitool.InvokeParams) []map[string]any {
	result := make([]map[string]any, 0, len(tasks))
	for _, subtask := range tasks {
		if subtask.GetString("subtask_name") == "" {
			continue
		}
		item := map[string]any{
			"subtask_name": subtask.GetString("subtask_name"),
			"subtask_goal": subtask.GetString("subtask_goal"),
			"depends_on":   subtask.GetStringSlice("depends_on"),
		}
		if identifier := subtask.GetString("subtask_identifier"); identifier != "" {
			item["subtask_identifier"] = identifier
		}
		if subTasks := subtask.GetObjectArray("sub_subtasks"); len(subTasks) > 0 {
			item["tasks"] = serializeTaskParams(subTasks)
		}
		result = append(result, item)
	}
	return result
}
