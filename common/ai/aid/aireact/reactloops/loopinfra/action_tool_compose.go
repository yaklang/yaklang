package loopinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
)

var loopAction_toolCompose = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE,
	Description: "Compose multiple tool calls into a workflow DAG for complex multi-step operations",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_compose_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'tool_compose'. Provide a JSON array of tool call nodes, each with 'call_id', 'tool_name', 'call_intent', and 'depends_on' fields. Example: [{\"call_id\":\"step1_search\",\"tool_name\":\"search\",\"call_intent\":\"search for data\",\"depends_on\":[]}]"),
		),
	},
	OutputExamples: `
# tool-compose description
在命名是工具调用节点时，call_id 需要以 _ 结尾，例如 "step1_search"，"step2_write"，"step3_combine" 等，如无法确定第几步，可以使用直接使用 "search_material" 等简明扼要的 identifier 来命名。
注意，工具之间如果有依赖关系，一定要通过 depends_on 来指定

Example - Sequential file operations(With AI-Tag tags):

	{
	"@action": "tool_compose",
	"human_readable_thought": "Need to read a file, process its content, and write the result",
	}
	<|WORKFLOW_DAG_{{.Nonce}}|>
	[
		{"call_id":"read","tool_name":"read-file","call_intent":"Read source file"},
		{"call_id":"write","tool_name":"write-file","call_intent":"Write processed result","depends_on":["read"]}
	]
	<|WORKFLOW_DAG_END_{{.Nonce}}|>
`,
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		emitter := loop.GetEmitter()
		isDone := utils.NewBool(false)
		pr, pw := utils.NewPipe()
		defer func() {
			pw.Close()
			isDone.SetTo(true)
		}()
		emitter.EmitDefaultStreamEvent("thought", pr, loop.GetCurrentTask().GetId())
		pw.WriteString("智能工具编排中..")
		go func() {
			for {
				time.Sleep(time.Second)
				if isDone.IsSet() {
					return
				}
				pw.WriteString(".")
			}
		}()
		action.WaitStream(loop.GetCurrentTask().GetContext())

		payload := action.GetString("tool_compose_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_compose_payload")
		}
		if payload == "" {
			return utils.Error("tool_compose_payload is required for ActionToolCompose but empty")
		}
		// Validate that the payload is valid JSON
		var nodes []workflowdag.ToolCallNode
		if err := json.Unmarshal([]byte(payload), &nodes); err != nil {
			// Try to parse it as a string that might be double-encoded
			var unquoted string
			if err2 := json.Unmarshal([]byte(payload), &unquoted); err2 == nil {
				if err3 := json.Unmarshal([]byte(unquoted), &nodes); err3 == nil {
					payload = unquoted
				}
			}
		}
		loop.Set("tool_compose_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		payload := loop.Get("tool_compose_payload")
		if payload == "" {
			operator.Feedback(utils.Error("tool_compose_payload is required for ActionToolCompose but empty"))
			return
		}
		invoker := loop.GetInvoker()

		ctx := invoker.GetConfig().GetContext()
		t := loop.GetCurrentTask()
		if t != nil {
			ctx = t.GetContext()
		}

		// Build the DAG from the payload
		dag, err := workflowdag.BuildToolCallDAG(ctx, payload)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to build tool compose DAG: %v", err)
			invoker.AddToTimeline("[TOOL_COMPOSE_ERROR]", errMsg)
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			operator.SetReflectionData("dag_build_error", err.Error())
			operator.Feedback(utils.Error(errMsg))
			operator.Continue()
			return
		}

		if mermaidCode, _ := dag.GenerateMermaidFlowChartWithStyles(); mermaidCode != "" {
			markdownCompose, markdownComposeWriter := utils.NewPipe()
			emitter := loop.GetEmitter()
			emitter.EmitStreamEventWithContentType("tool_compose_progress", markdownCompose, loop.GetCurrentTask().GetId(), "text/markdown")
			markdownComposeWriter.WriteString("## Tool Compose DAG\n")
			markdownComposeWriter.WriteString("```mermaid\n")
			markdownComposeWriter.WriteString(mermaidCode)
			markdownComposeWriter.WriteString("```\n")
			markdownComposeWriter.Close()
		}

		// Log the DAG structure
		log.Infof("Tool compose DAG built successfully with %d nodes", len(dag.GetAllNodes()))
		invoker.AddToTimeline("[TOOL_COMPOSE_START]", fmt.Sprintf("Executing tool compose DAG with %d nodes", len(dag.GetAllNodes())))

		// Create a semaphore for concurrency control (set to 1 for sequential execution)
		sem := make(chan struct{}, 1)
		var executionErrors []string
		var errorsMu sync.Mutex

		// Execute the DAG with concurrency control
		err = dag.ExecuteWithHandler(func(ctx context.Context, node *workflowdag.ToolCallNode) error {
			// Acquire semaphore (concurrency = 1 means sequential execution)
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			log.Infof("Executing tool call: %s (tool: %s)", node.CallID, node.ToolName)
			invoker.AddToTimeline(fmt.Sprintf("[TOOL_COMPOSE_EXEC:%s]", node.CallID),
				fmt.Sprintf("Calling tool '%s' with intent: %s", node.ToolName, node.CallIntent))

			// Execute the tool using the invoker's ExecuteToolRequiredAndCall
			result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, node.ToolName)
			if err != nil {
				errMsg := fmt.Sprintf("Tool '%s' (call_id: %s) execution failed: %v", node.ToolName, node.CallID, err)
				log.Warnf(errMsg)
				invoker.AddToTimeline(fmt.Sprintf("[TOOL_COMPOSE_ERROR:%s]", node.CallID), errMsg)

				errorsMu.Lock()
				executionErrors = append(executionErrors, errMsg)
				errorsMu.Unlock()

				node.Error = err
				// If the node allows failure, continue; otherwise, return error
				if node.AllowFailed() {
					return nil
				}
				return err
			}

			if directly {
				// User requested direct answer, record and continue
				invoker.AddToTimeline(fmt.Sprintf("[TOOL_COMPOSE_DIRECT:%s]", node.CallID),
					"User requested direct answer during tool execution")
			}

			if result != nil {
				node.Result = result
				invoker.AddToTimeline(fmt.Sprintf("[TOOL_COMPOSE_DONE:%s]", node.CallID),
					fmt.Sprintf("Tool '%s' completed successfully", node.ToolName))
			}

			return nil
		})

		if err != nil {
			errMsg := fmt.Sprintf("Tool compose DAG execution failed: %v", err)
			invoker.AddToTimeline("[TOOL_COMPOSE_FAILED]", errMsg)
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			operator.SetReflectionData("dag_execution_error", err.Error())
			operator.SetReflectionData("execution_errors", executionErrors)
			operator.Feedback(utils.Error(errMsg))
			operator.Continue()
			return
		}

		// Collect results summary
		var resultSummary []string
		for _, node := range dag.GetAllNodes() {
			status := "completed"
			if node.IsFailed() {
				status = "failed"
			} else if node.IsSkipped() {
				status = "skipped"
			}
			resultSummary = append(resultSummary, fmt.Sprintf("%s(%s): %s", node.CallID, node.ToolName, status))
		}

		invoker.AddToTimeline("[TOOL_COMPOSE_COMPLETE]",
			fmt.Sprintf("All tool calls completed. Results: %v", resultSummary))

		// Verify user satisfaction
		task := loop.GetCurrentTask()
		if task != nil {
			verifyResult, err := invoker.VerifyUserSatisfaction(ctx, task.GetUserInput(), true, payload)
			if err != nil {
				operator.Fail(err)
				return
			}
			loop.PushSatisfactionRecordWithCompletedTaskIndex(
				verifyResult.Satisfied,
				verifyResult.Reasoning,
				verifyResult.CompletedTaskIndex,
				verifyResult.NextMovements,
			)

			if verifyResult.Satisfied {
				operator.Exit()
				return
			}
		}

		// If there were any errors during execution, provide feedback
		if len(executionErrors) > 0 {
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Standard)
			operator.SetReflectionData("partial_errors", executionErrors)
		}

		operator.Continue()
	},
}
