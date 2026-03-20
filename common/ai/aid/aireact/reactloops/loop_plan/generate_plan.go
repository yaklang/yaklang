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
	var taskCount atomic.Int32

	// Ordering mechanism: syncCallback runs synchronously in the parser goroutine
	// (before each field handler goroutine is spawned), establishing a deterministic
	// chain of channels. Each handler goroutine waits for the previous one to finish
	// writing before it writes, eliminating the race on emitWriter output ordering.
	type orderCtx struct {
		waitCh <-chan struct{}
		doneCh chan struct{}
	}
	var ctxMap sync.Map // reader -> *orderCtx

	// prevDone is only accessed by syncCallback which runs sequentially in the parser goroutine.
	initCh := make(chan struct{})
	close(initCh)
	prevDone := (<-chan struct{})(initCh)

	wg := new(sync.WaitGroup)

	syncCallback := func(key string, reader io.Reader, parents []string) {
		myDone := make(chan struct{})
		ctxMap.Store(reader, &orderCtx{waitCh: prevDone, doneCh: myDone})
		prevDone = myDone
	}

	handler := func(key string, reader io.Reader, parents []string) {
		v, _ := ctxMap.LoadAndDelete(reader)
		oc := v.(*orderCtx)

		wg.Add(1)
		defer wg.Done()
		defer close(oc.doneCh)

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

		<-oc.waitCh
		buf.WriteTo(emitWriter)
	}

	err := jsonextractor.ExtractStructuredJSONFromStream(fieldReader,
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_name", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_identifier", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("subtask_goal", handler, syncCallback),
		jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback("depends_on", handler, syncCallback),
		jsonextractor.WithStreamErrorCallback(func(err error) {
			log.Errorf("plan tasks stream parse error: %v", err)
		}),
	)
	if err != nil {
		log.Errorf("plan tasks stream handler error: %v", err)
	}

	wg.Wait()
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
					aitool.WithParam_Description("将主任务拆解为一系列**细粒度、可执行的小任务**。核心粒度要求：每个子任务必须确保执行 agent 在 **约 3 步**（约 3 次工具调用，不超过 5 步）内可以完成。如果一个子任务需要超过 3 步才能完成，应考虑进一步拆分；超过 5 步则必须拆分。更细粒度的拆解有利于任务交付、进度追踪和失败重试。拆分维度：按操作阶段（探索/分析/实施/验证）拆分，或按操作对象（不同文件/模块/组件）拆分。子任务数量不设硬性上限，以合理粒度为准，如果任务复杂，可以达到十几步。通过 depends_on 字段管理依赖关系，独立任务可并行执行。"),
				},
				nil,
				aitool.WithStringParam("subtask_name", aitool.WithParam_Description("子任务的详细、可操作的名称。**必须采用'使用xx解决/完成xxx的xxx问题/目标'的格式**，包含三个要素：1）使用什么工具类别或方法（注意：**禁止使用内部工具函数名**如 write_file, read_file, send_http_request_packet, find_files, grep_text 等，应使用工具类别描述如'文件工具'、'HTTP请求'、'代码搜索'、'端口扫描'、'爬虫工具'等）；2）执行什么动作；3）针对什么目标对象的什么问题。例如：'使用端口扫描探测目标主机的服务暴露面'、'使用爬虫工具收集Web应用的攻击面'、'使用代码搜索定位项目中的硬编码密钥'、'构造HTTP请求验证目标的SQL注入漏洞'、'记录漏洞评估阶段的发现到项目文档'。名称应体现单一操作阶段，如果名称中出现'并且'、'同时'等并列连接词，说明任务粒度过大，需要拆分。**长度建议控制在30个汉字（或等效字符数）以内**。")),
				aitool.WithStringParam("subtask_identifier", aitool.WithParam_Description("子任务的语义标识符，使用英文蛇形命名(snake_case)，用于后续创建工作目录或引用。例如：'setup_dev_env'、'write_unit_tests'。可选字段，若不提供系统将根据 subtask_name 自动生成。")),
				aitool.WithStringParam("subtask_goal", aitool.WithParam_Description("定义该子任务的具体目标和衡量其完成的明确标准。要求：1）**完成条件**：在什么具体情况下可以认定此子任务已完成？2）**交付物**：此子任务完成后，应产出哪些具体成果？以'产出：xxx'的格式明确列出。3）**粒度自检**：目标描述应对应约 3 步可完成的工作量（不超过 5 步）。如果目标描述中包含多个独立的交付物或操作阶段，说明任务需要进一步拆分。例如，'读取项目配置文件并确认使用的编程语言和构建工具。产出：项目技术栈摘要'是合适的粒度；而'安装工具并配置规则并集成到CI'跨越了多个阶段，应拆分为独立子任务。")),
				aitool.WithStringArrayParam("depends_on", aitool.WithParam_Description("该子任务依赖的其他子任务名称(subtask_name)列表。被依赖的子任务需先完成后本任务才能开始执行。如无依赖关系则留空数组[]。例如：[\"使用find_files分析项目的技术栈组成\"]表示本任务需要在该任务完成后才能执行。合理利用依赖关系：探索类任务通常无依赖可并行执行，实施类任务依赖分析结果，验证类任务依赖实施完成。")),
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
				AINodeId:  "main-plan",
			},
			{
				FieldName: "main_task_identifier",
				AINodeId:  "main-plan",
			},
			{
				FieldName: "main_task_goal",
				AINodeId:  "main-plan-goal",
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
