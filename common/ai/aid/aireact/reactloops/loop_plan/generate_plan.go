package loop_plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func planTasksStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	var mu sync.Mutex
	var taskCount atomic.Int32

	err := jsonextractor.ExtractStructuredJSONFromStream(fieldReader,
		jsonextractor.WithRegisterMultiFieldStreamHandler(
			[]string{"subtask_name", "subtask_goal", "subtask_identifier", "depends_on"},
			func(key string, reader io.Reader, parents []string) {
				reader = utils.JSONStringReader(reader)
				var buf bytes.Buffer
				switch key {
				case "subtask_name":
					if taskCount.Add(1) > 1 {
						buf.WriteString("\n")
					}
					buf.WriteString("[")
					io.Copy(&buf, reader)
					buf.WriteString("]")
				case "subtask_goal":
					buf.WriteString(": ")
					io.Copy(&buf, reader)
				case "subtask_identifier":
					buf.WriteString(" #")
					io.Copy(&buf, reader)
				case "depends_on":
					raw, _ := io.ReadAll(reader)
					trimmed := strings.TrimSpace(string(raw))
					if trimmed != "" && trimmed != "[]" {
						var deps []string
						if json.Unmarshal([]byte(trimmed), &deps) == nil && len(deps) > 0 {
							buf.WriteString(fmt.Sprintf(" (depends: %s)", strings.Join(deps, ", ")))
						}
					}
				}
				mu.Lock()
				buf.WriteTo(emitWriter)
				mu.Unlock()
			},
		),
		jsonextractor.WithStreamErrorCallback(func(err error) {
			log.Errorf("plan tasks stream parse error: %v", err)
		}),
	)
	if err != nil {
		log.Errorf("plan tasks stream handler error: %v", err)
	}
}

var generate = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"plan",
		"Generate a detailed plan based on the user's requirements or enhance the existing plan if one is already present.",
		[]aitool.ToolOption{
			aitool.WithStringParam("main_task", aitool.WithParam_Description("对用户原始需求进行提炼和重述，形成一个**清晰、具体、且可直接执行的主任务**。应以**动词开头**，明确指出核心行动，例如：'创建一个营销活动计划' 或 '分析用户流失数据'。")),
			aitool.WithStringParam("main_task_identifier", aitool.WithParam_Description("主任务的语义标识符，使用英文蛇形命名(snake_case)，用于后续创建工作目录等场景。例如：'create_marketing_plan'、'analyze_user_churn_data'。可选字段，若不提供系统将自动生成。")),
			aitool.WithStringParam("main_task_goal", aitool.WithParam_Description("定义主任务的最终目标及衡量其完成的明确标准。**必须清晰、无歧义地阐述以下三点**：1）**完成状态**：任务推进到何种程度可被视为已完成？2）**成功指标**：用哪些具体的、可量化的指标来评估任务是否成功达成目标？3）**交付成果**：任务完成后，预期的最终产出或交付物是什么？目标是提供一个**可验证的、客观的完成基准**。")),
			aitool.WithStructArrayParam(
				"tasks",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("将主任务拆解为一系列**具体、可执行的小任务**，每个小任务应包含以下要素：1）**任务描述**：简明扼要地说明任务内容和预期结果；2）**优先级**：根据任务的重要性和紧急程度进行排序（高、中、低）；3）**依赖关系**：通过 depends_on 字段明确指出该任务依赖哪些其他任务。确保所有小任务共同支持主任务的达成，并且每个任务都是独立且可操作的。"),
				},
				nil,
				aitool.WithStringParam("subtask_name", aitool.WithParam_Description("子任务的简洁、概括性名称。**强烈推荐采用'动词+名词'的格式**（例如：'设计用户调研问卷'、'部署测试环境'），以便清晰表达子任务的核心动作和对象。**长度建议控制在20个汉字（或等效字符数）以内**，以方便在任务管理和沟通中引用与追踪。")),
				aitool.WithStringParam("subtask_identifier", aitool.WithParam_Description("子任务的语义标识符，使用英文蛇形命名(snake_case)，用于后续创建工作目录或引用。例如：'setup_dev_env'、'write_unit_tests'。可选字段，若不提供系统将根据 subtask_name 自动生成。")),
				aitool.WithStringParam("subtask_goal", aitool.WithParam_Description("定义该子任务的具体目标和衡量其完成的明确标准。**必须清晰、无歧义地阐述以下三点**：1）**完成条件**：在什么具体情况下可以认定此子任务已完成？2）**交付物/输出要求**：此子任务完成后，应产出哪些具体的成果或达到哪些明确的输出标准？3）**成功指标（若适用）**：如果可能，提供可量化的指标来衡量子任务的完成质量。**目标是确保每个子任务都有一个明确、可验证的终点。** 例如，应描述为'生成包含至少三个设计方案的初步设计稿'，而非'进行初步设计'。避免使用如'进一步分析'、'收集相关信息'等缺乏明确完成标志的模糊描述")),
				aitool.WithStringArrayParam("depends_on", aitool.WithParam_Description("该子任务依赖的其他子任务名称(subtask_name)列表。被依赖的子任务需先完成后本任务才能开始执行。如无依赖关系则留空数组[]。例如：[\"配置开发环境\"]表示本任务需要在'配置开发环境'完成后才能执行。多个独立任务可以通过不设置依赖实现并行执行。")),
			),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName:     "tasks",
				AINodeId:      "plan",
				StreamHandler: planTasksStreamHandler,
			},
			{
				FieldName: "main_task",
				AINodeId:  "plan",
			},
			{
				FieldName: "main_task_identifier",
				AINodeId:  "plan",
			},
			{
				FieldName: "main_task_goal",
				AINodeId:  "plan",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			mainName := action.GetString("main_task")
			mainGoal := action.GetString("main_task_goal")
			taskList := action.GetInvokeParamsArray("tasks")
			if mainGoal == "" || mainName == "" || len(taskList) == 0 {
				return utils.Errorf("main_task parameter is missing")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.Set(PLAN_DATA_KEY, string(utils.Jsonify(action.GetParams())))
			op.Exit()
		},
	)
}
