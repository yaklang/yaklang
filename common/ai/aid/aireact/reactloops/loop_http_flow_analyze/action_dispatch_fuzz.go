package loop_http_flow_analyze

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	loop_http_fuzztest "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_http_fuzztest"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var dispatchFuzzTestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"dispatch_fuzz_test",
		"Launch a dedicated HTTP fuzzing sub-loop targeting a specific HTTP flow identified as potentially vulnerable. "+
			"Provide the flow identifier and a description of the suspected vulnerability. "+
			"The sub-loop will run independently and return a vulnerability analysis summary.",
		[]aitool.ToolOption{
			aitool.WithStringParam("hidden_index",
				aitool.WithParam_Description("Hidden index of the target HTTP flow (preferred selector)")),
			aitool.WithStringParam("hash",
				aitool.WithParam_Description("Hash of the target HTTP flow (used when hidden_index is unavailable)")),
			aitool.WithIntegerParam("id",
				aitool.WithParam_Description("Numeric ID of the target HTTP flow")),
			aitool.WithStringParam("vulnerability_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Suspected vulnerability type(s) to test. E.g.: SQL注入, XSS, IDOR/越权, 路径穿越, SSRF, 命令注入, 信息泄漏, 未授权访问")),
			aitool.WithStringParam("task_description",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("中文描述：针对这条 flow 要验证什么、为什么怀疑存在该漏洞、重点关注哪些参数或 header")),
		},
		// ActionVerifier
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("hidden_index") == "" &&
				action.GetString("hash") == "" &&
				action.GetInt("id") == 0 {
				return utils.Error("dispatch_fuzz_test: must provide hidden_index, hash, or id to identify the target flow")
			}
			if action.GetString("vulnerability_type") == "" {
				return utils.Error("dispatch_fuzz_test: vulnerability_type is required")
			}
			if action.GetString("task_description") == "" {
				return utils.Error("dispatch_fuzz_test: task_description is required")
			}
			return nil
		},
		// ActionHandler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			db := consts.GetGormProjectDatabase()
			if db == nil {
				operator.Fail("project database is not available")
				return
			}

			emitter := loop.GetEmitter()
			invoker := loop.GetInvoker()
			task := loop.GetCurrentTask()
			taskID := ""
			if task != nil {
				taskID = task.GetId()
			}

			vulnType := action.GetString("vulnerability_type")
			taskDesc := action.GetString("task_description")
			locatorDesc := buildLocatorDesc(action)

			emitter.EmitThoughtStream(taskID,
				"[dispatch_fuzz_test] 正在加载目标 flow: %s，漏洞类型: %s", locatorDesc, vulnType)

			// Step 1: Load flow
			var flow *schema.HTTPFlow
			var err error
			switch {
			case action.GetInt("id") > 0:
				flow, err = yakit.GetHTTPFlow(db, int64(action.GetInt("id")))
			case action.GetString("hash") != "":
				flow, err = yakit.GetHTTPFlowByHash(db, action.GetString("hash"))
			default:
				flow, err = yakit.GetHTTPFlowByHiddenIndex(db, action.GetString("hidden_index"))
			}

			if err != nil || flow == nil {
				emitter.EmitThoughtStream(taskID, "[dispatch_fuzz_test] flow 加载失败: %v", err)
				recordMetaAction(loop, "dispatch_fuzz_test",
					"flow load failed: "+locatorDesc, utils.InterfaceToString(err))
				operator.Continue()
				return
			}

			rawRequest := flowRequest(flow)
			if strings.TrimSpace(rawRequest) == "" {
				emitter.EmitThoughtStream(taskID, "[dispatch_fuzz_test] flow 无可用 raw request: %s", locatorDesc)
				recordMetaAction(loop, "dispatch_fuzz_test",
					"no raw request: "+locatorDesc, "skipped")
				operator.Continue()
				return
			}

			// Step 2: Build sub-task userInput
			// fuzztest's buildInitTask will automatically extract raw HTTP packet from userInput
			subTaskUserInput := buildFuzzSubTaskUserInput(rawRequest, vulnType, taskDesc, flow)

			// Step 3: Create unique sub-task ID
			subTaskId := fmt.Sprintf("%s-fuzz-%s-%s",
				taskID,
				utils.RandStringBytes(6),
				sanitizeIDSegment(vulnType))

			// Step 4: Create fuzztest sub-loop
			fuzzLoop, err := reactloops.CreateLoopByName(
				loop_http_fuzztest.LoopHTTPFuzztestName,
				r,
				reactloops.WithMaxIterations(8),
			)
			if err != nil {
				emitter.EmitThoughtStream(taskID,
					"[dispatch_fuzz_test] 创建 fuzztest 子循环失败: %v", err)
				operator.Fail(fmt.Errorf("dispatch_fuzz_test: failed to create fuzztest loop: %w", err))
				return
			}

			// Step 5: Create and execute sub-task
			subTask := aicommon.NewSubTaskBase(task, subTaskId, subTaskUserInput)
			emitter.EmitThoughtStream(taskID,
				"[dispatch_fuzz_test] 启动 fuzztest 子循环: %s", subTaskId)
			invoker.AddToTimeline("dispatch_fuzz_test",
				fmt.Sprintf("启动 fuzztest 子循环 [%s]，目标: %s，漏洞类型: %s", subTaskId, locatorDesc, vulnType))

			execErr := fuzzLoop.ExecuteWithExistedTask(subTask)

			// Step 6: Collect results and write to findings
			fuzzResult := collectFuzzSubLoopResult(fuzzLoop, locatorDesc, vulnType, flow)
			if _, changed := appendFindings(loop, fuzzResult); changed {
				emitter.EmitThoughtStream(taskID,
					"[dispatch_fuzz_test] fuzztest 结果已合并到 FINDINGS")
			}

			// Record to dispatched_fuzz_tasks for reactive_data rendering
			appendDispatchedFuzzTask(loop, dispatchedFuzzTask{
				SubTaskID:      subTaskId,
				FlowLocator:    locatorDesc,
				FlowURL:        flow.Url,
				VulnType:       vulnType,
				TaskDesc:       taskDesc,
				ResultSummary:  fuzzResult,
				ExecutionError: utils.InterfaceToString(execErr),
			})

			invoker.AddToTimeline("dispatch_fuzz_test_done",
				fmt.Sprintf("fuzztest 子循环 [%s] 完成，结果长度: %d", subTaskId, len(fuzzResult)))

			recordMetaAction(loop, "dispatch_fuzz_test",
				fmt.Sprintf("fuzz: %s vulnType=%s", locatorDesc, vulnType),
				utils.ShrinkTextBlock(fuzzResult, 300))

			operator.Continue()
		},
	)
}

// buildFuzzSubTaskUserInput constructs the userInput to pass to the fuzztest sub-loop.
// The fuzztest buildInitTask will automatically extract raw HTTP packet from userInput.
func buildFuzzSubTaskUserInput(rawRequest, vulnType, taskDesc string, flow *schema.HTTPFlow) string {
	var sb strings.Builder
	sb.WriteString("请对下方 HTTP 请求进行安全模糊测试，重点验证以下漏洞类型：")
	sb.WriteString(vulnType)
	sb.WriteString("\n\n")
	sb.WriteString("## 测试说明\n\n")
	sb.WriteString(taskDesc)
	sb.WriteString("\n\n")

	// Flow metadata
	if flow != nil {
		sb.WriteString(fmt.Sprintf("## 目标 Flow 信息\n\n- URL: %s\n- 方法: %s\n- 状态码: %d\n- Tags: %s\n\n",
			flow.Url, flow.Method, flow.StatusCode, flow.Tags))
	}

	// Embed raw HTTP request (trigger condition for fuzztest buildInitTask)
	sb.WriteString("## 原始 HTTP 请求\n\n")
	sb.WriteString("```http\n")
	sb.WriteString(rawRequest)
	sb.WriteString("\n```\n\n")
	sb.WriteString("请先分析请求结构，然后针对上述漏洞类型制定并执行测试策略。")

	return sb.String()
}

// collectFuzzSubLoopResult extracts results from the completed fuzztest sub-loop and formats as findings snippet.
func collectFuzzSubLoopResult(fuzzLoop *reactloops.ReActLoop, locator, vulnType string, flow *schema.HTTPFlow) string {
	diffResult := strings.TrimSpace(fuzzLoop.Get("diff_result"))
	verResult := strings.TrimSpace(fuzzLoop.Get("verification_result"))
	analysis := strings.TrimSpace(fuzzLoop.Get("diff_result_analysis"))
	if analysis == "" {
		analysis = strings.TrimSpace(fuzzLoop.Get("diff_result_compressed"))
	}

	flowURL := ""
	if flow != nil {
		flowURL = flow.Url
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Fuzz 测试结果 [%s]\n\n", vulnType))
	sb.WriteString(fmt.Sprintf("- **目标**: %s\n", utils.ShrinkString(flowURL, 120)))
	sb.WriteString(fmt.Sprintf("- **定位**: %s\n", locator))

	if analysis != "" {
		sb.WriteString("\n### 分析摘要\n\n")
		sb.WriteString(utils.ShrinkTextBlock(analysis, 1500))
		sb.WriteString("\n")
	} else if diffResult != "" {
		sb.WriteString("\n### 差异分析\n\n")
		sb.WriteString(utils.ShrinkTextBlock(diffResult, 1500))
		sb.WriteString("\n")
	}

	if verResult != "" {
		sb.WriteString("\n### 验证结果\n\n")
		sb.WriteString(utils.ShrinkTextBlock(verResult, 800))
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// sanitizeIDSegment sanitizes a string to be suitable as part of a task ID.
func sanitizeIDSegment(s string) string {
	s = strings.ToLower(s)
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	cleaned := strings.Trim(result.String(), "-")
	if len(cleaned) > 20 {
		cleaned = cleaned[:20]
	}
	return cleaned
}
