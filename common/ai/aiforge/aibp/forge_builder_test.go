package aibp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"

	// 直接导入以触发 init 函数，替代原来的 depinjector
	_ "github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
)

var planJson = `{
  "@action": "plan",
  "main_task": "计算1+1的值",
  "main_task_goal": "计算1+1的值",
  "tasks": [
    {
      "subtask_name": "计算1+1的值",
      "subtask_goal": "计算1+1的值",
      "subtask_identifier": "calculate_result"
    }
  ]
}`

// The VM/factory assertions below exercise production Forge handles. Respond to
// the current coordinator/worker lifecycle, matching the requested wire protocol;
// obsolete legacy prompt fragments must not drive these integration fixtures.
func forgeBuilderResponse(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, args map[string]any) (*aicommon.AIResponse, error) {
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	rsp := i.NewAIResponse()
	if wire.ToolCallCallback != nil {
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("forge-builder-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
		wire.FinishReasonCallback("tool_calls", nil)
	} else {
		args["@action"], args["identifier"] = name, name
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		rsp.EmitOutputStream(strings.NewReader(string(raw)))
	}
	rsp.Close()
	return rsp, nil
}

func MockAICallback(t *testing.T, initFlag, persistentFlag, planFlag string) aicommon.AICallbackType {
	var calls int32
	var mu sync.Mutex
	workers := map[string]int{}
	briefPattern := regexp.MustCompile(`\[CURRENT_EXECUTION\]\n([^\n]+)`)
	reviewPattern := regexp.MustCompile(`\[([^\]]+)\]: awaiting_review; attempt=(\d+)`)
	return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if atomic.AddInt32(&calls, 1) > 32 {
			return nil, fmt.Errorf("Forge builder fixture did not converge: %s", req.GetCallerLabel())
		}
		prompt := req.GetPrompt()
		switch req.GetCallerLabel() {
		case "liteforge[intent-capability-recommend]":
			return rag.MockAIService(func(string) string { return aicommon.MockedIntentRecommendActionJSON })(i, req)
		case "liteforge[intent-keyword-gen]":
			return rag.MockAIService(func(string) string { return aicommon.MockedIntentKeywordGenActionJSON })(i, req)
		case "liteforge[task-short-id]":
			return rag.MockAIService(func(string) string { return `{"@action":"task-short-id","identifier":"calculate_result"}` })(i, req)
		case "liteforge[session-title-generator]":
			return rag.MockAIService(func(string) string { return `{"@action":"session-title-generator","session_title":"Forge builder"}` })(i, req)
		case aicommon.CallerLabelTimelineCompress:
			return forgeBuilderResponse(i, req, "timeline-summary", map[string]any{
				"summary": "计算已完成，结果为 2。", "ratain_timeline_item_range": "", "memory_entities": []any{},
			})
		case "liteforge[memory-triage]":
			return rag.MockAIService(func(string) string { return `{"@action":"memory-triage","memory_entities":[]}` })(i, req)
		case "liteforge[tag-selection]":
			return rag.MockAIService(func(string) string { return `{"@action":"tag-selection","tags":["test"]}` })(i, req)
		}
		if brief := briefPattern.FindStringSubmatch(prompt); len(brief) == 2 {
			if persistentFlag != "" && !strings.Contains(prompt, persistentFlag) {
				return nil, fmt.Errorf("persistent flag missing from worker brief")
			}
			var task struct {
				ID string `json:"task_id"`
			}
			if err := json.Unmarshal([]byte(brief[1]), &task); err != nil {
				return nil, err
			}
			mu.Lock()
			step := workers[task.ID]
			workers[task.ID] = step + 1
			mu.Unlock()
			if step == 0 {
				return forgeBuilderResponse(i, req, "submit_task_result", map[string]any{"summary": "result is 2"})
			}
			return forgeBuilderResponse(i, req, "finish", map[string]any{})
		}
		if req.GetCallerLabel() == "react-loop:coordinator" {
			start := strings.LastIndex(prompt, "# PLAN STATUS")
			if start < 0 {
				return nil, fmt.Errorf("coordinator lost its plan status")
			}
			status := prompt[start:]
			if strings.Contains(status, "已有计划：false") {
				for _, flag := range []string{initFlag, planFlag} {
					if flag != "" && !strings.Contains(prompt, flag) {
						return nil, fmt.Errorf("initial/plan flag missing from coordinator context")
					}
				}
				var plan map[string]any
				if err := json.Unmarshal([]byte(planJson), &plan); err != nil {
					return nil, err
				}
				delete(plan, "@action")
				return forgeBuilderResponse(i, req, "create_plan", map[string]any{"plan": plan, "plan_document": "# 计算表达式\n验证 1+1。"})
			}
			if strings.Contains(status, "阶段：PLAN") {
				return forgeBuilderResponse(i, req, "submit_plan", map[string]any{})
			}
			if match := reviewPattern.FindStringSubmatch(status); len(match) == 3 {
				attempt, _ := strconv.ParseUint(match[2], 10, 64)
				return forgeBuilderResponse(i, req, "review_task", map[string]any{"task_id": match[1], "attempt_id": attempt, "decision": "accept", "reason": "计算结果为 2，与冻结任务书一致。"})
			}
			return forgeBuilderResponse(i, req, "wait_messages", map[string]any{})
		}
		return nil, fmt.Errorf("unexpected Forge builder request: %s", req.GetCallerLabel())
	}
}

func runTestForgeByAICommon(t *testing.T, forge *schema.AIForge, initFlag, persistentFlag string) (any, error) {
	db := consts.GetGormProfileDatabase()
	forge.IsTemporary = true
	err := yakit.CreateOrUpdateAIForgeByName(db, forge.ForgeName, forge)
	if err != nil {
		return nil, err
	}

	defer func() {
		yakit.DeleteAIForgeByName(db, forge.ForgeName)
	}()

	result, err := aicommon.ExecuteForgeFromDB(forge.ForgeName, context.Background(), map[string]any{
		"query": "1+1",
	},
		aicommon.WithAICallback(MockAICallback(t, initFlag, persistentFlag, "")),
		// Factory/VM assertions use a mocked AI callback; memory must use the
		// matching local fixture rather than initialize a remote embedder.
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableDynamicPlanning(true),
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}
func RunTestForge(t *testing.T, forge *schema.AIForge, initFlag, persistentFlag string) (any, error) {
	return runTestForgeByAICommon(t, forge, initFlag, persistentFlag)
	// db := consts.GetGormProfileDatabase()
	// forge.IsTemporary = true
	// err := yakit.CreateOrUpdateAIForgeByName(db, forge.ForgeName, forge)
	// if err != nil {
	// 	return nil, err
	// }

	// defer func() {
	// 	yakit.DeleteAIForgeByName(db, forge.ForgeName)
	// }()

	// result, err := yak.ExecuteForge(forge.ForgeName, map[string]any{
	// 	"query": "1+1",
	// },
	// 	aicommon.WithAICallback(MockAICallback(t, initFlag, persistentFlag, "")),
	// 	aicommon.WithAgreeYOLO(),
	// 	aicommon.WithDebugPrompt(true),
	// )
	// if err != nil {
	// 	return nil, err
	// }
	// return result, nil
}

func TestBuildForgeFromYak(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
init = "帮我计算表达式的值` + initFlag + `"
persis = "一定要算准一点` + persistentFlag + `"

forgeHandle = func(params) {
	result = ""
    bp = aiagent.CreateForge("add",
        aiagent.persistentPrompt(persis),
		aiagent.initPrompt(init),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			result = config.GetContextProvider().CurrentTask.TaskSummary
		}),
    )
    ordr,err = bp.CreateCoordinator(context.Background(),params)
    if err != nil {
		return nil
	}
    err = ordr.Run()
    if err != nil {
		return nil
	}
		println(result)
    return result
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestBuildForgeFromYakWithDefaultPrompt(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()

forgeHandle = func(params) {
	result = ""
    bp = aiagent.CreateForge("add",
        aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			result = config.GetContextProvider().CurrentTask.TaskSummary
		}),
    )
    ordr,err = bp.CreateCoordinator(context.Background(),params)
    if err != nil {
		return nil
	}
    err = ordr.Run()
    if err != nil {
		return nil
	}
    return result
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestBuildForgeFromYakWithDefaultForgeHandle(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakWithWarpDefaultForgeHandle(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res,_ = __DEFAULT_FORGE_HANDLE__(params,opts...)
	res.Formated = "result is 2"
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	forgeResult, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
	assert.Equal(t, forgeResult.Formated, "result is 2")
}
func TestBuildForgeFromYakWithRewriteDefaultPrompt(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
__INIT_PROMPT__ = "帮我计算表达式的值" + "` + initFlag + `"
__PERSISTENT_PROMPT__ = "一定要算准一点" + "` + persistentFlag + `"
`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakNoCheck(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent:     `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestBuildForgeFromYakNoQuery(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	_, ok := result.(*aiforge.ForgeResult)
	assert.Assert(t, ok)
}

func TestNewForgeExecutor(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forge := &schema.AIForge{
		ForgeName:        forgeName,
		PersistentPrompt: "一定要算准一点" + persistentFlag,
		InitPrompt:       "帮我计算表达式的值" + initFlag,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res = ""
	opts = append(opts,
		aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			res = config.GetContextProvider().CurrentTask.TaskSummary
		}),
	)
	excutor,err := aiagent.NewExecutor("test",params,opts...)
	if err != nil {
		return nil
	}
	err = excutor.Run()
	if err != nil {
		return nil
	}
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}

func TestNewForgeExecutorFromJson(t *testing.T) {
	var initFlag = uuid.New().String()
	var persistentFlag = uuid.New().String()
	forgeName := utils.RandStringBytes(10)
	forgeData := map[string]any{
		"name":              forgeName,
		"persistent_prompt": "一定要算准一点" + persistentFlag,
		"init_prompt":       "帮我计算表达式的值" + initFlag,
		"forge_content":     "query = cli.String(\"query\", cli.setRequired(true), cli.setHelp(\"query\"))",
	}
	jsonData, err := json.Marshal(forgeData)
	if err != nil {
		t.Errorf("marshal forge data failed: %v", err)
		return
	}
	jsonForge := strconv.Quote(string(jsonData))
	forge := &schema.AIForge{
		ForgeName: forgeName,
		ForgeContent: `query = cli.String("query", cli.setRequired(true), cli.setHelp("query"))
cli.check()
forgeHandle = func(params,opts...) {
	res = ""
	opts = append(opts,
		aiagent.persistentPrompt(__PERSISTENT_PROMPT__),
		aiagent.initPrompt(__INIT_PROMPT__),
		aiagent.agreeYOLO(true),
		aiagent.resultHandler((config) => {
			res = config.GetContextProvider().CurrentTask.TaskSummary
		}),
	)
	excutor,err := aiagent.NewExecutorFromJson(` + jsonForge + `,params,opts...)
	if err != nil {
		return nil
	}
	err = excutor.Run()
	if err != nil {
		return nil
	}
	return res
}`,
	}
	result, err := RunTestForge(t, forge, initFlag, persistentFlag)
	if err != nil {
		t.Errorf("run forge %v failed: %v", forge.ForgeName, err)
		return
	}
	assert.Equal(t, result, "result is 2")
}
